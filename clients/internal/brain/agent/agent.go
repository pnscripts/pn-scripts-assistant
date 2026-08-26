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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"pn-brain/internal/brain/llm"
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
	DB       *store.DB
	Registry *tools.Registry
	Log      *slog.Logger
}

// Pending is an action waiting for approval.
type Pending struct {
	ID      int64  `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
}

// Result is the outcome of one turn.
type Result struct {
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
	specs := l.specs()

	var actions []string

	for step := 0; step < MaxSteps; step++ {
		resp, err := provider.Chat(ctx, llm.Request{Messages: messages, Tools: specs})
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
				Reply:        reply,
				Provider:     resp.Provider,
				Model:        resp.Model,
				ActionsTaken: actions,
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
				id, err := l.DB.RecordInvocation(conversationID, tool.Name(), string(call.Arguments), summary, string(tools.Mutating))
				if err != nil {
					return Result{}, err
				}

				pending = append(pending, Pending{ID: id, Tool: tool.Name(), Summary: summary})

				continue
			}

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
	text := strings.TrimSpace(content)

	// Models often fence it even when asked not to.
	if strings.HasPrefix(text, "```") {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}

		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
	}

	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return llm.ToolCall{}, false
	}

	var probe struct {
		Name string `json:"name"`
		// Both spellings appear in the wild.
		Arguments json.RawMessage `json:"arguments"`
		Params    json.RawMessage `json:"parameters"`
	}

	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return llm.ToolCall{}, false
	}

	if probe.Name == "" {
		return llm.ToolCall{}, false
	}

	if _, known := l.Registry.Get(probe.Name); !known {
		return llm.ToolCall{}, false
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

// specs describes the tools to the model, in the registry's stable order.
func (l *Loop) specs() []llm.ToolSpec {
	all := l.Registry.All()
	out := make([]llm.ToolSpec, 0, len(all))

	for _, t := range all {
		out = append(out, llm.ToolSpec{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}

	return out
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
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		var probe map[string]any

		if err := json.Unmarshal([]byte(text), &probe); err == nil {
			if _, hasName := probe["name"]; hasName {
				return ""
			}
		}
	}

	return text
}

func truncate(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + fmt.Sprintf("\n\n[truncated at %d characters]", n)
}
