package tools

import (
	"slices"
	"testing"
)

/*
 * What a tool needs is data, not something learned by failing.
 *
 * Calling godot_build on a machine without Godot to find out there is no
 * Godot costs a model's turn — minutes, on a machine with no graphics card —
 * to learn something that could be read off a list.
 */
func TestToolsSayWhatTheyCannotWorkWithout(t *testing.T) {
	registry := NewRegistry(GodotStatus{}, TypeText{}, ReadFile{})

	if got := registry.Needing("godot_status"); !slices.Equal(got, []string{"godot"}) {
		t.Errorf("godot_status needs %v", got)
	}

	if got := registry.Needing("type_text"); !slices.Equal(got, []string{"xdotool"}) {
		t.Errorf("type_text needs %v", got)
	}

	// Most tools need nothing beyond this program: reading a file is this
	// program reading a file.
	if got := registry.Needing("read_file"); len(got) != 0 {
		t.Errorf("read_file claims to need %v", got)
	}

	// Asked about all of it at once, because sixty questions to the machine
	// is sixty processes and the same answer.
	all := registry.WhatIsNeeded()

	if !slices.Contains(all, "godot") || !slices.Contains(all, "xdotool") {
		t.Errorf("the whole list is %v", all)
	}

	if !slices.IsSorted(all) {
		t.Errorf("the list is not in a fixed order: %v", all)
	}
}

// Something added at run time answers for itself, and beats the table — the
// same rule as Serves, because a skill replacing a name is how a skill
// overrides one.
func TestSomethingAddedLaterSaysWhatItNeeds(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(needy{}); err != nil {
		t.Fatal(err)
	}

	if got := registry.Needing("godot_status"); !slices.Equal(got, []string{"unreal"}) {
		t.Errorf("the tool's own answer was ignored: %v", got)
	}
}

// needy stands in for a skill that replaces a built-in name.
type needy struct{ GodotStatus }

func (needy) Needs() []string { return []string{"unreal"} }
