package appearance

import (
	"strings"
	"testing"
)

/*
 * Every status the interface shows can have its colour changed.
 *
 * The panel used to offer four while the interface drew eight, so half of them
 * were unchangeable — and the ones left out were exactly the ones somebody is
 * most likely to want moved, since "working out what you said" and "thinking"
 * sit next to each other in meaning and must be far apart in colour.
 */
func TestEveryStatusColourCanBeSet(t *testing.T) {
	dir := t.TempDir()
	store := Open(dir)

	statuses := []string{
		"idle", "listening", "hearing", "thinking", "tool",
		"speaking", "waiting", "learning", "thinking line",
	}

	for _, status := range statuses {
		if _, err := store.Set(status, "purple"); err != nil {
			t.Errorf("cannot colour %q: %v", status, err)
		}
	}

	look := store.Current()

	// And each one actually landed, rather than all writing to one field.
	for name, got := range map[string]string{
		"idle": look.Idle, "listening": look.Listening, "hearing": look.Hearing,
		"thinking": look.ThinkingCore, "tool": look.Tool, "speaking": look.Speaking,
		"waiting": look.Waiting, "learning": look.Learning,
		"thinking line": look.ThinkingLine,
	} {
		if got == "" {
			t.Errorf("%s was left empty after being set", name)
		}
	}
}

// A colour it cannot place says which ones it can, rather than a bare refusal.
func TestAnUnknownPartSaysWhatItCanColour(t *testing.T) {
	store := Open(t.TempDir())

	_, err := store.Set("the wallpaper", "green")
	if err == nil {
		t.Fatal("colouring something that does not exist was accepted")
	}

	for _, named := range []string{"hearing", "tool", "waiting", "learning"} {
		if !strings.Contains(err.Error(), named) {
			t.Errorf("the refusal does not mention %q as something it can colour: %v",
				named, err)
		}
	}
}
