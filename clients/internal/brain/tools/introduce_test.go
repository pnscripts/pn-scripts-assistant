package tools

import (
	"context"
	"strings"
	"testing"
)

/*
 * What it can do is read off what it has, not out of a list somebody typed.
 *
 * The eighteen hand-written lines this replaced described eighteen of
 * seventy-odd tools, said nothing about the machine, and had to be edited by
 * hand whenever the program learned something. An assistant whose account of
 * itself is a string constant is one that will eventually be lying.
 */
func TestItDescribesTheToolsItActuallyHas(t *testing.T) {
	registry := NewRegistry(ReadFile{}, WriteFile{}, RunCommand{}, GodotStatus{})

	said, err := Introduce{
		Loaded:    func() []string { return loadedIn(registry) },
		Describes: func(name string) string { tool, _ := registry.Get(name); return tool.Description() },
		Here:      func() string { return "On this machine, now:\nGame engines: Godot 4.7.1" },
	}.Facts()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"read_file", "write_file", "run_command", "godot_status"} {
		if !strings.Contains(said, want) {
			t.Errorf("%s is loaded and not mentioned:\n%s", want, said)
		}
	}

	// Grouped by what somebody wants, not by function name.
	for _, want := range []string{"Files and folders", "Code and projects", "Making games"} {
		if !strings.Contains(said, want) {
			t.Errorf("no heading %q:\n%s", want, said)
		}
	}

	// And the machine, which the old list never mentioned at all.
	if !strings.Contains(said, "Godot 4.7.1") {
		t.Errorf("it does not say what is on the machine:\n%s", said)
	}

	// Nothing claimed that is not loaded.
	if strings.Contains(said, "send_email") {
		t.Errorf("it named a tool that is not loaded:\n%s", said)
	}
}

// A description is cut at a word, and never left as the start of a clause.
func TestADescriptionIsCutWhereItReads(t *testing.T) {
	long := "change wording inside a document that already exists — the whole of it, or one paragraph"

	got := describe("edit_document", func(string) string { return long })

	if strings.Contains(got, "— )") || strings.HasSuffix(got, "— ") {
		t.Errorf("it left the start of a clause: %q", got)
	}

	if strings.HasSuffix(strings.TrimSuffix(got, ")"), " ") {
		t.Errorf("it left a trailing space: %q", got)
	}

	if !strings.HasPrefix(got, "edit_document (") || !strings.HasSuffix(got, ")") {
		t.Errorf("it is not a name and a description: %q", got)
	}
}

/*
 * The model's copy carries an instruction; the person's copy does not.
 *
 * Both exist because both readers exist: the model is told to say this in its
 * own words, and a person seeing it — when there is no model to phrase it —
 * should not be reading a note addressed to somebody else.
 */
func TestTheModelIsToldWhatToDoWithItAndAPersonIsNot(t *testing.T) {
	registry := NewRegistry(ReadFile{})

	build := Introduce{Loaded: func() []string { return loadedIn(registry) }}

	forModel, err := build.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	forPerson, err := build.Facts()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(forModel, "in your own words") {
		t.Error("the model was not told what to do with it")
	}

	if strings.Contains(forPerson, "in your own words") {
		t.Errorf("a person is shown a note written to a model:\n%s", forPerson)
	}

	if !strings.Contains(forModel, "read_file") || !strings.Contains(forPerson, "read_file") {
		t.Error("the two copies disagree about what is loaded")
	}
}

// With nothing loaded it says so plainly rather than describing an assistant
// in general.
func TestWithNothingLoadedItSaysSo(t *testing.T) {
	said, err := Introduce{Loaded: func() []string { return nil }}.Facts()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "talk and nothing else") {
		t.Errorf("it said: %q", said)
	}
}

func loadedIn(r *Registry) []string {
	var out []string

	for _, tool := range r.All() {
		out = append(out, tool.Name())
	}

	return out
}

/*
 * The shape is what an introduction is written from.
 *
 * The full list is four thousand characters, which a model on a machine with
 * no graphics card reads at about eight tokens a second before writing a
 * word. To introduce itself it needs the shape of what it has; somebody who
 * actually asks gets the list.
 */
func TestTheShapeIsShortEnoughToWriteFrom(t *testing.T) {
	registry := NewRegistry(ReadFile{}, WriteFile{}, RunCommand{}, GodotStatus{}, GodotDocs{}, SearchFiles{})

	build := Introduce{
		Loaded:    func() []string { return loadedIn(registry) },
		Describes: func(name string) string { tool, _ := registry.Get(name); return tool.Description() },
		Here:      func() string { return "On this machine, now:\nGame engines: Godot 4.7.1" },
	}

	shape, err := build.Shape()
	if err != nil {
		t.Fatal(err)
	}

	full, err := build.Facts()
	if err != nil {
		t.Fatal(err)
	}

	if len(shape) >= len(full) {
		t.Errorf("the shape (%d) is not shorter than the list (%d)", len(shape), len(full))
	}

	// It still says how much there is, what kinds, and what is on the machine.
	for _, want := range []string{"6 tools", "Making games (2)", "Godot 4.7.1"} {
		if !strings.Contains(shape, want) {
			t.Errorf("the shape does not say %q:\n%s", want, shape)
		}
	}

	// And no tool descriptions, which is the whole saving.
	if strings.Contains(shape, "run a command on the computer") {
		t.Errorf("the shape carries descriptions:\n%s", shape)
	}
}
