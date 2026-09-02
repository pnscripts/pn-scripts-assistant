package brain

import (
	"strings"
	"testing"
	"time"
)

// said is a previous conversation of a given age, which is what every test
// here is really about.
func said(topic string, age time.Duration) *LastTalk {
	return &LastTalk{ID: 7, Topic: topic, When: time.Now().Add(-age), Turns: 4}
}

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
	if line := lastTopicLine(said("the DEV folder", time.Second), true); line != "" {
		t.Errorf("a conversation from a moment ago was read back: %q", line)
	}

	line := lastTopicLine(said("the DEV folder", time.Hour), true)

	if !strings.Contains(line, "the DEV folder") {
		t.Errorf("it does not say what we were on: %q", line)
	}

	// Nothing to say about a conversation with no opening line, and nothing to
	// say on a brain that has never been talked to.
	if got := lastTopicLine(said("   ", time.Hour), true); got != "" {
		t.Errorf("an empty topic produced %q", got)
	}

	if got := lastTopicLine(nil, false); got != "" {
		t.Errorf("a brain with no history produced %q", got)
	}
}

/*
 * And it says which conversation you are now in.
 *
 * Opening the program dropped you back into the last conversation whatever its
 * age, without saying so — so a sentence typed on Tuesday joined a thread from
 * Friday night, and the history stopped being a record of what was discussed
 * when. Either behaviour is defensible. Not saying which is not.
 */
func TestItSaysWhetherThisIsTheSameConversationOrANewOne(t *testing.T) {
	carrying := lastTopicLine(said("the DEV folder", 20*time.Minute), true)

	if !strings.Contains(strings.ToLower(carrying), "carrying on") {
		t.Errorf("carrying on is not said: %q", carrying)
	}

	fresh := lastTopicLine(said("the DEV folder", 30*time.Hour), false)

	if !strings.Contains(strings.ToLower(fresh), "new conversation") {
		t.Errorf("a fresh start is not said: %q", fresh)
	}

	// And both say when, because "the last one" is only useful with a when.
	for _, line := range []string{carrying, fresh} {
		if !strings.Contains(line, "ago") && !strings.Contains(line, "yesterday") {
			t.Errorf("it does not say how long ago: %q", line)
		}
	}
}

/*
 * The rule itself: an interruption is not the end of a conversation, and a day
 * away is not a continuation of one.
 */
func TestWhatCountsAsTheSameConversation(t *testing.T) {
	b := &Brain{}

	if !b.carryingOn(said("something", CarryOnWithin-time.Minute)) {
		t.Error("a restart mid-conversation started a new one")
	}

	if b.carryingOn(said("something", CarryOnWithin+time.Minute)) {
		t.Error("coming back the next day carried on with yesterday's thread")
	}

	if b.carryingOn(nil) {
		t.Error("a brain with no history claimed to be carrying one on")
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

	line := lastTopicLine(said(long, time.Hour), true)

	if len(line) > 130 {
		t.Errorf("the greeting is %d characters: %q", len(line), line)
	}

	if !strings.Contains(line, "…") {
		t.Errorf("a cut topic does not show it was cut: %q", line)
	}
}
