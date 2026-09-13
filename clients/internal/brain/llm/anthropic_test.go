package llm

import (
	"encoding/json"
	"testing"
)

func blocksOf(t *testing.T, m anthropicMessage) []anthropicBlock {
	t.Helper()

	blocks, ok := m.Content.([]anthropicBlock)
	if !ok {
		t.Fatalf("expected content blocks, got %T: %#v", m.Content, m.Content)
	}

	return blocks
}

/*
 * Tool output reaches Anthropic instead of being dropped on the way.
 *
 * Anthropic has no role for tool output: a call is a tool_use block on the
 * assistant's own turn and its result is a tool_result block on the next user
 * turn. Sending it as a role of its own did not fail loudly — it matched no
 * case in the mapping and was silently discarded, so the model was asked the
 * same question again with no sign it had ever looked anything up. That is the
 * "it keeps re-reading the same file" loop, and this is the test that holds it
 * shut.
 */
func TestToolOutputReachesAnthropicAsContentBlocks(t *testing.T) {
	body := NewAnthropic("sk-ant-test", "").requestBody(Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are PN Brain."},
			{Role: RoleUser, Content: "what machine is this"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID:        "c1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"/etc/hostname"}`),
			}}},
			{Role: RoleTool, ToolCallID: "c1", Content: "petar-desktop"},
		},
		Tools: []ToolSpec{{
			Name:       "read_file",
			Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	})

	// The system prompt is a field of its own here, not a message.
	if body.System != "You are PN Brain." {
		t.Errorf("the system prompt was not lifted out: %q", body.System)
	}

	if len(body.Messages) != 3 {
		t.Fatalf("mapped %d messages, want 3", len(body.Messages))
	}

	asked := blocksOf(t, body.Messages[1])

	if len(asked) != 1 || asked[0].Type != "tool_use" || asked[0].ID != "c1" {
		t.Fatalf("the assistant's call was not sent as a tool_use: %+v", asked)
	}

	if got := string(asked[0].Input); got != `{"path":"/etc/hostname"}` {
		t.Errorf("the call's arguments went out as %s", got)
	}

	// The result comes back as a user turn, which is the only shape Anthropic
	// will take it in.
	if body.Messages[2].Role != RoleUser {
		t.Errorf("the result was sent as role %q", body.Messages[2].Role)
	}

	answered := blocksOf(t, body.Messages[2])

	if len(answered) != 1 || answered[0].Type != "tool_result" {
		t.Fatalf("the result was not sent as a tool_result: %+v", answered)
	}

	if answered[0].ToolUseID != "c1" || answered[0].Content != "petar-desktop" {
		t.Errorf("the result does not answer the call: %+v", answered[0])
	}
}

/*
 * Every result for one assistant turn travels in a single message.
 *
 * Anthropic wants each tool_use answered in the user turn that immediately
 * follows it. Two calls answered by two separate user messages is a refused
 * request, because the second result arrives after the turn that was supposed
 * to contain it has already ended.
 */
func TestEveryResultForOneTurnTravelsInOneMessage(t *testing.T) {
	body := NewAnthropic("sk-ant-test", "").requestBody(Request{
		Messages: []Message{
			{Role: RoleUser, Content: "read both"},
			{Role: RoleAssistant, Content: "Looking at both.", ToolCalls: []ToolCall{
				{ID: "c1", Name: "read_file"},
				{ID: "c2", Name: "read_file"},
			}},
			{Role: RoleTool, ToolCallID: "c1", Content: "first"},
			{Role: RoleTool, ToolCallID: "c2", Content: "second"},
		},
		Tools: []ToolSpec{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})

	if len(body.Messages) != 3 {
		t.Fatalf("mapped %d messages, want 3 — the two results must share one", len(body.Messages))
	}

	// What it said and what it asked for travel together, in that order.
	asked := blocksOf(t, body.Messages[1])

	if len(asked) != 3 || asked[0].Type != "text" || asked[0].Text != "Looking at both." {
		t.Fatalf("the assistant's words were lost beside its calls: %+v", asked)
	}

	answered := blocksOf(t, body.Messages[2])

	if len(answered) != 2 {
		t.Fatalf("the two results were not coalesced: %+v", answered)
	}

	if answered[0].ToolUseID != "c1" || answered[1].ToolUseID != "c2" {
		t.Errorf("the results are not in the order they were asked for: %+v", answered)
	}
}

// An ordinary turn is still a plain string, which is nearly every message.
func TestAnOrdinaryTurnIsStillAString(t *testing.T) {
	body := NewAnthropic("sk-ant-test", "").requestBody(Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
			{Role: RoleAssistant, Content: "Hello."},
		},
	})

	for i, m := range body.Messages {
		if _, ok := m.Content.(string); !ok {
			t.Errorf("message %d became blocks for no reason: %#v", i, m.Content)
		}
	}
}
