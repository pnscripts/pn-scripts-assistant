package setup

import (
	"context"
	"strings"
	"testing"
)

/*
 * The opening is written for this machine, or the page keeps its own words.
 *
 * Both halves are the design. A machine with a model on it gets an
 * introduction about itself; the first run of all — no model, which is what
 * the wizard is for — keeps the paragraphs the page ships with, and that is
 * the only honest answer when there is nothing to write with.
 */
func TestTheWelcomeIsWrittenOrLeftAlone(t *testing.T) {
	s := &Server{root: t.TempDir()}

	ctx, stop := context.WithTimeout(context.Background(), HowLongToWrite)
	defer stop()

	// Written here rather than through the handler, which never waits: this
	// is the writing itself, and a test that asserted the handler's first
	// answer would be asserting that nothing had been written yet.
	said := s.welcomeInWords(ctx)

	if said == "" {
		t.Skip("no model on this machine to write it with, which is the other half")
	}

	if strings.Contains(said, "TOOLS I HAVE") || strings.Contains(said, "On this machine, now:") {
		t.Errorf("the facts were handed over instead of being written up:\n%s", said)
	}

	if len(said) > 1200 {
		t.Errorf("the opening is %d characters, which nobody reads", len(said))
	}

	t.Logf("written for this machine:\n%s", said)
}
