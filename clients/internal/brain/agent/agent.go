// Package agent lets the brain use its capabilities rather than only describe
// them: ask the model, run what it asks for, feed the results back, repeat.
//
// The behaviour that matters is what happens when the model wants to change
// something. The loop does not ask forgiveness, and it does not block a request
// waiting on a person — it stops, leaves the action queued for approval, and
// says so plainly. Work resumes on a later turn once the decision exists.
//
// That is what keeps the permission gate meaningful: there is no arrangement of
// model output that turns a pending action into a taken one.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/pace"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/sandbox"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
)

// MaxSteps caps model-to-tool-to-model round trips in a single turn.
//
// Without it, a model that keeps re-reading the same file loops forever, and
// small local models do this readily.
const MaxSteps = 8

// ToolOutputLimit truncates what is written into the transcript.
//
// A large file read would otherwise dominate the conversation and every future
// prompt built from it, crowding out the exchange that gave it meaning.
const ToolOutputLimit = 4000

// Loop runs a turn to completion, or to the point where a person is needed.
type Loop struct {
	/*
	 * Aloud starts speaking an answer before it has finished being written.
	 *
	 * Set only for turns being held out loud. Nil for anything typed, where
	 * there is nothing to gain and a voice nobody asked for.
	 */
	Aloud func(ctx context.Context) TalkAloud

	// Interrupted reports that its owner cut in, so the rest of an answer can
	// be abandoned rather than written out to nobody.
	Interrupted func() bool

	/*
	 * OffLimits hides a tool from the model and refuses to run it.
	 *
	 * For rules that can change while the program is running, which is
	 * privacy and nothing else so far. A tool the model can see is a tool it
	 * will try, so a forbidden one has to be invisible rather than merely
	 * refused — and it has to be refused as well, because a model that has
	 * seen it once in a conversation will name it again.
	 *
	 * Nil means everything registered is allowed.
	 */
	OffLimits func(tool string) bool

	/*
	 * MayI decides whether a tool that changes something runs or waits.
	 *
	 * A separate question from OffLimits, and the separation is the point.
	 * OffLimits is privacy — what may leave the machine — and it hides a tool
	 * entirely. This is permission: what may be done on the machine in front
	 * of you, by a tool that is perfectly allowed to exist.
	 *
	 * Asked fresh at the moment of acting, because a person can grant a
	 * permission in the approval they are looking at and the next call in the
	 * same turn should already know about it.
	 *
	 * Nil keeps the old behaviour exactly: everything that changes something
	 * stops and asks.
	 */
	/*
	 * MayI decides whether an action goes ahead, and who is asking.
	 *
	 * Who, because several agents mean several different amounts of
	 * authority, and an agent able to borrow another's would make the roster
	 * a way round the gate rather than a narrowing of it. Empty is the owner
	 * talking to the assistant directly, which is where every decision used
	 * to come from and still means the same thing.
	 *
	 * Still one closure, wired once. Several agents must not mean several
	 * gates that are meant to agree.
	 */
	MayI func(who, tool string, changesSomething bool, level risk.Level) permits.Answer

	/*
	 * NothingAsks is whether its owner has said never to stop.
	 *
	 * MayI already answers that for the capability. This is the other half:
	 * a protected file puts a question whatever has been granted, because
	 * "you may read files" is not "you may read this key" — and on "never
	 * stop, never refuse" that question is exactly what its owner chose not
	 * to be asked. The switch was offered as "nothing asks", and a key file
	 * still stopping the work would be the setting not doing what it says.
	 *
	 * The summary still names the file and the rule, so the record says what
	 * was read and why it would have been asked about. Nil keeps asking.
	 */
	NothingAsks func() bool

	/*
	 * Model chooses which model answers a turn, or is nil to leave it alone.
	 *
	 * Set by the brain, which knows what is installed. It is asked once per
	 * turn rather than once per step, so a turn that uses a tool does not
	 * change model halfway through and forget how it was going to finish.
	 */
	Model func(message string) llm.Choice

	/*
	 * Quietly keeps this loop out of the panel that says what the brain is
	 * doing for its owner.
	 *
	 * For the loop a task runs on. progress holds a single current step by
	 * design — it is a line somebody reads while they wait, not a log — so a
	 * task working in the background would overwrite the line belonging to the
	 * conversation somebody is actually sitting in front of. "Is it working on
	 * my question" would then have the wrong answer on screen while being
	 * right underneath. A task's work still shows, in the activity feed and in
	 * its own view.
	 */
	Quietly bool

	DB       *store.DB
	Registry *tools.Registry
	Log      *slog.Logger

	/*
	 * The schemas, built once and rebuilt when the tool list changes.
	 *
	 * It was a sync.Once, from when the registry was built at startup and
	 * never touched again. A skill can now be taught in the middle of a
	 * conversation, and a cache that can never be invalidated would mean the
	 * model was never told about it — the tool would exist, be callable, and
	 * be invisible, which is the most confusing of the three possibilities.
	 */
	specsMu sync.Mutex

	// offered is what each conversation has already been shown, so the list
	// only ever grows within one. See choosing.go.
	offered remembered

	/*
	 * Have reports whether something a tool needs is on this machine.
	 *
	 * Wired to the one reading of the machine — see internal/brain/environs.
	 * Nil means offer everything, which is right for a caller that knows
	 * nothing about the machine: a tool withheld because nobody asked is
	 * worse than one that tries and explains itself.
	 */
	Have        func(id string) bool
	cachedSpecs []llm.ToolSpec
	specsFor    int64
}

// Pending is an action waiting for approval.
type Pending struct {
	ID      int64  `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// Result is the outcome of one turn.
/*
 * Step is one thing the assistant did, kept in enough detail to be looked at.
 *
 * ActionsTaken has always been beside this and stays: it is the one-line
 * summary a person reads without asking for more, and plenty of the interface
 * wants exactly that. What it could not do is answer "yes, but what did it
 * actually run, and what came back" — so watching the assistant work meant
 * trusting a sentence that describes a category of action rather than the
 * action.
 *
 * Asked is the arguments as the model wrote them; Result is what came back,
 * trimmed. Both are the difference between a tool called with the wrong
 * argument and a tool that is merely slow, which look identical from outside.
 */
type Step struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Asked   string `json:"asked,omitempty"`
	Result  string `json:"result,omitempty"`
	Failed  bool   `json:"failed,omitempty"`
	Millis  int64  `json:"millis"`

	// Level is how serious the call was, weighed on what it was handed. See
	// the risk package.
	Level risk.Level `json:"level,omitempty"`

	// Evidence is what the call can prove it did, from its own arguments and
	// output. See tools.Perform.
	Evidence []store.Evidence `json:"evidence,omitempty"`
}

type Result struct {
	// AlreadySpoken is true when the answer was said aloud as it was written,
	// so the caller must not say the whole thing again.
	AlreadySpoken bool

	Reply        string
	Provider     string
	Model        string
	ActionsTaken []string

	// Steps is the same work with the detail kept, for an interface that lets
	// somebody open one up.
	Steps []Step

	Pending      []Pending
	HitStepLimit bool

	/*
	 * Rounds is how many times the model was asked during this turn.
	 *
	 * A turn that uses a tool is several calls, and on this machine each of
	 * them is most of a minute — so anything budgeting a long piece of work in
	 * model calls has to be told, rather than guessing from how many tools
	 * happened to run.
	 */
	Rounds int

	/*
	 * Asked is true when the turn ended on a question rather than an answer.
	 *
	 * Worth distinguishing from an ordinary reply, because everything
	 * downstream treats "here is what I found" and "which of these did you
	 * mean" differently: the second is not something to learn from, and it is
	 * the one case where the brain has deliberately not finished.
	 */
	Asked bool
}

// WaitingForApproval reports whether the turn stopped for a person.
func (r Result) WaitingForApproval() bool { return len(r.Pending) > 0 }

// Run executes one turn.
func (l *Loop) Run(
	ctx context.Context,
	conversationID int64,
	provider llm.Provider,
	messages []llm.Message,
) (Result, error) {
	return l.RunWithLimit(ctx, conversationID, provider, messages, 0)
}

// RunWithLimit is Run with a cap on how much the model may produce.
func (l *Loop) RunWithLimit(
	ctx context.Context,
	conversationID int64,
	provider llm.Provider,
	messages []llm.Message,
	maxTokens int,
) (Result, error) {
	return l.RunShaped(ctx, conversationID, provider, messages, maxTokens, true)
}

// RunShaped is the full form: the caller decides whether tools are offered.
//
// Withholding them is not a small saving. The schemas come to about 533 tokens
// and are sent on every call, which on a CPU processing ten tokens a second is
// most of a minute before the model has read the question. For a spoken turn
// that is a bad trade — somebody talking wants an answer, not a file written —
// so voice asks without them and gets the time back.
func (l *Loop) RunShaped(
	ctx context.Context,
	conversationID int64,
	provider llm.Provider,
	messages []llm.Message,
	maxTokens int,
	withTools bool,
) (Result, error) {
	return l.RunAs(ctx, conversationID, provider, messages, maxTokens, withTools, llm.Choice{})
}

/*
 * RunAs is RunShaped with the model already decided.
 *
 * For a caller that has more to go on than the message. The conductor knows
 * which step of which task this is and what shape of work it is, where
 * RunShaped has only a sentence to judge by. The choice is still made once and
 * held for the whole turn, which is the property that matters — only who makes
 * it moves.
 *
 * An empty Choice.Model falls back to l.Model exactly as before, so nothing
 * that calls RunShaped notices this exists.
 */
func (l *Loop) RunAs(
	ctx context.Context,
	conversationID int64,
	provider llm.Provider,
	messages []llm.Message,
	maxTokens int,
	withTools bool,
	choice llm.Choice,
) (Result, error) {
	return l.RunBrief(ctx, conversationID, provider, messages, Brief{
		Choice: choice, WithTools: withTools, MaxTokens: maxTokens,
	})
}

/*
 * Brief is everything a caller decides about one turn.
 *
 * A struct rather than three more parameters, because the list had reached
 * seven and the next thing to be decided about a turn will not be the last.
 */
type Brief struct {
	// Choice is the model, already decided. An empty Model falls back to the
	// loop's own chooser.
	Choice llm.Choice

	WithTools bool
	MaxTokens int

	/*
	 * Only, when set, is the only tools this turn may use.
	 *
	 * A narrowing of what the assistant may already do, and never a widening:
	 * everything named here still has to exist, still goes through the same
	 * permits book, and is still refused by privacy. So an agent with a short
	 * tool list can be given to a model that has talked its way into asking
	 * for something else, and the answer is no.
	 *
	 * It also makes small models better rather than only safer. How well a
	 * model chooses among tools falls off sharply with its size, and a
	 * researcher offered five options chooses better than one offered
	 * thirty-one.
	 */
	Only []string

	/*
	 * Never is tools this turn may not use, whatever else it may.
	 *
	 * Separate from Only because it answers a different question. Only is
	 * "what is this job about" — a narrowing for the sake of the choice being
	 * easy. Never is "what is not this job's to decide", and it has to hold
	 * even for the generalist, whose Only list is empty precisely because it
	 * does everything.
	 *
	 * What it exists for: decide_waiting carries out the actions the approval
	 * gate is holding. In a conversation that is correct — it runs because
	 * its owner asked for it in the sentence they just spoke. In a task the
	 * sentence was written by a planner, and an agent working unattended
	 * approving what its owner has not seen is the gate deciding itself.
	 */
	Never []string

	/*
	 * As is who is acting, by the name the roster knows them by.
	 *
	 * Empty is the owner's own assistant answering the owner. It reaches the
	 * approval gate, so that what one agent has been allowed is not what all
	 * of them have been allowed.
	 */
	As string

	/*
	 * Within is the project this turn's work is confined to, or nil for none.
	 *
	 * Every call that can say where it would write, or where it would run a
	 * program, is held to it: outside the project's folders it is refused,
	 * and a build that would write anywhere but the project's outputs is
	 * refused too. Refused before the gate, like Never, so no approval can be
	 * asked for — and so be given — for work outside the project.
	 */
	Within Confine

	// Integrations is the integrations the work may use — a project's —
	// or nil when it is not confined to one. See tools.MayUse.
	Integrations *[]string

	/*
	 * Risk is how serious the work this turn belongs to is.
	 *
	 * A floor under every call that changes something, never under a look:
	 * a critical step's file write is as critical as the step, where its
	 * directory listing is still only a directory listing. Empty is a
	 * conversation, where each call is weighed on its own.
	 */
	Risk risk.Level
}

// Confine is where a project's work may write. See workspace.Scope.
type Confine interface {
	Allows(path string) bool
	Output(path string) bool

	// Writable is the folders themselves, for the kernel to hold what runs
	// to them.
	Writable() []string
}

// Confined is ctx for a call made on a project: whatever the call starts may
// write only in the project and the caches its tools keep.
func Confined(ctx context.Context, within Confine) context.Context {
	if within == nil {
		return ctx
	}

	return sandbox.WithWritable(ctx, sandbox.ForWork(within.Writable()...))
}

/*
 * outside is why a call would act outside the project, or empty.
 *
 * A command with no folder of its own is refused rather than assumed to be in
 * the project: where this program happens to be running is not a place, and
 * "go build" run there is a build of whatever that is.
 */
// Outside is outside, for the one other place a call is carried out: an
// action approved after the turn that asked for it had ended.
func Outside(within Confine, tool tools.Tool, args json.RawMessage) string {
	return outside(within, tool, args)
}

func outside(within Confine, tool tools.Tool, args json.RawMessage) string {
	if within == nil {
		return ""
	}

	writes, runs, said := tools.TouchesOf(tool, args)
	if !said {
		/*
		 * Failing closed. A tool that changes something and cannot say where
		 * was, until an audit tried it, let through on every project — a
		 * subtitle file written to any folder, "put it back" anywhere. Only
		 * what writes no files at all (an integration, governed by its own
		 * grants) goes on without saying.
		 */
		if _, free := tool.(tools.FileFree); free || tool.Risk() != tools.Mutating {
			return ""
		}

		return tool.Name() + " cannot say where it would write, so it is not used on a project"
	}

	for _, dir := range runs {
		if strings.TrimSpace(dir) == "" {
			return "it does not say which folder to run in — give the project's folder as dir"
		}

		if !within.Allows(dir) {
			return dir + " is outside the project"
		}
	}

	for _, path := range writes {
		if strings.TrimSpace(path) == "" {
			continue
		}

		if !within.Allows(path) {
			if workspace.Protected(path) {
				return path + " is the project's own settings, which its owner changes, not the work"
			}

			return path + " is outside the project"
		}
	}

	if why := tools.Unconfinable(tool, args); why != "" && !sandbox.Enabled() {
		return why
	}

	return ""
}

// RunBrief is one turn, with everything about it decided by the caller.
func (l *Loop) RunBrief(
	ctx context.Context,
	conversationID int64,
	provider llm.Provider,
	messages []llm.Message,
	brief Brief,
) (result Result, err error) {
	maxTokens, withTools, choice := brief.MaxTokens, brief.WithTools, brief.Choice

	var actions []string

	// The same work, kept in full. See Step.
	var steps []Step

	/*
	 * Which model, decided once for the whole turn.
	 *
	 * Once, not per step: a turn that calls a tool is several model calls, and
	 * swapping model between them means the one that reads the tool's answer is
	 * not the one that asked for it.
	 */
	var model string

	// Tools are offered unless the turn is plainly conversational. Describing
	// them costs hundreds of tokens of prompt on a processor that manages ten a
	// second, and "good morning" needs none of them.
	offerTools := withTools

	chosen := choice.Model != ""

	if !chosen && l.Model != nil {
		choice = l.Model(lastUserMessage(messages))
		chosen = true
	}

	if chosen {
		model = choice.Model
		offerTools = withTools && choice.Tools

		// Held to its job for this turn only, where a smaller model needs it.
		if choice.Guidance != "" {
			messages = append(messages,
				llm.Message{Role: llm.RoleSystem, Content: choice.Guidance})
		}

		l.usingModel(choice.Model)

		l.Log.Info("model for this turn",
			"model", choice.Model, "why", choice.Why, "tools", offerTools)

		// Which model, and whether it was handed the tools — the two facts
		// that move the timings more than anything else about the machine.
		pace.Asked(choice.Model, offerTools)
	}

	var specs []llm.ToolSpec

	if offerTools {
		/*
		 * Chosen: what every turn needs, what this conversation has already
		 * been shown, and what this message plausibly needs — up to a budget.
		 *
		 * Everything the assistant has is still available; this is what fits
		 * in one question. See choosing.go for the two measurements that
		 * decide it: what a tool list costs to read on this machine, and how
		 * badly a small model chooses when given too many.
		 */
		specs = Choosing{
			Have:  l.Have,
			Needs: l.needsFor,
			Cued:  l.cuesFor,
		}.choose(
			l.granted(without(onlyThese(l.specs(), brief.Only), brief.Never), brief),
			lastUserMessage(messages),
			l.offered.already(conversationID),
		)

		// Remembered, so the next turn in this conversation begins the same
		// way and the model keeps what it has already read.
		l.offered.keep(conversationID, namesOf(specs))
	}

	// Whether the model has already been asked to keep a promise this turn.
	pressed := false

	// What is happening, for anything watching. A turn that uses a tool is
	// several model calls with work between them, and on this machine that can
	// run to minutes — long enough that an interface saying nothing is
	// indistinguishable from one that has crashed.
	l.began()
	defer l.finished()

	/*
	 * How many times the model was asked, reported by every way out.
	 *
	 * A defer rather than a field on each of the five returns, for the same
	 * reason the assistant's message is appended above the loop: a number that
	 * five branches each have to remember to set is a number that is wrong on
	 * whichever branch was added last.
	 */
	rounds := 0

	defer func() { result.Rounds = rounds }()

	for step := 0; step < MaxSteps; step++ {
		rounds++

		l.round(step + 1)
		l.doing("thinking", "Thinking")

		/*
		 * Which model, with how much to read, before it starts.
		 *
		 * This is where the minutes go on a machine without a graphics card,
		 * and it was the one step in the panel that said nothing at all about
		 * itself. Both numbers explain the wait: the model because a
		 * seven-billion-parameter one is minutes where a three-billion one is
		 * seconds, and the size of the conversation because every token of it
		 * is read again on every step of the turn.
		 */
		l.detail(fmt.Sprintf("%s · reading %s", model, sizeOfPrompt(messages)))

		// Which step this is, so what the model produced is reported against
		// the step that ran it. The call outlives its own step — it starts
		// under "Thinking" and returns after answering and speaking — so
		// attaching to whatever is current put a model's running time under
		// "Listening", which had not been running at all.
		thinkingStep := l.mark()
		thought := time.Now()

		request := llm.Request{
			Messages: messages, Tools: specs, MaxTokens: maxTokens, Model: model,
		}

		/*
		 * When there is nothing to decide, say it as it is written.
		 *
		 * Only with no tools offered: a turn that might call one has to be read
		 * whole before anything can happen, because a tool call is not
		 * speakable and half of one is not anything. Without tools the answer
		 * is words and nothing else, so the voice can start on the first
		 * sentence while the rest is still being produced — which is the
		 * difference between talking to something and waiting for it.
		 */
		var resp llm.Response
		var err error

		/*
		 * Streamed whenever the provider can, tools or not.
		 *
		 * Holding back every turn that might call a tool meant that exactly the
		 * turns worth waiting for — the ones that do something — were the ones
		 * that said nothing for minutes. And a model that is going to call a
		 * tool says so in its first characters: an opening brace, or a fence.
		 * That is enough to tell the two apart before a word has been spoken.
		 */
		/*
		 * Streamed whenever the provider can, whether or not it is spoken.
		 *
		 * This used to depend on the voice being wired, which is only true for
		 * turns held out loud — so a typed question showed nothing at all
		 * until the whole answer arrived, which on this machine is minutes of
		 * an empty panel. The voice and the screen want the same stream for
		 * different reasons, and only one of them was getting it.
		 */
		resp, err = l.streamAloud(ctx, provider, request)
		if err != nil {
			return Result{}, err
		}

		// Local models regularly print a tool call as prose instead of making
		// one. Measured against qwen2.5-coder:7b, two of five replies did this,
		// including a correct decision to list a directory. Discarding that
		// would throw away the model's actual intent and leave the brain
		// looking incapable of using its own tools.
		//
		// Recovering it is safe because nothing downstream changes: the call
		// goes through the same registry lookup and the same risk gate, so a
		// Mutating tool still stops for approval whether the model asked for it
		// properly or in prose.
		/*
		 * How long it took and roughly how fast, once it has answered.
		 *
		 * The rate is the number worth having. Somebody watching a turn take
		 * four minutes cannot tell an overloaded machine from a model too
		 * large for it, and those want opposite responses — wait, or choose a
		 * smaller model. Tokens per second says which.
		 */
		l.detailOn(thinkingStep, rateOfReply(resp.Content, time.Since(thought)))

		if len(resp.ToolCalls) == 0 {
			if recovered, ok := l.recoverToolCall(resp.Content); ok {
				resp.ToolCalls = []llm.ToolCall{recovered}
				resp.Content = ""
			}
		}

		/*
		 * An id for every call, whether or not the provider gave one.
		 *
		 * Ollama sends none and needs none — it matches results to calls by
		 * position. Anthropic and every OpenAI-compatible service refuse a
		 * result that names no call, so the id is the whole of what ties the
		 * two together. A recovered call carried the literal "recovered",
		 * which two of them in one reply would have shared, and two results
		 * answering the same call is that same refusal by another route.
		 */
		for i := range resp.ToolCalls {
			if id := resp.ToolCalls[i].ID; id == "" || id == "recovered" {
				resp.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d", step, i)
			}
		}

		if len(resp.ToolCalls) == 0 {
			/*
			 * It said it would, and it did not.
			 *
			 * This is the failure this program has had more than any other, in
			 * every form: "Understood, I'll approve everything", "I will keep
			 * track of what is waiting", "To see what is waiting, give me the
			 * command:" — and then nothing at all happens. Every earlier fix
			 * addressed a way the call was being lost after the model made it.
			 * This one is for when the model simply did not make it.
			 *
			 * Asked once, and only once. A model that answers a direct
			 * instruction with another promise is not going to be talked round
			 * by a third attempt, and each one costs a full turn on a processor
			 * where that is minutes.
			 */
			if promised(resp.Content) && len(specs) > 0 && !pressed {
				pressed = true

				messages = append(messages,
					llm.Message{Role: llm.RoleAssistant, Content: resp.Content},
					llm.Message{Role: llm.RoleSystem, Content: keepThePromise})

				l.Log.Info("the model promised an action without taking one; asking again")
				l.doing("thinking", "Doing it")

				continue
			}

			reply := presentable(resp.Content)

			// A small model sometimes prints a tool call as prose instead of
			// making one. presentable removes it, which can leave nothing at
			// all — and silence reads as a crash. Say what happened instead,
			// because a person who sees "no reply" has no idea whether to wait,
			// retry, or check a log.
			if reply == "" {
				reply = "The model returned a tool call as text rather than making one, " +
					"so there is no answer to show. Asking again usually works; a larger " +
					"local model is more reliable at this."
			}

			/*
			 * The same answer twice is not an answer.
			 *
			 * A small model asked something it cannot do will often produce
			 * the identical sentence again — word for word, to a different
			 * question — and handing that back is the thing that makes a
			 * conversation stop being one. Somebody asked to learn everything,
			 * was offered a choice of three, said "learn everything" again,
			 * and was offered the same three in the same words.
			 *
			 * Saying so is more use than repeating it, and it is honest: the
			 * model is stuck, and the person is the only one who can move it.
			 */
			if repeatOf(reply, messages) {
				reply = "That is word for word what I just said, which means I " +
					"have not understood you rather than that the answer has " +
					"not changed. Put it a different way, or tell me exactly " +
					"what you want done and I will do that instead of offering."
			}

			return Result{
				Reply:         reply,
				Provider:      resp.Provider,
				Model:         resp.Model,
				ActionsTaken:  actions,
				Steps:         steps,
				AlreadySpoken: resp.Spoken,
			}, nil
		}

		/*
		 * The message that asked, before any of the answers.
		 *
		 * This is the whole of why a hosted model could not use a tool twice.
		 * Every path below appends the tool's result and none of them appended
		 * the message that asked for it, so the conversation read as answers
		 * to questions nobody had put: Anthropic dropped them and asked its
		 * question again, unaware it had ever looked anything up, and every
		 * OpenAI-compatible service refused the request outright.
		 *
		 * Appended once, here, so that every call in it is answered by one of
		 * the branches below — including the ones naming a tool that does not
		 * exist and the ones privacy refuses. Both APIs require an answer to
		 * every call, and putting this above the loop makes that true by
		 * construction rather than by five branches each remembering to.
		 */
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		var pending []Pending

		for _, call := range resp.ToolCalls {
			tool, ok := l.Registry.Get(call.Name)
			if !ok {
				// Hallucinated tool names are common with small models. Telling
				// the model rather than failing the turn lets it correct itself.
				messages = append(messages, toolResult(call, "No such tool: "+call.Name))

				continue
			}

			/*
			 * And refused as well as hidden.
			 *
			 * Hiding it from the list is what stops the model reaching for it;
			 * this is what stops it anyway. A model that saw the tool earlier
			 * in a conversation will name it again after the rule changes, and
			 * a guard that only removes the menu is a convention rather than a
			 * rule.
			 */
			if l.OffLimits != nil && l.OffLimits(call.Name) {
				messages = append(messages, toolResult(call,
					"That is not allowed under the current privacy setting."))

				continue
			}

			// Granted, and allowed on this project, or refused — for the same
			// reason as everything above: hidden is not enough.
			if known, found := l.Registry.Get(call.Name); found {
				if ok, why := tools.MayUse(known, brief.As, brief.Integrations); !ok {
					messages = append(messages, toolResult(call, "Refused: "+why+"."))

					continue
				}
			}

			/*
			 * And refused as well as hidden, for the same reason.
			 *
			 * Leaving a tool out of the list is what stops it being reached
			 * for; this is what stops it anyway. A model that saw the tool
			 * earlier in the same conversation will name it again — and an
			 * agent whose limits were only a shorter menu would be a
			 * convention rather than a rule.
			 */
			if !allowedBy(brief.Only, call.Name) {
				messages = append(messages, toolResult(call,
					"That is not one of the tools for this piece of work."))

				continue
			}

			/*
			 * And what is not this job's to decide, whatever else it may do.
			 *
			 * Hidden above and refused here, like everything else: a model
			 * that saw the tool in an earlier turn will name it again, and a
			 * limit that is only a shorter menu is a convention rather than a
			 * rule.
			 */
			if listed(brief.Never, call.Name) {
				messages = append(messages, toolResult(call,
					"That is the owner's own decision to make, not this job's. "+
						"Say what needs deciding and leave it to them."))

				continue
			}

			/*
			 * And nothing outside the project it belongs to, when it belongs
			 * to one. Refused rather than put to its owner: a turn working on
			 * a game has no business asking to write in their home folder.
			 */
			if why := outside(brief.Within, tool, call.Arguments); why != "" {
				refused := "Refused: " + why + ". Work on this project stays in its own folders."

				messages = append(messages, toolResult(call, refused))

				l.DB.AddMessage(conversationID, llm.RoleTool, "", "", "["+tool.Summarize(call.Arguments)+"]\n"+refused)

				steps = append(steps, Step{Tool: tool.Name(), Summary: tool.Summarize(call.Arguments),
					Asked: askedFor(call.Arguments), Failed: true, Result: refused})

				continue
			}

			summary := tool.Summarize(call.Arguments)

			/*
			 * A safe tool reaching for something protected stops and asks.
			 *
			 * It used to be refused outright, which was the wrong shape: this
			 * is the owner's machine and his files, and a program that answers
			 * "no" to its owner has decided something that was not its to
			 * decide. Nothing is out of reach now — the things that matter
			 * come to him first.
			 *
			 * It is also the better answer to the thing being defended
			 * against. A page the brain reads can carry text telling it to
			 * open a credentials file and post the contents somewhere, and the
			 * whole of that attack is that nobody sees it happen. Asking is
			 * precisely what breaks it.
			 */
			held, rule, ask := protect.InArguments(call.Arguments)

			if ask && tool.Risk() != tools.Mutating {
				summary = protect.Explain(summary, held, rule)
			}

			/*
			 * A question ends the turn here, before anything else happens.
			 *
			 * Checked before permission, before protection, before execution,
			 * because none of those apply: nothing is being done. The turn
			 * stops and the question goes to the person, and their next
			 * message is the answer.
			 *
			 * It has to end the turn rather than return a result. A model that
			 * asks a question and is handed control again answers it itself on
			 * the next step, which is exactly the guessing this exists to stop.
			 */
			if tool.Name() == askFirst {
				if q, ok := tools.ReadQuestion(call.Arguments); ok {
					l.Log.Info("asked rather than guessed", "question", q.Question)

					text := q.Text()

					l.DB.AddMessage(conversationID, llm.RoleAssistant, "", "", text)

					return Result{
						Reply:        text,
						ActionsTaken: actions,
						Steps:        steps,
						Asked:        true,
					}, nil
				}
			}

			/*
			 * Allowed, refused, or put in front of a person.
			 *
			 * This was two states — safe ran and mutating always stopped —
			 * with no way to say "yes, and stop asking me about this one". So
			 * somebody who used a capability daily either approved it by hand
			 * every time or did without it. See the permits package.
			 *
			 * A protection rule still forces the question whatever has been
			 * granted: those fire on the *arguments*, not the capability, and
			 * "you may write files" is not "you may write this file".
			 */
			changes := tool.Risk() == tools.Mutating

			/*
			 * How serious this call is: the tool's own weighing of what it was
			 * handed, and — for anything that changes something — never less
			 * than the work it belongs to.
			 */
			level := tools.LevelOf(tool, call.Arguments)

			if changes {
				level = risk.Max(level, brief.Risk)
			}

			answer := permits.Ask

			if l.MayI != nil {
				answer = l.MayI(brief.As, tool.Name(), changes, level)
			} else if !changes {
				answer = permits.Allow
			}

			if answer == permits.Refuse {
				l.Log.Info("refused by a standing decision", "tool", tool.Name())

				/*
				 * Told to the model, not only to the person.
				 *
				 * A refusal the model cannot see is a refusal it will make
				 * again on the next step, and then a third time, until the
				 * step limit — a turn that reads as the brain having hung. It
				 * has to come back as the result of the call, in words that
				 * say the decision is standing rather than that the tool is
				 * broken.
				 */
				refused := fmt.Sprintf(
					"Refused: %s is something the owner has told me never to do. "+
						"Do not try it again; say so and offer something else. "+
						"They can change it under Permissions.", tool.Name())

				messages = append(messages, toolResult(call, refused))

				l.DB.AddMessage(conversationID, llm.RoleTool, "", "",
					"["+summary+"]\n"+refused)

				continue
			}

			if ask && l.NothingAsks != nil && l.NothingAsks() {
				ask = false
			}

			/*
			 * And the few things that are always its owner's, whatever the
			 * setting: installing, switching an integration on, connecting an
			 * account, spending, hiring somebody for good. See tools.Consenting.
			 *
			 * After the setting rather than before, so that nothing above can
			 * clear it — and after refusals, so a refused tool stays refused.
			 */
			if why := tools.ConsentFor(tool, call.Arguments); why != "" {
				ask = true
				summary = "Always asked — " + why + ": " + summary
			}

			/*
			 * The level said in the question, when it is worth saying.
			 *
			 * High and critical only. Prefixing every approval with "medium"
			 * would be a word people learn to skip, and the point is the two
			 * that should not be skipped.
			 */
			asked := summary

			if level.AtLeast(risk.High) {
				asked = level.Title() + " risk — " + summary
			}

			if answer == permits.Ask || ask {
				summary = asked

				l.doing("waiting", "Waiting for you: "+summary)

				id, err := l.DB.RecordInvocation(conversationID, tool.Name(), string(call.Arguments), summary, string(tools.Mutating))
				if err != nil {
					return Result{}, err
				}

				pending = append(pending, Pending{ID: id, Tool: tool.Name(), Summary: summary})

				continue
			}

			l.usingTool(tool.Name(), summary)

			/*
			 * What it was actually asked to do, in its own words.
			 *
			 * The summary is written for a person and generalises — "Search
			 * the web" — while the arguments say which search, which file,
			 * which window. That difference is the whole value of watching:
			 * a tool called with the wrong argument and a tool that is merely
			 * slow look identical until you can see what it was handed.
			 */
			l.detail(askedFor(call.Arguments))

			started := time.Now()

			output, evidence, err := tools.Perform(Confined(ctx, brief.Within), tool, call.Arguments)

			step := Step{
				Tool:     tool.Name(),
				Summary:  summary,
				Asked:    askedFor(call.Arguments),
				Millis:   time.Since(started).Milliseconds(),
				Level:    level,
				Evidence: evidence,
			}

			if err != nil {
				output = "Error: " + err.Error()
				step.Failed = true
				step.Result = err.Error()

				l.detail("failed: " + truncate(err.Error(), 120))
			} else {
				actions = append(actions, summary)
				step.Result = truncate(output, 4000)

				// What came back and how long it took, so a tool that returned
				// nothing is distinguishable from one that returned plenty —
				// which is the difference between a wrong answer and no answer.
				l.detail(fmt.Sprintf("%s in %s",
					sizeOfResult(output), took(time.Since(started))))
			}

			step.Millis = time.Since(started).Milliseconds()
			steps = append(steps, step)

			messages = append(messages, toolResult(call, output))

			// Persisted so a later turn still knows what was looked up.
			l.DB.AddMessage(conversationID, llm.RoleTool, "", "",
				"["+summary+"]\n"+truncate(output, ToolOutputLimit))
		}

		if len(pending) > 0 {
			return l.awaitApproval(conversationID, resp, pending, actions, steps), nil
		}
	}

	return Result{
		Reply: fmt.Sprintf("I stopped after %d steps without reaching an answer. "+
			"Ask me to continue if that was too soon.", MaxSteps),
		ActionsTaken: actions,
		Steps:        steps,
		HitStepLimit: true,
	}, nil
}

// askFirst is the one tool the loop handles itself rather than executing.
// See tools/ask.go.
const askFirst = "ask_first"

// awaitApproval ends the turn with the actions queued and nothing done.
func (l *Loop) awaitApproval(conversationID int64, resp llm.Response, pending []Pending, actions []string, steps []Step) Result {
	var b strings.Builder

	if text := presentable(resp.Content); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	}

	if len(pending) == 1 {
		b.WriteString("This needs your approval before I can do it:\n")
	} else {
		b.WriteString("These need your approval before I can do them:\n")
	}

	for _, p := range pending {
		b.WriteString("  • " + p.Summary + "\n")
	}

	reply := strings.TrimSpace(b.String())

	l.DB.AddMessage(conversationID, llm.RoleAssistant, resp.Provider, resp.Model, reply)

	return Result{
		Reply:        reply,
		Provider:     resp.Provider,
		Model:        resp.Model,
		ActionsTaken: actions,
		Steps:        steps,
		Pending:      pending,
	}
}

// recoverToolCall reads a tool call the model wrote as text.
//
// Deliberately strict: the whole reply must be one JSON object naming a tool
// that actually exists. Anything looser would start treating prose about tools,
// or JSON quoted from a file, as an instruction to act.
/*
 * lineStarts is where in the text a line begins with an opening brace.
 *
 * The whole text counts as the first line, since a reply that opens with a
 * call has no newline before it.
 */
func lineStarts(text string) []int {
	var out []int

	if strings.HasPrefix(text, "{") {
		out = append(out, 0)
	}

	for at := 0; ; {
		next := strings.Index(text[at:], "\n{")

		if next < 0 {
			break
		}

		out = append(out, at+next+1)
		at += next + 2
	}

	return out
}

/*
 * balanced takes the first complete JSON object off the front of a string.
 *
 * Counting braces rather than handing the whole rest to the parser, because
 * the rest is usually a sentence — and a parser given an object followed by
 * prose reports a failure rather than the object it successfully read.
 *
 * Strings are tracked, so a brace inside one does not close the object. That
 * is not a hypothetical here: a call to write a file carries the file's
 * contents as an argument.
 */
func balanced(text string) (string, bool) {
	var (
		depth   int
		inside  bool
		escaped bool
	)

	for i, r := range text {
		switch {
		case escaped:
			escaped = false

		case r == '\\' && inside:
			escaped = true

		case r == '"':
			inside = !inside

		case inside:
			// Braces inside a string are content, not structure.

		case r == '{':
			depth++

		case r == '}':
			depth--

			if depth == 0 {
				return text[:i+1], true
			}
		}
	}

	return "", false
}

func (l *Loop) recoverToolCall(content string) (llm.ToolCall, bool) {
	for _, candidate := range jsonCandidates(content) {
		var probe struct {
			Name string `json:"name"`
			// Both spellings appear in the wild.
			Arguments json.RawMessage `json:"arguments"`
			Params    json.RawMessage `json:"parameters"`
		}

		if err := json.Unmarshal([]byte(candidate), &probe); err != nil {
			continue
		}

		if probe.Name == "" {
			continue
		}

		// The guard that makes looking inside prose safe at all: the object has
		// to name a tool this brain actually has. Quoted JSON from a file, or a
		// model describing a tool it imagined, does not.
		if _, known := l.Registry.Get(probe.Name); !known {
			continue
		}

		args := probe.Arguments
		if len(args) == 0 {
			args = probe.Params
		}

		if len(args) == 0 {
			args = json.RawMessage("{}")
		}

		l.Log.Info("recovered a tool call the model wrote as text", "tool", probe.Name)

		return llm.ToolCall{ID: "recovered", Name: probe.Name, Arguments: args}, true
	}

	return llm.ToolCall{}, false
}

/*
 * jsonCandidates pulls the places a tool call may legitimately be written.
 *
 * The whole reply, and the inside of any fenced code block. Nothing else — in
 * particular not every pair of braces in the text, which was tried and is
 * wrong: a model explaining itself writes things like
 *
 *     Try {"name": "read_file"} to see it.
 *
 * and treating that as an instruction to act is how an assistant starts doing
 * things nobody asked for. There is a test that says so.
 *
 * A fence, though, is the model formatting a call rather than talking about
 * one, and that is the case that mattered here. "Approve everything waiting"
 * did nothing for as long as it did because the model got it entirely right —
 * correct tool, correct arguments — and introduced it the way a person would:
 *
 *     Sure, I'll approve everything for you.
 *
 *     ```json
 *     {"name": "decide_waiting", "arguments": {"decision": "approve"}}
 *     ```
 *
 * One sentence of politeness in front, and the call was dropped. The brain then
 * said it would do the thing and did not do it, which is the worst outcome
 * available to it.
 */
/*
 * JSONObjects is every place in a reply that might be a JSON object.
 *
 * Exported for the planner, which has exactly the problem tool-call recovery
 * has and no reason to solve it twice: a small model asked for JSON writes a
 * sentence in front of it, or fences it, or wraps its working in <think> tags
 * and puts the object after. Two copies of that brace counter is how one of
 * them stops getting fixed.
 */
func JSONObjects(text string) []string { return jsonCandidates(text) }

func jsonCandidates(content string) []string {
	/*
	 * The model's deliberation comes off first.
	 *
	 * qwen3 and the other hybrid reasoning models wrap their working in <think>
	 * tags and then write the call after it. The whole reply therefore does not
	 * begin with a brace, so it was never read as a call at all — and the brain
	 * told its owner "the model returned a tool call as text rather than making
	 * one", which was true and entirely self-inflicted.
	 */
	text := strings.TrimSpace(withoutThinking(content))

	if text == "" {
		return nil
	}

	var out []string

	// Fenced blocks first: a fence is the model formatting a call, which is a
	// stronger signal than the shape of the reply as a whole.
	rest := text

	for {
		open := strings.Index(rest, "```")
		if open < 0 {
			break
		}

		body := rest[open+3:]

		// The fence may be tagged, as in ```json.
		if i := strings.IndexByte(body, '\n'); i >= 0 {
			body = body[i+1:]
		}

		close := strings.Index(body, "```")
		if close < 0 {
			out = append(out, strings.TrimSpace(body))

			break
		}

		out = append(out, strings.TrimSpace(body[:close]))
		rest = body[close+3:]
	}

	/*
	 * A call written on its own line at the end, after a sentence.
	 *
	 * This is the shape that produced "To see what is waiting for you, please
	 * give me the command:" followed by nothing at all. The model announced
	 * the call and then wrote it out bare, with no fence — so the display
	 * stripped it, because presentable already knew to look there, and
	 * recovery did not, because it did not. Two functions disagreeing about
	 * where a call can be is exactly how one gets thrown away.
	 *
	 * Only at the very end, and only as its own object. That is what keeps
	 * "try {"name": "read_file"} to see it" from being an instruction: it is
	 * in the middle of a sentence with words after it.
	 */
	if brace := strings.LastIndex(text, "\n{"); brace > 0 {
		out = append(out, strings.TrimSpace(text[brace+1:]))
	}

	/*
	 * And an object that begins a line, wherever that line is.
	 *
	 * The two cases above are a call at the end and a call in a fence. The one
	 * that was still being lost is a call written first, with a pleasantry
	 * after it — which is what a small model does when it has been told to be
	 * conversational and to use tools, and it produced "the model returned a
	 * tool call as text rather than making one" while the call sat there in
	 * plain sight.
	 *
	 * Only at the start of a line, and only the balanced object. That is what
	 * keeps `try {"name": "read_file"} to see it` from being an instruction:
	 * it sits in the middle of a sentence, with words either side of it, and a
	 * model writing a call does not put it there.
	 */
	for _, at := range lineStarts(text) {
		if object, ok := balanced(text[at:]); ok {
			out = append(out, object)
		}
	}

	// And the reply itself, for a model that answered with nothing else.
	out = append(out, text)

	return out
}

// lastUserMessage is what the person actually said this turn, which is what
// the choice of model is made from — not the whole conversation, which carries
// every earlier subject with it.
func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}

	return ""
}

/*
 * TalkAloud is something that speaks text as it arrives.
 *
 * An interface so the agent does not depend on the speech package, which brings
 * a process, a socket and an audio device with it — none of which belongs in
 * the test for a conversation loop.
 */
type TalkAloud interface {
	Write(text string)
	Close()
	Started() bool
}

// streamAloud runs one model call, speaking it as it is written.
func (l *Loop) streamAloud(
	ctx context.Context, provider llm.Provider, request llm.Request,
) (llm.Response, error) {
	streamer, ok := provider.(llm.Streamer)
	if !ok {
		return provider.Chat(ctx, request)
	}

	/*
	 * A voice if there is one, and nothing if there is not.
	 *
	 * A typed turn still streams — the page shows the answer being written —
	 * it simply has nobody to say it to. Handling that here rather than at the
	 * call site keeps one path through the streaming, which is the part that
	 * has to be right.
	 */
	voice := TalkAloud(silentVoice{})

	if l.Aloud != nil {
		voice = l.Aloud(ctx)
	}

	defer voice.Close()

	l.doing("answering", "Answering")

	/*
	 * Nothing is spoken until it is clear this is an answer and not a call.
	 *
	 * A model about to call a tool opens with a brace or a fence, so a short
	 * look at the first characters separates the two — and half a tool call
	 * read out loud is a string of punctuation, which is the one outcome worse
	 * than silence.
	 */
	var (
		opening strings.Builder
		decided bool
		speak   bool
	)

	/*
	 * Everything written so far, for the page to show as it grows.
	 *
	 * The reply used to reach the screen in one piece at the end, so a turn
	 * taking a minute showed nothing for a minute — and on a spoken turn the
	 * assistant talked the whole way through while the transcript beside it
	 * sat empty, which reads as a program that has stopped.
	 *
	 * Kept separately from the voice's copy because the two want different
	 * things: the voice takes whole sentences, since half a sentence read
	 * aloud is worse than waiting, and the page takes whatever there is, since
	 * half a sentence on screen is a sentence being written.
	 */
	var written strings.Builder

	resp, err := streamer.ChatStream(ctx, request, func(text string) {
		// Cut in on: the rest of this answer is not wanted.
		if l.Interrupted != nil && l.Interrupted() {
			return
		}

		if !decided {
			opening.WriteString(text)

			verdict, settled := isProse(opening.String())
			if !settled {
				return
			}

			decided, speak = true, verdict

			if speak {
				voice.Write(opening.String())

				written.WriteString(opening.String())
				l.writing(written.String())
			}

			return
		}

		if speak {
			voice.Write(text)

			// Shown as well as said. A tool call is deliberately not shown:
			// it is machinery, and half of one on screen is punctuation.
			written.WriteString(text)
			l.writing(written.String())
		}
	})
	if err != nil {
		return resp, err
	}

	// Everything arrived before the question was settled: short answers do
	// that, and they are still answers.
	if !decided {
		if verdict, _ := isProse(opening.String()); verdict {
			voice.Write(opening.String())
		}
	}

	// Recorded on the response so the caller knows not to say it all again.
	resp.Spoken = voice.Started()

	/*
	 * Cut in on, so the answer is what was actually delivered.
	 *
	 * Stopping the voice used to throw the rest away and leave the full reply
	 * in the record, so the conversation held a paragraph its owner never
	 * heard — and the next turn was answered as though they had. Then the
	 * follow-up made no sense to either of them: they were replying to the
	 * first sentence and it was continuing from the fifth.
	 *
	 * What was said is what happened. Keeping the delivered part, and marking
	 * it as cut off, is what lets the next turn pick up from the right place
	 * rather than from a version of the conversation only one of them was in.
	 */
	if l.Interrupted != nil && l.Interrupted() {
		if said := strings.TrimSpace(written.String()); said != "" {
			resp.Content = said + " …"
			resp.CutOff = true
		}
	}

	return resp, nil
}

/*
 * isProse decides whether what has begun to arrive is an answer or a tool call.
 *
 * Returns whether it is prose, and whether there is yet enough to say. A model
 * writing a call opens with a brace or a fence; one answering opens with a
 * word. A dozen characters settles it either way, which at ten tokens a second
 * is about a second of held breath.
 */
func isProse(sofar string) (prose, settled bool) {
	trimmed := strings.TrimLeft(sofar, " \t\r\n")

	if trimmed == "" {
		// Only whitespace so far, and whitespace decides nothing.
		return false, len(sofar) > 40
	}

	switch trimmed[0] {
	case '{', '[':
		return false, true
	case '`':
		return false, true
	}

	// Enough of a word to be sure it is one.
	return true, len(trimmed) >= 12
}

/*
 * keepThePromise is what the model is told when it undertook to do something
 * and then did not.
 *
 * Short and blunt. A long explanation is more prompt to read on a machine where
 * reading the prompt is most of the wait, and a small model follows a blunt
 * instruction better than a reasoned one.
 */
const keepThePromise = "You just said you would do something and did not do " +
	"it. Saying it is not doing it. Call the tool now. If no tool can do what " +
	"you promised, say so plainly instead."

/*
 * promised reports a reply that undertakes to act without acting.
 *
 * Deliberately narrow, and anchored at the start of a sentence. "I will" in the
 * middle of an explanation is discussion; at the front of the answer it is an
 * undertaking. Getting this wrong in the loose direction costs a whole extra
 * turn on a machine where a turn is minutes, so it only fires on the shapes
 * that have actually been seen doing it.
 */
func promised(content string) bool {
	text := strings.ToLower(strings.TrimSpace(withoutThinking(content)))

	if text == "" {
		return false
	}

	// The first sentence is where an undertaking lives.
	if cut := strings.IndexAny(text, ".!?\n"); cut > 0 {
		text = text[:cut]
	}

	/*
	 * At the start only.
	 *
	 * Matching these anywhere in the sentence was the first attempt, and it
	 * took "That depends on what you mean — I will need more detail" for an
	 * undertaking. That is discussion, and pressing on it wastes a whole turn
	 * of a machine where a turn is minutes.
	 */
	for _, opening := range []string{
		"i will ", "i'll ", "i am going to ", "i'm going to ", "let me ",
		"understood, i will", "understood, i'll", "understood. i will",
		"sure, i'll", "sure, i will", "okay, i'll", "of course, i'll",
	} {
		if strings.HasPrefix(text, opening) {
			return true
		}
	}

	/*
	 * And the present continuous, which is the commoner shape by far.
	 *
	 * "I'm checking what is waiting for you. One moment." is not a plan to
	 * act, it is a claim to be acting — and it is worse than "I will", because
	 * it describes something already happening that is not. Said to somebody
	 * waiting, it is indistinguishable from work being done, so they wait.
	 *
	 * Matched as "I'm" plus a verb of doing rather than as "I'm" plus
	 * anything, because "I'm not sure" and "I'm afraid that is not something I
	 * can do" are the honest answers this must never press on.
	 */
	for _, opening := range []string{"i'm ", "i am ", "im "} {
		rest, is := strings.CutPrefix(text, opening)

		if !is {
			continue
		}

		for _, doing := range []string{
			"checking", "looking", "reading", "searching", "finding",
			"remembering", "recalling", "processing", "working on",
			"gathering", "fetching", "getting", "opening", "running",
			"writing", "learning", "scanning", "starting", "going through",
			"pulling", "reviewing", "retrieving",
		} {
			if strings.HasPrefix(rest, doing) {
				return true
			}
		}
	}

	// "Understood." on its own, which is the shortest form of the same thing.
	return text == "understood" || text == "sure" || text == "of course"
}

// specs describes the tools to the model, in the registry's stable order.
//
// The schemas are compacted and the result cached. They are written across
// several indented lines for people to read, and every tab and newline in them
// is a token the model is charged for on every single call — measured here at
// 336 tokens of schema, of which roughly half was whitespace. On a CPU
// processing ten tokens a second that is real time spent transmitting
// indentation.
func (l *Loop) specs() []llm.ToolSpec {
	l.specsMu.Lock()

	if version := l.Registry.Version(); l.cachedSpecs == nil || version != l.specsFor {
		all := l.Registry.All()
		out := make([]llm.ToolSpec, 0, len(all))

		for _, t := range all {
			out = append(out, llm.ToolSpec{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  compactJSON(t.Parameters()),
			})
		}

		l.cachedSpecs, l.specsFor = out, version
	}

	cached := l.cachedSpecs

	l.specsMu.Unlock()

	if l.OffLimits == nil {
		return cached
	}

	/*
	 * The forbidden ones filtered out of the cached list, not out of the
	 * cache.
	 *
	 * Privacy can change between one turn and the next, so this cannot be
	 * decided once — but building every schema again on every turn would
	 * throw away the caching that exists because whitespace in a schema is
	 * tokens the model is charged for.
	 */
	out := make([]llm.ToolSpec, 0, len(cached))

	for _, spec := range cached {
		if l.OffLimits(spec.Name) {
			continue
		}

		out = append(out, spec)
	}

	return out
}

// compactJSON strips formatting whitespace, leaving the schema unchanged.
func compactJSON(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer

	if err := json.Compact(&buf, raw); err != nil {
		// Unparseable here means the tool declared invalid JSON, which its own
		// test catches. Send it as written rather than dropping the tool.
		return raw
	}

	return json.RawMessage(buf.Bytes())
}

func toolResult(call llm.ToolCall, output string) llm.Message {
	return llm.Message{
		Role:       llm.RoleTool,
		Name:       call.Name,
		ToolCallID: call.ID,
		Content:    truncate(output, ToolOutputLimit),
	}
}

// presentable strips raw tool-call JSON that small models emit as prose.
//
// A model that has already made a structured tool call will often also print
// the call as text. Showing that to a person is showing them the machinery.
func presentable(content string) string {
	text := strings.TrimSpace(withoutThinking(content))

	if text == "" {
		return ""
	}

	// A reply that is nothing but a JSON object with a "name" field is a tool
	// call leaking into prose, not an answer.
	if looksLikeACall(text) {
		return ""
	}

	/*
	 * And the same thing with a sentence in front of it.
	 *
	 * Small models like to announce the call and then print it, which leaves
	 * the owner reading "Understood." followed by a wall of JSON braces. The
	 * announcement is the answer; the JSON is plumbing that escaped, and it is
	 * shown on screen and read aloud by the voice if it is left in.
	 */
	if fence := strings.Index(text, "```"); fence > 0 && looksLikeACall(text[fence:]) {
		return strings.TrimSpace(text[:fence])
	}

	if brace := strings.LastIndex(text, "\n{"); brace > 0 && looksLikeACall(text[brace+1:]) {
		return strings.TrimSpace(text[:brace])
	}

	return text
}

/*
 * withoutThinking removes a reasoning model's working.
 *
 * Models like deepseek-r1 narrate their reasoning inside <think> tags before
 * answering. It is genuinely useful and it is not the answer: shown, it buries
 * the reply under a page of deliberation, and spoken, the voice reads several
 * minutes of the model talking to itself before it gets to the point.
 */
func withoutThinking(text string) string {
	for {
		open := strings.Index(text, "<think>")
		if open < 0 {
			break
		}

		close := strings.Index(text[open:], "</think>")
		if close < 0 {
			// Still thinking, and nothing after it yet.
			return strings.TrimSpace(text[:open])
		}

		text = text[:open] + text[open+close+len("</think>"):]
	}

	return strings.TrimSpace(text)
}

// looksLikeACall reports text that is a written-out tool call and nothing else.
func looksLikeACall(text string) bool {
	text = strings.TrimSpace(text)

	// Unwrap a fence, which is how a model most often writes one.
	if strings.HasPrefix(text, "```") {
		body := text[3:]

		if i := strings.IndexByte(body, '\n'); i >= 0 {
			body = body[i+1:]
		}

		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(body), "```"))
	}

	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return false
	}

	var probe map[string]any

	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return false
	}

	_, hasName := probe["name"]

	return hasName
}

func truncate(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + fmt.Sprintf("\n\n[truncated at %d characters]", n)
}

/*
 * askedFor renders a tool's arguments as one readable line.
 *
 * The raw JSON is the honest thing to show and the wrong one: it is mostly
 * punctuation, and the panel it goes in is a column a hundred and fifty pixels
 * wide. The values are what carry meaning — the query, the path, the address —
 * so the keys are dropped and the values kept.
 */
func askedFor(raw json.RawMessage) string {
	var args map[string]any

	if err := json.Unmarshal(raw, &args); err != nil || len(args) == 0 {
		return ""
	}

	// Sorted, so the same call reads the same way every time. Map order in Go
	// is deliberately random, and a line that reshuffles itself between polls
	// is unreadable.
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var parts []string

	for _, k := range keys {
		text := strings.TrimSpace(fmt.Sprint(args[k]))
		if text == "" || text == "<nil>" {
			continue
		}

		parts = append(parts, truncate(text, 80))
	}

	if len(parts) == 0 {
		return ""
	}

	return truncate(strings.Join(parts, " · "), 160)
}

// sizeOfResult describes what a tool returned, in whatever unit fits.
func sizeOfResult(output string) string {
	switch trimmed := strings.TrimSpace(output); {
	case trimmed == "":
		return "nothing back"
	case len(trimmed) < 400:
		// Short enough to show, and a short result is usually the interesting
		// one: a count, a yes, a single line.
		return truncate(strings.ReplaceAll(trimmed, "\n", " "), 120)
	default:
		return fmt.Sprintf("%d lines back", strings.Count(trimmed, "\n")+1)
	}
}

// took renders a duration the way somebody reads it aloud.
func took(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// sizeOfPrompt describes how much the model has to read, in words rather than
// tokens, because words are a unit somebody can picture.
func sizeOfPrompt(messages []llm.Message) string {
	var words int

	for _, m := range messages {
		words += len(strings.Fields(m.Content))
	}

	switch {
	case words < 1000:
		return fmt.Sprintf("%d words", words)
	default:
		return fmt.Sprintf("%.1fk words", float64(words)/1000)
	}
}

/*
 * rateOfReply says how quickly the model produced its answer.
 *
 * Counted in words, and deliberately not called tokens. It is the same
 * measurement to within a constant, and "twelve words a second" means
 * something to somebody watching a sentence appear while "sixteen tokens a
 * second" does not.
 */
func rateOfReply(content string, elapsed time.Duration) string {
	words := len(strings.Fields(content))

	if words == 0 || elapsed <= 0 {
		return took(elapsed)
	}

	/*
	 * A rate needs enough words to mean anything.
	 *
	 * Three words over a minute is not a slow model, it is a model that
	 * decided to call a tool and said almost nothing on the way — and
	 * reporting "0.0 a second" for it describes a machine that has stopped.
	 * Below a sentence or two the duration alone is the honest number.
	 */
	if words < 20 {
		return fmt.Sprintf("%d words in %s", words, took(elapsed))
	}

	return fmt.Sprintf("%d words in %s · %.1f a second",
		words, took(elapsed), float64(words)/elapsed.Seconds())
}

// silentVoice is what a typed turn speaks through: nothing at all.
//
// Started reports false, so the caller is told the answer was not said aloud
// and shows it in full rather than assuming it has already been heard.
type silentVoice struct{}

func (silentVoice) Write(string)  {}
func (silentVoice) Close()        {}
func (silentVoice) Started() bool { return false }

/*
 * LongEnoughToBeARepeat is the shortest answer worth calling a repetition.
 *
 * Short replies repeat legitimately and constantly — "yes", "done", "not
 * yet" — and treating those as a fault would have the brain lecture somebody
 * for agreeing with them twice.
 */
const LongEnoughToBeARepeat = 60

/*
 * repeatOf reports that this answer is word for word the last one given.
 *
 * Compared against the conversation as it was sent to the model rather than
 * against the database, because that is the same list the model itself just
 * read: if it is in there, the model had it in front of it and produced it
 * again anyway.
 */
func repeatOf(reply string, messages []llm.Message) bool {
	if len([]rune(reply)) < LongEnoughToBeARepeat {
		return false
	}

	want := plainly(reply)

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != llm.RoleAssistant {
			continue
		}

		return plainly(messages[i].Content) == want
	}

	return false
}

// plainly reduces an answer to its words, so that punctuation and spacing do
// not make two identical sentences look different.
func plainly(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

/*
 * What is happening, for anything watching — unless nobody is.
 *
 * Every one of these is progress.X guarded by Quietly. They exist because
 * progress holds one current step for the whole program: a task working in the
 * background and a person waiting on an answer would be writing to the same
 * line, and the person would lose. Two background writers already clear a
 * foreground turn this way, in the learning worker and the place watcher; this
 * is at least not a third.
 */
func (l *Loop) began() {
	if !l.Quietly {
		progress.Begin()
	}
}

func (l *Loop) finished() {
	if !l.Quietly {
		progress.Done()
	}
}

func (l *Loop) round(n int) {
	if !l.Quietly {
		progress.Round(n)
	}
}

func (l *Loop) doing(kind, note string) {
	if !l.Quietly {
		progress.Set(kind, note)
	}
}

func (l *Loop) usingTool(name, note string) {
	if !l.Quietly {
		progress.SetTool(name, note)
	}
}

func (l *Loop) usingModel(name string) {
	if !l.Quietly {
		progress.UsingModel(name)
	}
}

func (l *Loop) detail(line string) {
	if !l.Quietly {
		progress.Detail(line)
	}
}

func (l *Loop) writing(text string) {
	if !l.Quietly {
		progress.Writing(text)
	}
}

func (l *Loop) mark() int64 {
	if l.Quietly {
		return 0
	}

	return progress.Mark()
}

func (l *Loop) detailOn(mark int64, line string) {
	if !l.Quietly {
		progress.DetailOn(mark, line)
	}
}

// cuesFor is the words a tool said mean it is worth offering, or none. Only
// skills say anything here; see tools.Cued.
/*
 * Have reports whether something a tool needs is on this machine.
 *
 * Wired to the one reading of the machine. Nil means offer everything, which
 * is what a caller that knows nothing about the machine should do: a tool
 * withheld because nobody asked is worse than one that tries and explains
 * itself.
 */
// (field on Loop; see the struct.)

// needsFor is what a tool cannot work without, from the registry.
func (l *Loop) needsFor(name string) []string {
	if l.Registry == nil {
		return nil
	}

	return l.Registry.Needing(name)
}

// namesOf is the names of these tools, for remembering what was offered.
func namesOf(specs []llm.ToolSpec) []string {
	out := make([]string, 0, len(specs))

	for _, spec := range specs {
		out = append(out, spec.Name)
	}

	return out
}

func (l *Loop) cuesFor(name string) []string {
	tool, ok := l.Registry.Get(name)
	if !ok {
		return nil
	}

	if cued, ok := tool.(tools.Cued); ok {
		return cued.Cues()
	}

	return nil
}

// granted drops the tools this agent has not been granted, or that the work's
// project does not allow. See tools.MayUse.
func (l *Loop) granted(specs []llm.ToolSpec, brief Brief) []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(specs))

	for _, spec := range specs {
		if t, ok := l.Registry.Get(spec.Name); ok {
			if allowed, _ := tools.MayUse(t, brief.As, brief.Integrations); !allowed {
				continue
			}
		}

		out = append(out, spec)
	}

	return out
}

// onlyThese narrows a list of schemas to the ones a turn may use. An empty
// list means no narrowing, which is the ordinary case.
func onlyThese(specs []llm.ToolSpec, only []string) []llm.ToolSpec {
	if len(only) == 0 {
		return specs
	}

	out := make([]llm.ToolSpec, 0, len(only))

	for _, spec := range specs {
		if allowedBy(only, spec.Name) {
			out = append(out, spec)
		}
	}

	return out
}

func allowedBy(only []string, name string) bool { return tools.Allowed(only, name) }

// without drops the tools a turn may not use. Separate from onlyThese because
// an empty Never means nothing is dropped, where an empty Only means nothing
// is filtered — the same value meaning opposite things.
func without(specs []llm.ToolSpec, never []string) []llm.ToolSpec {
	if len(never) == 0 {
		return specs
	}

	out := make([]llm.ToolSpec, 0, len(specs))

	for _, spec := range specs {
		if !listed(never, spec.Name) {
			out = append(out, spec)
		}
	}

	return out
}

func listed(names []string, name string) bool { return tools.Listed(names, name) }
