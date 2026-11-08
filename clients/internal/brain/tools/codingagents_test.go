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
		{ID: "codex", Title: "Codex", Ready: true},
	}
}

func (twoAgents) AskCodingAgent(_ context.Context, id, folder, task, access string) (AgentAnswer, error) {
	return AgentAnswer{Agent: id, Said: task + " in " + folder + " as " + access}, nil
}

func TestTheCodingAgentToolSaysWhoWhereAndHowFar(t *testing.T) {
	tool := AskCodingAgent{Agents: twoAgents{}}

	raw := json.RawMessage(`{"task":"Explain main.go","folder":"/p"}`)

	if s := tool.Summarize(raw); s != "Ask Codex — only reads /p and answers:\nExplain main.go" {
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
