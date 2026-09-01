package brain

import (
	"strings"
	"testing"
	"time"
)

/*
 * Coming back to a conversation, the first thing worth hearing is what it was
 * about.
 *
 * The greeting opened with the review queue — "there is one thing I would like
 * to remember" — which is housekeeping, and housekeeping is not what somebody
 * wants first on returning to something they were part way through.
 */
func TestTheGreetingSaysWhatWeWereOn(t *testing.T) {
	// Just said: reading it back describes the screen they are looking at.
	if line := topicLine("the DEV folder", time.Second); line != "" {
		t.Errorf("a conversation from a moment ago was read back: %q", line)
	}

	line := topicLine("the DEV folder", time.Hour)

	if !strings.Contains(line, "the DEV folder") {
		t.Errorf("it does not say what we were on: %q", line)
	}

	// Nothing to say about a conversation with no opening line.
	if got := topicLine("   ", time.Hour); got != "" {
		t.Errorf("an empty topic produced %q", got)
	}
}

/*
 * A long opening line is cut where it can still be read.
 *
 * Conversations are named by their first sentence, and a first sentence can be
 * a paragraph — an unclipped one would be the whole greeting, spoken aloud.
 */
func TestALongTopicIsCutShort(t *testing.T) {
	long := "I want you to learn from every documentation inside " +
		"/media/petar/c8fc2986-4b79-4d7b-9a8c-e6db653915ac/DEV and tell me about it"

	line := topicLine(long, time.Hour)

	if len(line) > 100 {
		t.Errorf("the greeting is %d characters: %q", len(line), line)
	}

	if !strings.Contains(line, "…") {
		t.Errorf("a cut topic does not show it was cut: %q", line)
	}
}
