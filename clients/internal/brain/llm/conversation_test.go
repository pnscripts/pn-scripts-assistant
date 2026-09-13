package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * Arguments arrive in two shapes and reach a tool in one.
 *
 * OpenAI and everything that copied it write them as a string containing JSON.
 * The loop hands whatever arrives to a tool that unmarshals it into a struct,
 * so the string form failed with "cannot unmarshal string into Go value of
 * type struct" — an error that named the tool and said nothing about the
 * provider that had written its arguments differently.
 */
func TestArgumentsWrittenAsAStringBecomeAnObject(t *testing.T) {
	for _, c := range []struct {
		what string
		from string
		want string
	}{
		{"the OpenAI shape", `"{\"path\":\"/etc/hosts\"}"`, `{"path":"/etc/hosts"}`},
		{"the Ollama shape", `{"path":"/etc/hosts"}`, `{"path":"/etc/hosts"}`},
		{"a tool that takes nothing", `""`, `{}`},
		{"nothing at all", ``, `{}`},
		{"a null", `null`, `{}`},
	} {
		got := string(AsObject(json.RawMessage(c.from)))

		if got != c.want {
			t.Errorf("%s: %s became %s, want %s", c.what, c.from, got, c.want)
		}
	}
}

// A string that does not contain JSON is left as it is. Unquoting it would
// trade an error a person can read for a request body that will not marshal.
func TestArgumentsThatAreNotJSONAreLeftAlone(t *testing.T) {
	got := string(AsObject(json.RawMessage(`"sorry, I cannot do that"`)))

	if got != `"sorry, I cannot do that"` {
		t.Errorf("a string that was not JSON came back as %s", got)
	}

	if !json.Valid([]byte(got)) {
		t.Error("what came back cannot be marshalled into a request")
	}
}

// A result whose call is right in front of it is what both APIs are asking
// for, and is passed through untouched.
func TestAToolResultKeepsItsRoleWhenTheCallIsThere(t *testing.T) {
	out := Replayable([]Message{
		{Role: RoleUser, Content: "read the file"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "read_file"}}},
		{Role: RoleTool, ToolCallID: "c1", Content: "hello"},
	})

	if len(out) != 3 {
		t.Fatalf("rewrote a conversation that was already correct: %d messages", len(out))
	}

	if out[2].Role != RoleTool || out[2].ToolCallID != "c1" {
		t.Errorf("a matched result was moved: %+v", out[2])
	}

	if len(out[1].ToolCalls) != 1 {
		t.Errorf("the call that was answered was dropped: %+v", out[1])
	}
}

/*
 * A call nobody answered is not sent.
 *
 * Both APIs require every call in a turn to be answered in the next one, so an
 * unanswered call is a refused request waiting to happen — and the assistant
 * having asked for something whose outcome nobody knows is worth less than the
 * conversation going through at all.
 */
func TestACallNobodyAnsweredIsNotSent(t *testing.T) {
	out := Replayable([]Message{
		{Role: RoleUser, Content: "read both files"},
		{Role: RoleAssistant, Content: "Looking.", ToolCalls: []ToolCall{
			{ID: "c1", Name: "read_file"},
			{ID: "c2", Name: "read_file"},
		}},
		{Role: RoleTool, ToolCallID: "c1", Content: "hello"},
	})

	if len(out[1].ToolCalls) != 1 || out[1].ToolCalls[0].ID != "c1" {
		t.Errorf("an unanswered call was sent anyway: %+v", out[1].ToolCalls)
	}

	if out[1].Content != "Looking." {
		t.Errorf("what the assistant said was lost: %q", out[1].Content)
	}
}

// An assistant turn with no words and no surviving calls has nothing in it,
// and Anthropic refuses one.
func TestAnAssistantTurnWithNothingInItIsDropped(t *testing.T) {
	out := Replayable([]Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "   "},
		{Role: RoleUser, Content: "still there?"},
	})

	for _, m := range out {
		if m.Role == RoleAssistant {
			t.Fatalf("an empty assistant turn was sent: %+v", m)
		}
	}
}

// Anthropic refuses a final assistant message that ends in whitespace, which a
// reply cut off part-way through often does.
func TestAFinalAssistantMessageDoesNotEndInWhitespace(t *testing.T) {
	out := Replayable([]Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "One moment.\n\n"},
	})

	if got := out[len(out)-1].Content; got != "One moment." {
		t.Errorf("the last assistant message ends in whitespace: %q", got)
	}
}

/*
 * A turn that offers no tools carries no calls in its history.
 *
 * Anthropic refuses a conversation containing a call to a tool nothing
 * declares. Declaring the tools again purely to make the history legal would
 * invite the model to use them on exactly the turns meant to be spoken.
 */
func TestATurnWithNoToolsKeepsTheEvidenceAndDropsTheCalls(t *testing.T) {
	out := Replayable(withoutToolCalls([]Message{
		{Role: RoleUser, Content: "read the file"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "read_file"}}},
		{Role: RoleTool, Name: "read_file", ToolCallID: "c1", Content: "hello"},
	}))

	for _, m := range out {
		if len(m.ToolCalls) > 0 {
			t.Errorf("a call survived into a turn that offers no tools: %+v", m)
		}

		if m.Role == RoleTool {
			t.Errorf("a tool result survived into a turn that offers no tools: %+v", m)
		}
	}

	var whole string
	for _, m := range out {
		whole += m.Content
	}

	if !strings.Contains(whole, "hello") {
		t.Errorf("what the tool returned was lost: %q", whole)
	}
}
