package mcpserve

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * permission_prompt against a stand-in for the door
 * (server/outside_permit_test.go holds it to the real one): whatever
 * happens, Claude Code is answered in the one form it reads, and
 * everything that is not the user's own yes is a deny that says why.
 */

// permitted runs the tool and reads its answer as Claude Code does: one
// JSON object with behavior, and message or updatedInput.
func permitted(t *testing.T, s *Server, arguments string) map[string]json.RawMessage {
	t.Helper()

	text, err := run(t, s, PermissionTool, arguments)
	if err != nil {
		t.Fatalf("%s: failed instead of answering: %v", arguments, err)
	}

	var answer map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		t.Fatalf("%s: not one JSON object: %q", arguments, text)
	}

	for field := range answer {
		if field != "behavior" && field != "message" && field != "updatedInput" {
			t.Errorf("%s: a field Claude Code does not read: %s", arguments, field)
		}
	}

	return answer
}

func TestAYesGivesTheInputBackAsItCame(t *testing.T) {
	s, door := answering(t, `200 {"behavior":"allow"}`)

	// The same fields, values and order; only the spaces between go.
	const input = `{"command":"ls -la","description":"List"}`

	got := permitted(t, s, `{"tool_name":"Bash","input":{ "command": "ls -la",  "description":"List" },"tool_use_id":"toolu_01","later":1}`)
	if string(got["behavior"]) != `"allow"` || string(got["updatedInput"]) != input || len(got) != 2 {
		t.Fatalf("a yes: %s", got)
	}

	// The door was sent what is asked about and what would be given, with
	// who asks, and nothing else of what the program handed over.
	if len(door.asked) != 1 || door.asked[0] != `permission_prompt {"input":`+input+`,"tool_name":"Bash"} by Claude Code/2.1.0` {
		t.Errorf("the door was asked: %q", door.asked)
	}

	// No input at all: a yes with none to give back.
	s, _ = answering(t, `200 {"behavior":"allow"}`)

	for _, arguments := range []string{`{"tool_name":"ExitPlanMode"}`, `{"tool_name":"ExitPlanMode","input":null}`} {
		if got := permitted(t, s, arguments); string(got["behavior"]) != `"allow"` || len(got) != 1 {
			t.Errorf("%s: %s", arguments, got)
		}
	}

	// One too long to give back is left out, which Claude Code reads as
	// "unchanged".
	s, _ = answering(t, `200 {"behavior":"allow"}`)

	long, _ := json.Marshal(map[string]string{"file_path": "/home/you/a.txt", "content": strings.Repeat("a", permitEcho)})

	if got := permitted(t, s, `{"tool_name":"Write","input":`+string(long)+`}`); string(got["behavior"]) != `"allow"` || len(got) != 1 {
		t.Errorf("a very long input was given back (%d fields)", len(got))
	}
}

func TestEveryNoSaysWhy(t *testing.T) {
	for _, c := range []struct {
		answer string
		says   []string
	}{
		{`200 {"behavior":"deny","ended":"refused"}`, []string{"The answer given on its card", "was no", "Do not try to do the same thing another way"}},
		{`200 {"behavior":"deny","ended":"unanswered"}`, []string{"Nobody answered", "within ten minutes", "is a no"}},
		{`200 {"behavior":"deny","ended":"taken back"}`, []string{"taken back", "turned permission questions from other programs off"}},
		{`200 {"behavior":"deny","ended":"gone"}`, []string{"stopped waiting", "That is a no"}},
		{`200 {"behavior":"deny","ended":"unrecorded"}`, []string{"could not write the answer on its record", "That is a no"}},
		{`200 {"behavior":"deny","ended":"not a permission"}`, []string{"permission questions only", "ask it in your answer instead"}},
		{`200 {"behavior":"deny"}`, []string{"did not say yes", "That is a no"}},
		{`403 {"error":"Its user has not allowed other programs to ask that.","outside":"not allowed","subject":"permissions"}`,
			[]string{"has not allowed other programs to put their permission questions to them", "Only the user can allow it", "What other programs may ask"}},
		{`503 {"error":"Too many permission questions are waiting for the assistant's user already.","outside":"permits full"}`,
			[]string{"as many permission questions as it shows at once are waiting", "ask again when some are answered"}},
		{`429 {"error":"Other programs have asked too many questions in the last minute.","outside":"too many"}`,
			[]string{"too many questions in the last minute"}},
		{`403 {"error":"The terms of use have not been accepted yet.","terms":"not accepted"}`, []string{"terms of use have not been accepted"}},
		{`404 not found`, []string{"older than this server"}},
		// Anything that is not a plain yes is a no: a yes misspelt, a yes
		// beside something else, a page.
		{`200 {"behavior":"Allow"}`, []string{"did not answer as PN Scripts Assistant"}},
		{`200 {"behavior":"allowed"}`, []string{"did not answer as PN Scripts Assistant"}},
		{`200 {"allow":true}`, []string{"did not answer as PN Scripts Assistant"}},
		{`200 <html>allow</html>`, []string{"did not answer as PN Scripts Assistant"}},
		{`200 "allow"`, []string{"did not answer as PN Scripts Assistant"}},
	} {
		s, _ := answering(t, c.answer)

		got := permitted(t, s, `{"tool_name":"Bash","input":{"command":"ls"}}`)

		var message string

		if string(got["behavior"]) != `"deny"` || json.Unmarshal(got["message"], &message) != nil || len(got) != 2 {
			t.Errorf("%s: answered %s", c.answer, got)

			continue
		}

		for _, part := range c.says {
			if !strings.Contains(message, part) {
				t.Errorf("%s: does not say %q: %s", c.answer, part, message)
			}
		}
	}

	// Nothing running there at all.
	got := permitted(t, New("http://127.0.0.1:1", "1"), `{"tool_name":"Bash","input":{"command":"ls"}}`)
	if string(got["behavior"]) != `"deny"` || !strings.Contains(string(got["message"]), "is not running on this computer") {
		t.Errorf("with nothing running: %s", got)
	}
}

// A question about nothing, or with something that is not an input, asks
// the user nothing and is a no.
func TestAPermissionQuestionThatCannotBeReadAsksNobody(t *testing.T) {
	s, door := answering(t, `200 {"behavior":"allow"}`)

	for _, arguments := range []string{
		``, `null`, `{}`, `[]`, `"Bash"`, `not json`,
		`{"tool_name":""}`, `{"tool_name":"   "}`, `{"tool_name":7}`, `{"input":{"command":"ls"}}`,
		`{"tool_name":"Bash","input":"ls"}`, `{"tool_name":"Bash","input":["ls"]}`, `{"tool_name":"Bash","input":7}`,
	} {
		if got := permitted(t, s, arguments); string(got["behavior"]) != `"deny"` || len(got["message"]) < 20 {
			t.Errorf("%q: %s", arguments, got)
		}
	}

	if len(door.asked) != 0 {
		t.Errorf("the user was asked about nothing: %q", door.asked)
	}
}

/*
 * What is sent on to be shown: an input as it came when short; a long one
 * with each long text cut and saying how long it was; and one that is
 * still too long as a note of its size. Never cut without saying so.
 */
func TestALongInputIsSentOnCutAndSaysSo(t *testing.T) {
	short := json.RawMessage(`{"command":"ls"}`)
	if got, cut := permitShown(short); string(got) != string(short) || cut != nil {
		t.Errorf("a short input was changed: %s", got)
	}

	long, _ := json.Marshal(map[string]any{
		"file_path": "/home/you/a.txt",
		"content":   strings.Repeat("я", 5000),
		"edits":     []string{strings.Repeat("b", 3000)},
		"count":     3,
	})

	var shown map[string]any

	got, cut := permitShown(long)
	if err := json.Unmarshal(got, &shown); err != nil || len(got) > permitMost {
		t.Fatalf("what is sent on (%d bytes): %v", len(got), err)
	}

	content, _ := shown["content"].(string)

	if shown["file_path"] != "/home/you/a.txt" || shown["count"] != float64(3) ||
		len([]rune(content)) != permitString || len(cut) != 1 || cut["content"] != 5000 ||
		!strings.HasPrefix(shown["edits"].(string), "(left out: ") {
		t.Errorf("what is sent on: %v", shown)
	}

	// Many long texts: a note of the size, and nothing of the input.
	many := map[string]string{}
	for i := 0; i < 40; i++ {
		many[strings.Repeat("k", i+1)] = strings.Repeat("v", 2000)
	}

	huge, _ := json.Marshal(many)

	got, cut = permitShown(huge)
	if cut != nil || !strings.Contains(string(got), "too_long_to_show") || strings.Contains(string(got), "vvvv") || len(got) > 200 {
		t.Errorf("an input too long to show: %s", got)
	}
}

// Found by the independent review: an input is measured as it travels.
// One under the limit by its own bytes but full of "<", ">" and "&", which
// are written as six bytes each on the way, would pass what the door
// reads, and the question would be refused unasked.
func TestAnInputIsMeasuredAsItTravels(t *testing.T) {
	// Written by hand, as Claude Code sends it: the marks as themselves.
	page := json.RawMessage(`{"file_path":"/home/you/a.html","content":"` + strings.Repeat("<b>&</b>", 500) + `"}`)
	if len(page) > permitWhole {
		t.Fatalf("the test's input is %d bytes, not under %d", len(page), permitWhole)
	}

	got, cut := permitShown(page)

	sent, _ := json.Marshal(got)
	if len(sent) > permitMost {
		t.Errorf("what is sent on is %d bytes as it travels, more than %d", len(sent), permitMost)
	}

	var shown map[string]string
	if err := json.Unmarshal(got, &shown); err != nil || shown["file_path"] != "/home/you/a.html" || len(shown["content"]) != permitString || cut["content"] != 4000 {
		t.Errorf("what is sent on: %s (%v)", got, err)
	}
}
