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

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
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
	 * Model chooses which model answers a turn, or is nil to leave it alone.
	 *
	 * Set by the brain, which knows what is installed. It is asked once per
	 * turn rather than once per step, so a turn that uses a tool does not
	 * change model halfway through and forget how it was going to finish.
	 */
	Model func(message string) llm.Choice

	DB       *store.DB
	Registry *tools.Registry
	Log      *slog.Logger

	specsOnce   sync.Once
	cachedSpecs []llm.ToolSpec
}

// Pending is an action waiting for approval.
type Pending struct {
	ID      int64  `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// Result is the outcome of one turn.
type Result struct {
	// AlreadySpoken is true when the answer was said aloud as it was written,
	// so the caller must not say the whole thing again.
	AlreadySpoken bool

	Reply        string
	Provider     string
	Model        string
	ActionsTaken []string
	Pending      []Pending
	HitStepLimit bool
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
	var actions []string

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

	if l.Model != nil {
		choice := l.Model(lastUserMessage(messages))
		model = choice.Model
		offerTools = withTools && choice.Tools

		// Held to its job for this turn only, where a smaller model needs it.
		if choice.Guidance != "" {
			messages = append(messages,
				llm.Message{Role: llm.RoleSystem, Content: choice.Guidance})
		}

		progress.UsingModel(choice.Model)

		l.Log.Info("model for this turn",
			"model", choice.Model, "why", choice.Why, "tools", offerTools)
	}

	var specs []llm.ToolSpec

	if offerTools {
		/*
		 * Narrowed to what this request could plausibly need.
		 *
		 * How well a model chooses among tools falls off sharply with its
		 * size, and thirty-one is a lot to choose between. Asked what it
		 * thought of a website, the small model here called list_models —
		 * not a near miss — and before that, look_at_screen, which spent
		 * seven minutes reading a desktop that could not have held the
		 * answer.
		 */
		specs = relevant(l.specs(), lastUserMessage(messages))
	}

	// Whether the model has already been asked to keep a promise this turn.
	pressed := false

	// What is happening, for anything watching. A turn that uses a tool is
	// several model calls with work between them, and on this machine that can
	// run to minutes — long enough that an interface saying nothing is
	// indistinguishable from one that has crashed.
	progress.Begin()
	defer progress.Done()

	for step := 0; step < MaxSteps; step++ {
		progress.Round(step + 1)
		progress.Set("thinking", "Thinking")

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
		progress.Detail(fmt.Sprintf("%s · reading %s", model, sizeOfPrompt(messages)))

		// Which step this is, so what the model produced is reported against
		// the step that ran it. The call outlives its own step — it starts
		// under "Thinking" and returns after answering and speaking — so
		// attaching to whatever is current put a model's running time under
		// "Listening", which had not been running at all.
		thinkingStep := progress.Mark()
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
		progress.DetailOn(thinkingStep, rateOfReply(resp.Content, time.Since(thought)))

		if len(resp.ToolCalls) == 0 {
			if recovered, ok := l.recoverToolCall(resp.Content); ok {
				resp.ToolCalls = []llm.ToolCall{recovered}
				resp.Content = ""
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
				progress.Set("thinking", "Doing it")

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

			return Result{
				Reply:         reply,
				Provider:      resp.Provider,
				Model:         resp.Model,
				ActionsTaken:  actions,
				AlreadySpoken: resp.Spoken,
			}, nil
		}

		var pending []Pending

		for _, call := range resp.ToolCalls {
			tool, ok := l.Registry.Get(call.Name)
			if !ok {
				// Hallucinated tool names are common with small models. Telling
				// the model rather than failing the turn lets it correct itself.
				messages = append(messages, toolResult(call, "No such tool: "+call.Name))

				continue
			}

			summary := tool.Summarize(call.Arguments)

			// A Mutating tool is recorded and the turn stops. It is not run
			// here under any circumstances.
			if tool.Risk() == tools.Mutating {
				progress.Set("waiting", "Waiting for you: "+summary)

				id, err := l.DB.RecordInvocation(conversationID, tool.Name(), string(call.Arguments), summary, string(tools.Mutating))
				if err != nil {
					return Result{}, err
				}

				pending = append(pending, Pending{ID: id, Tool: tool.Name(), Summary: summary})

				continue
			}

			progress.SetTool(tool.Name(), summary)

			/*
			 * What it was actually asked to do, in its own words.
			 *
			 * The summary is written for a person and generalises — "Search
			 * the web" — while the arguments say which search, which file,
			 * which window. That difference is the whole value of watching:
			 * a tool called with the wrong argument and a tool that is merely
			 * slow look identical until you can see what it was handed.
			 */
			progress.Detail(askedFor(call.Arguments))

			started := time.Now()

			output, err := tool.Execute(ctx, call.Arguments)
			if err != nil {
				output = "Error: " + err.Error()

				progress.Detail("failed: " + truncate(err.Error(), 120))
			} else {
				actions = append(actions, summary)

				// What came back and how long it took, so a tool that returned
				// nothing is distinguishable from one that returned plenty —
				// which is the difference between a wrong answer and no answer.
				progress.Detail(fmt.Sprintf("%s in %s",
					sizeOfResult(output), took(time.Since(started))))
			}

			messages = append(messages, toolResult(call, output))

			// Persisted so a later turn still knows what was looked up.
			l.DB.AddMessage(conversationID, llm.RoleTool, "", "",
				"["+summary+"]\n"+truncate(output, ToolOutputLimit))
		}

		if len(pending) > 0 {
			return l.awaitApproval(conversationID, resp, pending, actions), nil
		}
	}

	return Result{
		Reply: fmt.Sprintf("I stopped after %d steps without reaching an answer. "+
			"Ask me to continue if that was too soon.", MaxSteps),
		ActionsTaken: actions,
		HitStepLimit: true,
	}, nil
}

// awaitApproval ends the turn with the actions queued and nothing done.
func (l *Loop) awaitApproval(conversationID int64, resp llm.Response, pending []Pending, actions []string) Result {
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
		Pending:      pending,
	}
}

// recoverToolCall reads a tool call the model wrote as text.
//
// Deliberately strict: the whole reply must be one JSON object naming a tool
// that actually exists. Anything looser would start treating prose about tools,
// or JSON quoted from a file, as an instruction to act.
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

	progress.Set("answering", "Answering")

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
				progress.Writing(written.String())
			}

			return
		}

		if speak {
			voice.Write(text)

			// Shown as well as said. A tool call is deliberately not shown:
			// it is machinery, and half of one on screen is punctuation.
			written.WriteString(text)
			progress.Writing(written.String())
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
	l.specsOnce.Do(func() {
		all := l.Registry.All()
		out := make([]llm.ToolSpec, 0, len(all))

		for _, t := range all {
			out = append(out, llm.ToolSpec{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  compactJSON(t.Parameters()),
			})
		}

		l.cachedSpecs = out
	})

	return l.cachedSpecs
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
