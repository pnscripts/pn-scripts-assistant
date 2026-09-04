package brain

import (
	"strings"
	"testing"
	"time"
)

/*
 * Coming back after five minutes is not an arrival.
 *
 * The greeting is written for opening the program after a while, and delivered
 * every launch it turns a conversation into a machine rebooting at somebody.
 * During an afternoon of restarts it was said eight times in an hour, word for
 * word, to a person who had never left the room.
 */
func TestItDoesNotGreetSomebodyWhoNeverLeft(t *testing.T) {
	line := lastTopicLine(&LastTalk{
		Topic: "fixing the core background",
		When:  time.Now().Add(-5 * time.Minute),
	}, true)

	// Under two minutes it says nothing about the topic; at five it does, and
	// that is the useful part of the greeting.
	if line == "" {
		t.Fatal("said nothing at all about a conversation five minutes old")
	}

	if !strings.Contains(line, "Carrying on") {
		t.Fatalf("did not carry on: %q", line)
	}
}

// And the recital about how much it knows is for somebody arriving, not for
// somebody who has been here all along.
func TestJustCameBackIsShorterThanHalfAnHour(t *testing.T) {
	if JustCameBack > 30*time.Minute {
		t.Fatalf("JustCameBack is %v, which would silence a real hello", JustCameBack)
	}

	if JustCameBack <= TooRecentToMention {
		t.Fatal("coming back must cover a longer span than being told the topic again")
	}
}
