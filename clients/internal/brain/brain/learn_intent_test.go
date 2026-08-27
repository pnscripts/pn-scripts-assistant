package brain

import (
	"os"
	"path/filepath"
	"testing"
)

// An instruction to learn from something must be recognised.
func TestLearnInstructionIsRecognised(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, message := range []string{
		"learn from " + dir,
		"Learn everything you can from " + dir + " please",
		"index " + dir,
		"scan " + dir + " and remember what is in it",
	} {
		got := readLearnInstruction(message)

		if !got.Found || got.Path != dir {
			t.Errorf("%q gave %+v, want the path %q", message, got, dir)
		}
	}
}

// A link is recognised separately, because it cannot be read the same way.
func TestLearnFromALink(t *testing.T) {
	got := readLearnInstruction("learn from https://example.com/handbook")

	if !got.Found || got.Link != "https://example.com/handbook" {
		t.Errorf("got %+v, want the link", got)
	}
}

// Talking about learning is not an instruction to go and read something.
//
// This is the important half. "I want to learn Go" is a fact about its owner
// and a perfectly good thing to remember; treated as a command it would send
// the brain off to scan whatever path it could find, and the owner would never
// find out why.
func TestTalkingAboutLearningIsNotAnInstruction(t *testing.T) {
	for _, message := range []string{
		"I want to learn Go",
		"learning Rust has been slow going",
		"what have you learned today?",
		"learn from your mistakes",
		"scan through your memory and tell me about my projects",
	} {
		if got := readLearnInstruction(message); got.Found {
			t.Errorf("%q was taken as an instruction: %+v", message, got)
		}
	}
}

// A path that does not exist is not quietly swapped for one that does.
func TestAMistypedPathIsNotFound(t *testing.T) {
	if got := readLearnInstruction("learn from /no/such/place/anywhere"); got.Found {
		t.Errorf("a path that does not exist was accepted: %+v", got)
	}
}
