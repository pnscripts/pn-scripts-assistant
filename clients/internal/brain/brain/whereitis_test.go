package brain

import (
	"os"

	"pn-brain/internal/brain/config"
	"strings"
	"testing"
)

/*
 * An assistant that can read the machine has to be told what the machine is.
 *
 * Told it had tools and never told where anything lives, "check my projects"
 * became unanswerable: no path in the question and none in its head. What it
 * did then was worse than asking — it called the directory tool with
 * "/absolute/path/to/your/projects", a placeholder out of a documentation
 * example, and reported the failure as though it had looked somewhere real.
 */
func TestTheAssistantIsToldWhereThingsAre(t *testing.T) {
	b := &Brain{Root: t.TempDir()}

	where := b.whereThingsAre()

	if where == "" {
		t.Fatal("it is told nothing about the machine it is running on")
	}

	if !strings.Contains(where, b.Root) {
		t.Error("it is not told where its own memory lives")
	}

	if home, err := os.UserHomeDir(); err == nil && !strings.Contains(where, home) {
		t.Error("it is not told the home folder, which is where somebody's work is")
	}
}

/*
 * And told not to invent one.
 *
 * A placeholder passed to a real tool produces a confident failure about a
 * directory nobody has, which reads as the machine being wrong rather than the
 * path being made up.
 */
func TestTheAssistantIsToldNotToInventPaths(t *testing.T) {
	b := &Brain{Root: t.TempDir(), Cfg: config.Config{Name: "PN Brain", Owner: "Petar"}}

	/*
	 * Whitespace collapsed before matching.
	 *
	 * The prompt is hard-wrapped, so any phrase long enough to be worth
	 * asserting is split across a line break somewhere — a test that matches
	 * the raw text fails on the wrapping rather than on the meaning, and then
	 * gets "fixed" by asserting something shorter and weaker.
	 */
	prompt := strings.Join(strings.Fields(b.SystemPrompt()), " ")

	for _, want := range []string{
		"never say you have no access to files",
		"never call a tool with an example path",
	} {
		if !strings.Contains(strings.ToLower(prompt), want) {
			t.Errorf("the prompt does not say: %q", want)
		}
	}

	if !strings.Contains(prompt, "Where things are on this machine") {
		t.Error("the prompt does not carry the real paths into the conversation")
	}
}

/*
 * The prompt itself says how much is remembered, so no answer can claim none.
 *
 * "I know nothing about you. I have no memory of our conversations. I am a
 * fresh start" — said while holding 1029 things about the person asking.
 * Getting that right depended on three things going right in a row: the
 * question keeping its tools, the model choosing what_you_know, and it calling
 * the tool rather than announcing it. The third failed after the first two
 * were fixed, which is why this does not depend on any of them.
 */
func TestThePromptAlwaysCarriesHowMuchIsRemembered(t *testing.T) {
	b := testBrain(t)

	// Nothing learned: it must say that plainly rather than imply a number.
	empty := b.whatIsRemembered()

	if !strings.Contains(empty, "learned nothing yet") {
		t.Errorf("an empty brain describes itself as %q", empty)
	}

	for i := 0; i < 3; i++ {
		if _, err := b.DB.AddFact("project", "Petar has a project", nil); err != nil {
			t.Skipf("cannot add facts here: %v", err)
		}
	}

	said := b.whatIsRemembered()

	if !strings.Contains(said, "3") {
		t.Errorf("the count is missing: %q", said)
	}

	if !strings.Contains(said, "Never say you have no memory") {
		t.Errorf("the prompt does not forbid the false answer: %q", said)
	}

	// And it reaches the prompt the model actually sees.
	prompt := strings.Join(strings.Fields(b.SystemPrompt()), " ")

	if !strings.Contains(prompt, "Never say you have no memory") {
		t.Error("what is remembered never reaches the system prompt")
	}
}
