package server

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
)

/*
 * The introduction reaches the greeting, and stops after it is delivered.
 *
 * Tested here rather than in the brain package because the wiring is what
 * failed in practice: the line was composed correctly, every tool was written
 * down as mentioned, and then the greeting was consumed by something that
 * never showed it. The introduction was spent without being given, and nothing
 * could notice — as far as the program was concerned the job was done.
 */
func TestTheGreetingCarriesTheIntroductionUntilItIsDelivered(t *testing.T) {
	_, _, b := newServer(t)

	first := b.Greet()

	if first.Shown == "" {
		t.Fatal("the greeting carried no introduction on a brain that has never given one")
	}

	/*
	 * What it carries is read off the registry now, not out of a list of
	 * eighteen sentences written in Go — so this asserts that it describes
	 * the tools this brain actually has, and still says the two things
	 * somebody should know before asking for any of them.
	 */
	// "without stopping to confirm" on a brain set to act freely, which is
	// the default its owner chose; the sentence follows the setting rather
	// than being written here. See Brain.howItAsks.
	for _, want := range []string{"read_file", "run_command", "written down", "privacy panel"} {
		if !strings.Contains(first.Shown, want) {
			t.Errorf("the introduction is missing %q:\n%s", want, first.Shown)
		}
	}

	// And nothing from the list it replaced, which described eighteen tools
	// of seventy-odd and never mentioned the machine.
	if strings.Contains(first.Shown, "learn everything") {
		t.Error("the hardcoded capability list is still being shown")
	}

	// Building it twice must not spend it: only delivery does.
	if again := b.Greet(); again.Shown == "" {
		t.Fatal("composing the greeting a second time lost the introduction")
	}

	b.Delivered(first)

	if after := b.Greet(); after.Shown != "" {
		t.Fatal("it introduced itself again after having been introduced")
	}
}

// And a greeting with nothing to show never marks anything as given.
func TestAnEmptyGreetingSpendsNothing(t *testing.T) {
	_, _, b := newServer(t)

	b.Delivered(brain.Greeting{Text: "Good evening."})

	if b.Greet().Shown == "" {
		t.Fatal("a greeting with no introduction in it spent the introduction")
	}
}
