package store

import (
	"strings"
	"testing"
)

/*
 * A killed turn leaves a question with nothing after it.
 *
 * The reply is written when a turn finishes, and a turn that was killed never
 * finished — so the transcript says the brain was asked something and ignored
 * it. Nothing in the process could have written that note, because the process
 * had gone; starting up is the first moment anything can.
 */
func TestQuestionsLeftHangingAreClosedOnStartup(t *testing.T) {
	db := open(t)

	// One that was cut short: a question and nothing after it.
	cut, err := db.NewConversation("learn everything")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(cut, "user", "", "", "learn everything in DEV"); err != nil {
		t.Fatal(err)
	}

	// And one that finished properly, which must not be touched.
	done, err := db.NewConversation("hello")
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range []struct{ role, text string }{
		{"user", "hello"}, {"assistant", "Hello, Petar."},
	} {
		if _, err := db.AddMessage(done, m.role, "", "", m.text); err != nil {
			t.Fatal(err)
		}
	}

	closed, err := db.FinishAbandonedTurns()
	if err != nil {
		t.Fatal(err)
	}

	if closed != 1 {
		t.Errorf("closed %d conversations, want just the unfinished one", closed)
	}

	cutMessages, err := db.History(cut)
	if err != nil {
		t.Fatal(err)
	}

	last := cutMessages[len(cutMessages)-1]

	if last.Role != "assistant" || !strings.Contains(last.Content, "cut short") {
		t.Errorf("the unfinished turn was not closed off: %+v", last)
	}

	// The finished one is left exactly as it was.
	doneMessages, err := db.History(done)
	if err != nil {
		t.Fatal(err)
	}

	if len(doneMessages) != 2 {
		t.Errorf("a finished conversation was written to: %d messages", len(doneMessages))
	}

	// And running again changes nothing: the last word is no longer a question.
	again, err := db.FinishAbandonedTurns()
	if err != nil {
		t.Fatal(err)
	}

	if again != 0 {
		t.Errorf("a second start closed %d more, so notes would pile up", again)
	}
}
