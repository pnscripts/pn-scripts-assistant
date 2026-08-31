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

/*
 * A link said out loud is still a link.
 *
 * Nobody says "aitch tee tee pee colon slash slash" — they say
 * "pnscripts.com" — and the recogniser writes down what they said. So an
 * instruction given by voice never matched, fell through to the model, and got
 * exactly the answer this file exists to prevent: "I will read that and learn
 * from it", followed by nothing at all. Which is worse than a refusal, because
 * it is indistinguishable from having worked, so nobody checks.
 */
func TestALinkSaidOutLoudIsLearnedFrom(t *testing.T) {
	for _, c := range []struct{ said, want string }{
		{"Brain, learn from pnscripts.com", "https://pnscripts.com"},
		{"learn everything from hosting.pnscripts.com", "https://hosting.pnscripts.com"},
		{"read this and learn: example.org/about", "https://example.org/about"},
		{"study dnevnik.bg please", "https://dnevnik.bg"},
		{"remember this page: docs.python.org", "https://docs.python.org"},

		// Pasted, with the protocol, as before.
		{"learn from https://pnscripts.com/docs", "https://pnscripts.com/docs"},
	} {
		got := readLearnInstruction(c.said)

		if !got.Found {
			t.Errorf("%q was not taken as an instruction to learn", c.said)

			continue
		}

		if got.Link != c.want {
			t.Errorf("%q gave link %q, want %q", c.said, got.Link, c.want)
		}
	}
}

/*
 * Ordinary sentences are not sent off to read the web.
 *
 * The matcher is deliberately narrow at both ends. Wrongly reading a page is a
 * request that leaves the machine, and doing it because somebody mentioned a
 * website in passing is exactly the kind of surprise this program must not
 * produce.
 */
func TestOrdinaryTalkIsNotAnInstructionToLearn(t *testing.T) {
	for _, innocent := range []string{
		"I want to learn Go this year",
		"what do you think about pnscripts.com?",
		"is example.org any good",
		"remember that I prefer tea",
		"scan the room for me",
	} {
		if got := readLearnInstruction(innocent); got.Found && got.Link != "" {
			t.Errorf("%q was taken as an instruction to go and read %q",
				innocent, got.Link)
		}
	}
}
