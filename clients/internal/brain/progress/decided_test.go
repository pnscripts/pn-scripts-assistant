package progress

import (
	"strings"
	"testing"
)

/*
 * A question that has been answered stops waiting in Doing now.
 *
 * The loop writes "Waiting for you: ..." when it stops for a decision, and
 * the feed showed that red row until some other work pushed it off, so a
 * question decided by hand, or by itself when the quiet hours opened, still
 * read as waiting long afterwards. The row belongs to one question: deciding
 * it settles that row, says how it was decided, and leaves any other
 * question that is still waiting exactly as it was.
 */
func TestADecidedQuestionStopsWaitingInDoingNow(t *testing.T) {
	fresh(t)

	Begin()
	Waiting("Waiting for you: Ask Claude Code", 7)
	Waiting("Waiting for you: Write notes.txt", 8)
	Done()

	if !Decided(7, ByItself) {
		t.Fatal("the row of question 7 was not found")
	}

	var first, second *Entry

	all := Recent()
	for i := range all {
		switch {
		case strings.HasSuffix(all[i].Note, "Ask Claude Code"):
			first = &all[i]
		case strings.HasSuffix(all[i].Note, "Write notes.txt"):
			second = &all[i]
		}
	}

	if first == nil || second == nil {
		t.Fatalf("a row went missing: %+v", all)
	}

	if first.Kind != KindDecided || first.Decided != ByItself || first.Note != "Started by itself: Ask Claude Code" {
		t.Errorf("the decided question still reads as waiting: %+v", *first)
	}

	if second.Kind != "waiting" || second.Note != "Waiting for you: Write notes.txt" || second.Decided != "" {
		t.Errorf("deciding one question settled another: %+v", *second)
	}

	// Each way of deciding says itself, and a question is decided once.
	Decided(8, Refused)

	for _, e := range Recent() {
		if e.Kind == "waiting" {
			t.Errorf("still waiting after both were decided: %+v", e)
		}

		if strings.HasSuffix(e.Note, "Write notes.txt") && e.Note != "Refused: Write notes.txt" {
			t.Errorf("a refusal is not said as one: %q", e.Note)
		}
	}

	if Decided(8, Approved) {
		t.Error("a question already decided was decided again")
	}

	if Decided(99, Approved) {
		t.Error("a question that never waited here was found")
	}
}

// An approval says so, and the duration of the wait is kept.
func TestAnApprovedQuestionSaysSo(t *testing.T) {
	fresh(t)

	Waiting("Waiting for you: Run the tests", 3)

	if !Decided(3, Approved) {
		t.Fatal("not found")
	}

	last := Recent()[len(Recent())-1]
	if last.Note != "Approved: Run the tests" || last.Took <= 0 {
		t.Errorf("%+v", last)
	}

	// The work goes on after the yes: a new step is a new row, the decided
	// one is not reopened.
	Set("tool", "Run the tests")

	all := Recent()
	if len(all) != 2 || all[0].Kind != KindDecided {
		t.Errorf("%+v", all)
	}
}

// Two questions in the same words are two rows: deciding one leaves the
// other waiting.
func TestTwoQuestionsInTheSameWordsAreDecidedApart(t *testing.T) {
	fresh(t)

	Waiting("Waiting for you: Run the tests", 4)
	Waiting("Waiting for you: Run the tests", 5)

	if !Decided(4, Refused) {
		t.Fatal("not found")
	}

	all := Recent()
	if len(all) != 2 || all[0].Kind != KindDecided || all[1].Kind != "waiting" {
		t.Errorf("%+v", all)
	}
}
