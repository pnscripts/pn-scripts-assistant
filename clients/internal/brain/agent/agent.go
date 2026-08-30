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
	"strings"
	"sync"

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

		l.Log.Info("model for this turn",
			"model", choice.Model, "why", choice.Why, "tools", offerTools)
	}

	var specs []llm.ToolSpec

	if offerTools {
		specs = l.specs()
	}

	// What is happening, for anything watching. A turn that uses a tool is
	// several model calls with work between them, and on this machine that can
	// run to minutes — long enough that an interface saying nothing is
	// indistinguishable from one that has crashed.
	progress.Begin()
	defer progress.Done()

	for step := 0; step < MaxSteps; step++ {
		progress.Round(step + 1)
		progress.Set("thinking", "Thinking")

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

		if l.Aloud != nil && len(specs) == 0 {
			resp, err = l.streamAloud(ctx, provider, request)
		} else {
			resp, err = provider.Chat(ctx, request)
		}
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
		if len(resp.ToolCalls) == 0 {
			if recovered, ok := l.recoverToolCall(resp.Content); ok {
				resp.ToolCalls = []llm.ToolCall{recovered}
				resp.Content = ""
			}
		}

		if len(resp.ToolCalls) == 0 {
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

			progress.Set("tool", summary)

			output, err := tool.Execute(ctx, call.Arguments)
			if err != nil {
				output = "Error: " + err.Error()
			} else {
				actions = append(actions, summary)
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
	text := strings.TrimSpace(content)

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

	voice := l.Aloud(ctx)
	defer voice.Close()

	progress.Set("answering", "Answering")

	resp, err := streamer.ChatStream(ctx, request, voice.Write)
	if err != nil {
		return resp, err
	}

	// Recorded on the response so the caller knows not to say it all again.
	resp.Spoken = voice.Started()

	return resp, nil
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
	text := strings.TrimSpace(content)

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
