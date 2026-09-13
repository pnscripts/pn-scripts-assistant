package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The planner's choice is taken when it names somebody real.
 *
 * And costs nothing when it does not: a model inventing "senior_architect"
 * should mean the step is done by the generalist, not that the task fails.
 */
func TestThePlannersChoiceIsTakenWhenItNamesSomebodyReal(t *testing.T) {
	roster := BuiltIn()

	if got := For(roster, "researcher", store.StepLook, "find out what changed"); got.Name != "researcher" {
		t.Errorf("named researcher, got %q", got.Name)
	}

	if got := For(roster, "senior_architect", store.StepDo, "think about it"); got.Name != "assistant" {
		t.Errorf("an invented name gave %q, want the generalist", got.Name)
	}

	// Case and spacing are the planner's business, not the roster's.
	if got := For(roster, "  Writer ", store.StepWrite, "write it up"); got.Name != "writer" {
		t.Errorf("got %q", got.Name)
	}
}

// When the planner says nothing, the step itself decides.
func TestAStepWithNoNameFindsSomebodyByWhatItNeeds(t *testing.T) {
	roster := BuiltIn()

	for _, c := range []struct {
		kind, instruction, want string
	}{
		{store.StepWrite, "write a summary of the quarter for a client", "writer"},
		{store.StepLook, "search the web for what changed in Godot 4.3", "researcher"},
		{store.StepDo, "export the Godot project and check the build runs", "game_developer"},
		{store.StepDo, "say hello", "assistant"},
	} {
		if got := For(roster, "", c.kind, c.instruction); got.Name != c.want {
			t.Errorf("%q went to %q, want %q", c.instruction, got.Name, c.want)
		}
	}
}

/*
 * A step that writes code is not writing prose.
 *
 * The kind says "write", which would otherwise send it to the writer — and
 * the writer has no edit_file and no run_command, so it would sit there
 * reaching for tools it does not have.
 */
func TestWritingCodeIsNotWritingProse(t *testing.T) {
	got := For(BuiltIn(), "", store.StepWrite, "write the function that parses the ledger")

	if got.Name == "writer" {
		t.Errorf("a code-writing step went to the writer")
	}
}

/*
 * Every agent has a genuinely different tool list and model.
 *
 * Two agents that differ only in name make the planner's job harder for
 * nothing, and a roster full of them routes badly while looking busy.
 */
func TestNoTwoAgentsAreTheSameJob(t *testing.T) {
	seen := map[string]string{}

	for _, a := range BuiltIn() {
		if a.For == "" {
			t.Errorf("%s does not say what it is for, so the planner cannot pick it", a.Name)
		}

		key := strings.Join(a.Tools, ",") + "|" + a.Uses

		if other, clash := seen[key]; clash {
			t.Errorf("%s and %s are the same job with two names", a.Name, other)
		}

		seen[key] = a.Name
	}
}

/*
 * An agent's tool list only ever removes.
 *
 * The property the whole roster rests on: a new agent can never be a new way
 * round the gate, because everything it names still has to exist, still goes
 * through the permits book, and is still refused by privacy.
 */
func TestAnAgentCanOnlyNarrow(t *testing.T) {
	writer, _ := Find(BuiltIn(), "writer")

	if writer.Allows("run_command") {
		t.Error("the writer may run commands")
	}

	if !writer.Allows("write_document") {
		t.Error("the writer may not write a document")
	}

	// The generalist names nothing, which means everything — and that is the
	// only way to get everything.
	assistant, _ := Find(BuiltIn(), "assistant")

	if len(assistant.Tools) != 0 {
		t.Error("the generalist has a list, so it is not the generalist")
	}

	if !assistant.Allows("run_command") {
		t.Error("the generalist cannot run a command")
	}
}

// Each agent asks for a size of model rather than a name, because the names
// differ on every machine — and a machine with one model answers every role
// with it, which is correct rather than a failure.
func TestModelsAreAskedForByRole(t *testing.T) {
	sizes := llm.Sizes{Work: "qwen2.5-coder:7b", Quick: "llama3.2:3b", Best: "qwen3:14b"}

	writer, _ := Find(BuiltIn(), "writer")
	researcher, _ := Find(BuiltIn(), "researcher")
	developer, _ := Find(BuiltIn(), "developer")

	if got := writer.Model(sizes); got != "qwen3:14b" {
		t.Errorf("the writer uses %q", got)
	}

	if got := researcher.Model(sizes); got != "llama3.2:3b" {
		t.Errorf("the researcher uses %q", got)
	}

	if got := developer.Model(sizes); got != "qwen2.5-coder:7b" {
		t.Errorf("the developer uses %q", got)
	}

	// One model installed: everybody uses it, and nothing breaks.
	only := llm.Sizes{Work: "gemma3:4b"}

	for _, a := range BuiltIn() {
		if got := a.Model(only); got != "gemma3:4b" {
			t.Errorf("%s asked for %q on a machine with one model", a.Name, got)
		}
	}
}

/*
 * A roster its owner edited wins over the one that shipped.
 *
 * Replacing rather than rebuilding: somebody who wants the developer on a
 * paid model edits one line, and keeps everybody else.
 */
func TestAnEditedAgentReplacesTheOneThatShipped(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		t.Fatal(err)
	}

	written := "name: developer\ntitle: Developer\nfor: writing code\n" +
		"uses: best\nprovider: anthropic\ntools: read_file, write_file\n---\n" +
		"Always run the tests.\n"

	if err := os.WriteFile(filepath.Join(Folder(root), "developer.md"), []byte(written), 0o600); err != nil {
		t.Fatal(err)
	}

	roster := Roster(root)

	developer, ok := Find(roster, "developer")
	if !ok {
		t.Fatal("the developer is gone")
	}

	if developer.Uses != UsesBest || developer.Provider != "anthropic" {
		t.Errorf("the edit was not picked up: %+v", developer)
	}

	if developer.Allows("run_command") {
		t.Error("the shipped tool list survived the edit")
	}

	if !strings.Contains(developer.Brief, "run the tests") {
		t.Errorf("the standing instructions were lost: %q", developer.Brief)
	}

	if developer.BuiltIn {
		t.Error("an edited agent still claims to be built in")
	}

	// And everybody else is still there.
	if _, ok := Find(roster, "writer"); !ok {
		t.Error("editing one agent removed the others")
	}
}

// A new name adds somebody rather than replacing anybody.
func TestANewNameAddsSomebody(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(Folder(root), "bookkeeper.md"),
		[]byte("for: invoices and what clients owe\nuses: quick\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	roster := Roster(root)

	if len(roster) != len(BuiltIn())+1 {
		t.Fatalf("the roster is %d", len(roster))
	}

	added, ok := Find(roster, "bookkeeper")
	if !ok {
		t.Fatal("the new agent is not there")
	}

	// A title nobody wrote is made from the name, so the interface never has
	// a blank where somebody's job should be.
	if added.Title == "" {
		t.Error("it has no title to show")
	}
}

// A file mid-edit is skipped, not fatal. These are hand-editable by design.
func TestOneBadAgentDoesNotEmptyTheTeam(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		t.Fatal(err)
	}

	// No "for:" line, so nothing could route to it.
	if err := os.WriteFile(filepath.Join(Folder(root), "half.md"),
		[]byte("name: half\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	roster := Roster(root)

	if len(roster) != len(BuiltIn()) {
		t.Errorf("the roster is %d, want the built-in %d", len(roster), len(BuiltIn()))
	}
}
