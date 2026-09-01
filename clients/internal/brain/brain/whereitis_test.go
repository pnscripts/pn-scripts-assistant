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
