package llm

import (
	"encoding/json"
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

		if AllowsMemoryFor(p.Name()) {
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
 * of its own — and tool output needs the id of the call it answers, which some
 * of these services drop silently rather than refusing when it is missing.
 */
func TestTheConversationIsMappedForTheOpenAIShape(t *testing.T) {
	body := NewOpenAI("sk-test", "gpt-4o-mini").requestBody(Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are PN Brain."},
			{Role: RoleUser, Content: "what is the weather"},
			{Role: RoleTool, Content: "Cloudy, 18 degrees.", ToolCallID: "call_1"},
		},
		Tools: []ToolSpec{{
			Name:        "web_search",
			Description: "Search the web",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
		MaxTokens: 512,
	})

	if len(body.Messages) != 3 {
		t.Fatalf("mapped %d messages, want 3", len(body.Messages))
	}

	if body.Messages[0].Role != RoleSystem {
		t.Error("the system prompt was not sent as a message")
	}

	if body.Messages[2].Role != "tool" || body.Messages[2].ToolCallID != "call_1" {
		t.Errorf("tool output lost its call id: %+v", body.Messages[2])
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
