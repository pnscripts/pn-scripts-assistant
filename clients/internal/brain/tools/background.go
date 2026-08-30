package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

/*
 * Work that outlives the sentence asking for it.
 *
 * Everything else here happens inside a turn: somebody asks, a tool runs, an
 * answer comes back, and nothing else can be said in between. That is right for
 * "what is waiting" and wrong for "read through that folder" — which is minutes
 * on this machine, during which its owner sits unable to ask anything else.
 *
 * These put the work behind the conversation instead. The turn ends at once,
 * the person carries on talking about whatever they like, and the answer is
 * announced when it arrives.
 */

// Background is what these tools need to run work behind the conversation.
type Background interface {
	Start(what string, work func(context.Context) (string, error)) (int64, error)
	List() []BackgroundJob
	Stop(id int64) error
}

// BackgroundJob is one piece of work, as a model should see it.
type BackgroundJob struct {
	ID     int64
	What   string
	State  string
	Took   time.Duration
	Result string
	Err    string
}

// InBackground runs another tool without waiting for it.
type InBackground struct {
	Jobs Background

	/*
	 * Registry is looked up when the tool runs, not when it is built.
	 *
	 * This tool lives in the registry it needs to read, so at the moment it is
	 * constructed there is nothing to point at yet. Taking the pointer then
	 * gave a nil one and a crash on the first call — a function resolves it
	 * later, by which time the registry exists.
	 */
	Registry func() *Registry
}

func (InBackground) Name() string { return "do_in_background" }

func (InBackground) Description() string {
	return "Run something that will take a while without making the owner wait " +
		"for it — reading a folder, a long command, a big search. Say what you " +
		"are starting and carry on talking; the answer is announced when it is " +
		"ready. Use only for work that takes minutes, never for a question that " +
		"can be answered now."
}

func (InBackground) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tool": {"type": "string", "description": "The name of the tool to run"},
			"arguments": {"type": "object", "description": "The arguments for that tool"},
			"what": {"type": "string", "description": "What this is, in the owner's words, to say back when it finishes"}
		},
		"required": ["tool", "arguments", "what"]
	}`)
}

/*
 * Safe in itself, and honest about it.
 *
 * Starting something is not doing it: the tool underneath keeps whatever risk
 * it has, and one that needs approval still gets it before anything happens.
 * Backgrounding is not a way around the gate and must never become one.
 */
func (InBackground) Risk() Risk { return Safe }

func (t InBackground) Summarize(raw json.RawMessage) string {
	var a backgroundArgs

	_ = json.Unmarshal(raw, &a)

	return "Start, without waiting: " + a.What
}

type backgroundArgs struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	What      string          `json:"what"`
}

func (t InBackground) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Jobs == nil || t.Registry == nil {
		return "", fmt.Errorf("there is nothing to run work in the background")
	}

	registry := t.Registry()
	if registry == nil {
		return "", fmt.Errorf("the tools are not ready yet")
	}

	var a backgroundArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	tool, known := registry.Get(a.Tool)
	if !known {
		return "", fmt.Errorf("there is no tool called %q", a.Tool)
	}

	/*
	 * A tool that needs approval is not started this way.
	 *
	 * The gate exists so that a person sees an action before it happens, and
	 * "in the background" would put exactly those actions where nobody is
	 * looking. It has to be approved in the ordinary way first.
	 */
	if tool.Risk() == Mutating {
		return "", fmt.Errorf(
			"%s changes something, so it is not started in the background — "+
				"ask for it directly and it will be shown for approval first", a.Tool)
	}

	if tool.Name() == "do_in_background" {
		return "", fmt.Errorf("that would start itself")
	}

	what := strings.TrimSpace(a.What)
	if what == "" {
		what = tool.Summarize(a.Arguments)
	}

	arguments := a.Arguments
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}

	id, err := t.Jobs.Start(what, func(ctx context.Context) (string, error) {
		return tool.Execute(ctx, arguments)
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Started (%d): %s. Carry on; it will be reported when done.",
		id, what), nil
}

// ListBackground reports what is happening behind the conversation.
type ListBackground struct{ Jobs Background }

func (ListBackground) Name() string { return "list_background" }

func (ListBackground) Description() string {
	return "Say what is running in the background and what recently finished. " +
		"Use when asked what you are doing, or whether something is done yet."
}

func (ListBackground) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListBackground) Risk() Risk { return Safe }

func (ListBackground) Summarize(json.RawMessage) string { return "Check what is running" }

func (t ListBackground) Execute(context.Context, json.RawMessage) (string, error) {
	if t.Jobs == nil {
		return "", fmt.Errorf("nothing runs in the background here")
	}

	jobs := t.Jobs.List()

	if len(jobs) == 0 {
		return "Nothing is running in the background.", nil
	}

	var b strings.Builder

	for _, j := range jobs {
		switch j.State {
		case "running":
			fmt.Fprintf(&b, "%d. %s — still going, %s so far\n",
				j.ID, j.What, j.Took.Round(time.Second))
		case "failed":
			fmt.Fprintf(&b, "%d. %s — failed: %s\n", j.ID, j.What, j.Err)
		case "stopped":
			fmt.Fprintf(&b, "%d. %s — stopped\n", j.ID, j.What)
		default:
			fmt.Fprintf(&b, "%d. %s — done in %s: %s\n",
				j.ID, j.What, j.Took.Round(time.Second), first(j.Result, 200))
		}
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// StopBackground ends something running behind the conversation.
type StopBackground struct{ Jobs Background }

func (StopBackground) Name() string { return "stop_background" }

func (StopBackground) Description() string {
	return "Stop something that is running in the background, by its number."
}

func (StopBackground) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "integer", "description": "Which one, from list_background"}},
		"required": ["id"]
	}`)
}

func (StopBackground) Risk() Risk { return Safe }

func (StopBackground) Summarize(raw json.RawMessage) string {
	var a struct {
		ID int64 `json:"id"`
	}

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Stop background job %d", a.ID)
}

func (t StopBackground) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Jobs == nil {
		return "", fmt.Errorf("nothing runs in the background here")
	}

	var a struct {
		ID int64 `json:"id"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := t.Jobs.Stop(a.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("Stopped %d.", a.ID), nil
}
