package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type twoAgents struct{}

func (twoAgents) CodingAgents() []CodingAgent {
	return []CodingAgent{
		{ID: "claude-code", Title: "Claude Code", Ready: false, Why: "not signed in"},
		{ID: "codex", Title: "Codex", Ready: true, Pace: "60 % of its allowance used with 30 % of its 7-day window gone — ahead of pace"},
	}
}

func (twoAgents) AskCodingAgent(_ context.Context, id, folder, task, access string) (AgentAnswer, error) {
	return AgentAnswer{Agent: id, Said: task + " in " + folder + " as " + access}, nil
}

func TestTheCodingAgentToolSaysWhoWhereAndHowFar(t *testing.T) {
	tool := AskCodingAgent{Agents: twoAgents{}}

	raw := json.RawMessage(`{"task":"Explain main.go","folder":"/p"}`)

	if s := tool.Summarize(raw); s != "Ask Codex (only reads /p and answers):\nExplain main.go\n\n"+
		"Codex: 60 % of its allowance used with 30 % of its 7-day window gone — ahead of pace." {
		t.Errorf("%q", s)
	}

	out, err := tool.Execute(context.Background(), raw)
	if err != nil || !strings.Contains(out, "Explain main.go in /p as read") || !strings.Contains(out, "No file in the folder changed.") {
		t.Errorf("%q %v", out, err)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"task":"x","folder":"/p","agent":"claude-code"}`)); err == nil ||
		!strings.Contains(err.Error(), "not signed in") {
		t.Errorf("an agent that cannot be asked was asked: %v", err)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"task":"x","folder":"/p","agent":"gemini"}`)); err == nil ||
		!strings.Contains(err.Error(), "Codex (ready)") {
		t.Errorf("%v", err)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"task":"x","folder":"/p","access":"root"}`)); err == nil {
		t.Error("an unknown access was accepted")
	}

	if tool.AlwaysAsks(raw) != "" || tool.AlwaysAsks(json.RawMessage(`{"access":"full"}`)) == "" {
		t.Error("only full access is always asked")
	}

	if len(tool.Outward(raw)) != 0 || len(tool.Outward(json.RawMessage(`{"access":"folder"}`))) != 1 {
		t.Error("only an agent that may write changes files")
	}

	if leaves, _ := Leaves(tool); !leaves {
		t.Error("it does not say what it sends off this machine")
	}
}

type heldAgent struct{ asked *int }

func (heldAgent) CodingAgents() []CodingAgent {
	return []CodingAgent{{ID: "codex", Title: "Codex", Ready: true,
		Pace: "70 % of its allowance used with 20 % of its 7-day window gone; ahead of pace",
		Held: "Held for your quiet hours (22:00 to 07:00): its allowance is being spent faster than it renews, " +
			"so heavy work waits until 22:00 today. Ask again then, or say yes to start it now anyway."}}
}

func (h heldAgent) AskCodingAgent(_ context.Context, id, folder, task, access string) (AgentAnswer, error) {
	*h.asked++

	return AgentAnswer{Agent: id, Said: "done"}, nil
}

/*
 * Work on an allowance spent ahead of its pace, outside the quiet hours, is
 * put to its user with until when it waits: their yes is the one press that
 * starts it anyway. Any access, reading included, and full access still
 * says why it is always asked.
 */
func TestAnAgentHeldForQuietHoursIsAskedWithUntilWhen(t *testing.T) {
	var asked int

	tool := AskCodingAgent{Agents: heldAgent{asked: &asked}}
	raw := json.RawMessage(`{"task":"Refactor the parser","folder":"/p"}`)

	if why := tool.AlwaysAsks(raw); why != "held for your quiet hours" {
		t.Errorf("reading while held: %q", why)
	}

	if why := tool.AlwaysAsks(json.RawMessage(`{"task":"x","folder":"/p","access":"full"}`)); why !=
		"full access for a coding agent; held for your quiet hours" {
		t.Errorf("full access while held: %q", why)
	}

	s := tool.Summarize(raw)
	if !strings.HasSuffix(s, "waits until 22:00 today. Ask again then, or say yes to start it now anyway.") ||
		!strings.Contains(s, "ahead of pace") {
		t.Errorf("%q", s)
	}

	// The yes runs it: being held is a question, never a refusal.
	if _, err := tool.Execute(context.Background(), raw); err != nil || asked != 1 {
		t.Errorf("the yes did not start it: %v, asked %d", err, asked)
	}
}
