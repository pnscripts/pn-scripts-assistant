package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * The services that share this code do not share a trust decision.
 *
 * Privacy is judged by provider name, so each has to answer with its own.
 * Somebody who agreed to send conversation to OpenAI has not thereby agreed to
 * OpenRouter — where the request may be served by any of the companies behind
 * it — and a shared implementation reporting one name for both would make that
 * distinction impossible to enforce.
 */
func TestEachServiceKeepsItsOwnName(t *testing.T) {
	if got := NewOpenAI("sk-test", "").Name(); got != "openai" {
		t.Errorf("OpenAI calls itself %q", got)
	}

	if got := NewOpenRouter("sk-test", "").Name(); got != "openrouter" {
		t.Errorf("OpenRouter calls itself %q", got)
	}

	// And neither is ever mistaken for the local one, which is the only
	// provider allowed to see what the brain has learned.
	for _, p := range []Provider{NewOpenAI("k", ""), NewOpenRouter("k", "")} {
		if p.Name() == Local {
			t.Errorf("%T reports itself as the local provider", p)
		}

		if AllowsMemoryFor(p.Name(), ModePrivate) {
			t.Errorf("%s would be sent the owner's memories", p.Name())
		}

		if ModePrivate.AllowsProvider(p.Name()) {
			t.Errorf("%s would be used in private mode", p.Name())
		}

		if ModeResearch.AllowsProvider(p.Name()) {
			t.Errorf("%s would be used in research mode, which keeps the model local",
				p.Name())
		}
	}
}

// Without a key a provider is not offered, and says so rather than failing at
// the far end of a request.
func TestAServiceWithNoKeyIsNotOffered(t *testing.T) {
	for _, p := range []Provider{NewOpenAI("", ""), NewOpenRouter("", "")} {
		if p.Available(t.Context()) {
			t.Errorf("%s offered itself with no key configured", p.Name())
		}

		if _, err := p.Chat(t.Context(), Request{}); err == nil {
			t.Errorf("%s accepted a request with no key", p.Name())
		}
	}
}

/*
 * A conversation is mapped onto the shape these services expect.
 *
 * The system prompt stays a message here, unlike Anthropic where it is a field
 * of its own. A tool result needs the id of the call it answers — and the
 * message that made that call has to be there in front of it, which is what
 * this test exists to hold: without it these services refuse the whole
 * request, and the multi-step turn only ever worked against Ollama.
 */
func TestTheConversationIsMappedForTheOpenAIShape(t *testing.T) {
	body := NewOpenAI("sk-test", "gpt-4o-mini").requestBody(Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are PN Scripts Assistant."},
			{Role: RoleUser, Content: "what is the weather"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{
				ID:        "call_1",
				Name:      "web_search",
				Arguments: json.RawMessage(`{"query":"weather"}`),
			}}},
			{Role: RoleTool, Name: "web_search", Content: "Cloudy, 18 degrees.", ToolCallID: "call_1"},
		},
		Tools: []ToolSpec{{
			Name:        "web_search",
			Description: "Search the web",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
		MaxTokens: 512,
	})

	if len(body.Messages) != 4 {
		t.Fatalf("mapped %d messages, want 4", len(body.Messages))
	}

	if body.Messages[0].Role != RoleSystem {
		t.Error("the system prompt was not sent as a message")
	}

	asked := body.Messages[2]

	if len(asked.ToolCalls) != 1 {
		t.Fatalf("the assistant's own call was not sent: %+v", asked)
	}

	if asked.ToolCalls[0].ID != "call_1" || asked.ToolCalls[0].Type != "function" {
		t.Errorf("the call was sent in a shape these services refuse: %+v", asked.ToolCalls[0])
	}

	// These services write arguments as a string containing JSON, and expect
	// them back the same way.
	if got := asked.ToolCalls[0].Function.Arguments; got != `{"query":"weather"}` {
		t.Errorf("arguments went out as %q", got)
	}

	// A turn that only asked for a tool sends null rather than an empty
	// string, which several of these services refuse alongside tool_calls.
	if asked.Content != nil {
		t.Errorf("an empty assistant turn sent content %#v", asked.Content)
	}

	if body.Messages[3].Role != "tool" || body.Messages[3].ToolCallID != "call_1" {
		t.Errorf("tool output lost its call id: %+v", body.Messages[3])
	}

	if len(body.Tools) != 1 || body.Tools[0].Function.Name != "web_search" {
		t.Errorf("the tool was not offered: %+v", body.Tools)
	}

	if body.Tools[0].Type != "function" {
		t.Errorf("the tool has type %q, which these services will refuse",
			body.Tools[0].Type)
	}

	if body.MaxTokens != 512 {
		t.Errorf("the reply cap was lost: %d", body.MaxTokens)
	}
}

/*
 * The second turn of a conversation that used a tool is still sendable.
 *
 * Tool output is kept in the transcript so a later turn knows what was looked
 * up, and it is read back with no id and nothing in front of it. That was a
 * refused request from every one of these services — not on the turn that used
 * the tool, but on the next thing the person said, which is why it read as the
 * assistant breaking at random.
 */
func TestAnOrphanedToolResultIsNotSentAsOne(t *testing.T) {
	body := NewOpenAI("sk-test", "gpt-4o-mini").requestBody(Request{
		Messages: []Message{
			{Role: RoleUser, Content: "what is the weather"},
			{Role: RoleTool, Name: "web_search", Content: "Cloudy, 18 degrees."},
			{Role: RoleAssistant, Content: "Cloudy and 18 degrees."},
			{Role: RoleUser, Content: "and tomorrow?"},
		},
	})

	for i, m := range body.Messages {
		if m.Role == "tool" {
			t.Fatalf("message %d went out as a tool result answering nothing: %+v", i, m)
		}
	}

	// The evidence survives; only the role it could no longer carry is gone.
	kept, _ := body.Messages[1].Content.(string)

	if !strings.Contains(kept, "Cloudy, 18 degrees.") {
		t.Errorf("what the tool returned was lost: %q", kept)
	}

	if !strings.Contains(kept, "web_search") {
		t.Errorf("the folded result does not say which tool it came from: %q", kept)
	}
}
