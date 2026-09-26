package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

// seventy tools of the size the real ones are, for measuring against.
func manyTools(n int) []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, n)

	for i := 0; i < n; i++ {
		out = append(out, llm.ToolSpec{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: strings.Repeat("what this tool does and when to use it. ", 3),
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"where"}}}`),
		})
	}

	return out
}

/*
 * A turn is given what it can afford to read.
 *
 * Measured on this machine: a turn with thirty-nine tools is 6,338 tokens and
 * this processor reads eight a second. All seventy-three would be about
 * 9,500 — twenty minutes before the first word.
 */
func TestATurnIsGivenWhatItCanAffordToRead(t *testing.T) {
	specs := manyTools(70)

	chosen := Choosing{}.choose(specs, "what is in that file", nil)

	if len(chosen) == 0 {
		t.Fatal("nothing at all was offered")
	}

	if len(chosen) >= len(specs) {
		t.Errorf("all %d tools were offered", len(chosen))
	}

	var spent int

	for _, spec := range chosen {
		spent += costOf(spec)
	}

	if spent > HowMuchSchema+2000 {
		t.Errorf("the offered tools cost %d characters against a budget of %d", spent, HowMuchSchema)
	}

	t.Logf("%d of %d tools, %d characters", len(chosen), len(specs), spent)
}

/*
 * The ordinary business of an assistant is always there.
 *
 * A turn without read_file is not a cheaper turn, it is a broken one — and
 * the budget must not be able to take one away.
 */
func TestWhatEveryTurnNeedsIsAlwaysOffered(t *testing.T) {
	specs := append(manyTools(70),
		llm.ToolSpec{Name: "read_file", Description: strings.Repeat("read a file. ", 40)},
		llm.ToolSpec{Name: "ask_first", Description: strings.Repeat("ask before guessing. ", 40)},
		llm.ToolSpec{Name: "write_file", Description: strings.Repeat("write a file. ", 40)},
	)

	chosen := names(Choosing{}.choose(specs, "tell me a joke", nil))

	for _, want := range []string{"read_file", "ask_first"} {
		if !chosen[want] {
			t.Errorf("%s was not offered", want)
		}
	}

	// And nothing that changes anything, for a message that asks for nothing
	// of the sort: a model that can see write_file while being asked for a
	// joke will find something to do with it.
	if chosen["write_file"] {
		t.Error("writing was offered to a turn that asked for a joke")
	}
}

/*
 * Something whose requirement is not on this machine is not offered.
 *
 * Not a refusal: the inventory in the prompt says Godot is not installed and
 * what it would take, so the assistant can still answer about it. What is
 * saved is the budget, and the model's attention, on a tool that could only
 * have failed.
 */
func TestAToolThatCannotWorkHereIsNotOffered(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "godot_build", Description: "build a Godot project"},
		{Name: "read_file", Description: "read a file"},
	}

	chosen := names(Choosing{
		Needs: func(tool string) []string {
			if tool == "godot_build" {
				return []string{"godot"}
			}

			return nil
		},
		Have: func(id string) bool { return id != "godot" },
	}.choose(specs, "build my game", nil))

	if chosen["godot_build"] {
		t.Error("a tool needing Godot was offered on a machine without Godot")
	}

	if !chosen["read_file"] {
		t.Error("everything else was dropped too")
	}
}

/*
 * What a conversation has been shown, it keeps being shown.
 *
 * Ollama keeps what it has already read only while the prompt begins the same
 * way, and the tools are part of that beginning. Measured here: 598 seconds
 * for a turn read from nothing against 58 for one whose prefix was reused. A
 * set that changed with every message would pay the first number every time.
 */
func TestWhatWasOfferedOnceStaysOffered(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "read_file", Description: "read a file"},
		{Name: "look_at_screen", Description: "look at the screen"},
	}

	// First message mentions the screen, so the housekeeping tool is offered.
	first := names(Choosing{}.choose(specs, "what is on my screen", nil))

	if !first["look_at_screen"] {
		t.Fatal("a tool the message asked for was not offered")
	}

	// The next message is about something else. Taking it away now would
	// throw away everything the model has already read.
	second := Choosing{}.choose(specs, "and what is two plus two", map[string]bool{"look_at_screen": true})

	if !names(second)["look_at_screen"] {
		t.Error("a tool already shown in this conversation was taken away")
	}
}

// The same question twice gives the same list, byte for byte: a list that
// reshuffles itself is a prompt that cannot be reused.
func TestTheSameQuestionGivesTheSameList(t *testing.T) {
	specs := manyTools(50)

	first := Choosing{}.choose(specs, "read the file and tell me what it says", nil)
	second := Choosing{}.choose(specs, "read the file and tell me what it says", nil)

	if len(first) != len(second) {
		t.Fatalf("two identical questions gave %d and %d tools", len(first), len(second))
	}

	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("position %d is %s and then %s", i, first[i].Name, second[i].Name)
		}
	}
}

// A tool named outright is offered, which is how a task's step asks for one.
func TestAToolAskedForByNameIsOffered(t *testing.T) {
	specs := append(manyTools(60), llm.ToolSpec{
		Name: "godot_status", Description: strings.Repeat("say whether Godot is installed. ", 20),
	})

	chosen := names(Choosing{}.choose(specs, "check it with godot_status", nil))

	if !chosen["godot_status"] {
		t.Error("a tool named in the message was not offered")
	}
}

func names(specs []llm.ToolSpec) map[string]bool {
	out := map[string]bool{}

	for _, spec := range specs {
		out[spec.Name] = true
	}

	return out
}
