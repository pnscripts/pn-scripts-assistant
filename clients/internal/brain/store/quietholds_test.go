package store

import "testing"

/*
 * A question held for the quiet hours and nothing else is marked to answer
 * itself when they open. Only a pending one can be marked or answered that
 * way, only once, and an answer it gave itself is not its user's habit.
 */
func TestAHeldQuestionAnswersItselfOnceAndOnlyFromWaiting(t *testing.T) {
	db := open(t)

	held, err := db.RecordInvocation(7, "ask_coding_agent", `{"task":"x"}`, "Ask Claude Code", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	asked, err := db.RecordInvocation(7, "ask_coding_agent", `{"task":"y","access":"full"}`, "Ask it, full", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AnswersItselfInQuietHours(held); err != nil {
		t.Fatal(err)
	}

	waiting, err := db.WaitingForQuietHours()
	if err != nil {
		t.Fatal(err)
	}

	if len(waiting) != 1 || waiting[0].ID != held || !waiting[0].AnswersItself || waiting[0].ConversationID != 7 {
		t.Fatalf("waiting for the quiet hours: %+v", waiting)
	}

	pending, err := db.PendingInvocations()
	if err != nil {
		t.Fatal(err)
	}

	if len(pending) != 2 || !pending[0].AnswersItself || pending[1].AnswersItself {
		t.Fatalf("the page cannot tell which answers itself: %+v", pending)
	}

	// One that would have been asked anyway never answers itself.
	if moved, err := db.AnswerItself(asked); err != nil || moved {
		t.Fatalf("an unmarked question answered itself: %v %v", moved, err)
	}

	if moved, err := db.AnswerItself(held); err != nil || !moved {
		t.Fatalf("the held question did not answer itself: %v %v", moved, err)
	}

	if moved, _ := db.AnswerItself(held); moved {
		t.Fatal("answered itself twice")
	}

	inv, err := db.Invocation(held)
	if err != nil || inv == nil {
		t.Fatal(err)
	}

	if inv.Status != InvocationApproved || !inv.AnsweredItself {
		t.Fatalf("%+v", inv)
	}

	if err := db.CompleteInvocation(held, InvocationDone, "Done."); err != nil {
		t.Fatal(err)
	}

	// Its user answering first wins: a decided question is never marked.
	if moved, _ := db.DecideInvocation(asked, InvocationDenied); !moved {
		t.Fatal("deny")
	}

	if err := db.AnswersItselfInQuietHours(asked); err != nil {
		t.Fatal(err)
	}

	if waiting, _ := db.WaitingForQuietHours(); len(waiting) != 0 {
		t.Fatalf("a decided question waits: %+v", waiting)
	}

	// Habits are what a person decided: one refusal, no approval.
	habits, err := db.Habits()
	if err != nil {
		t.Fatal(err)
	}

	if len(habits) != 1 || habits[0].Approved != 0 || habits[0].Refused != 1 {
		t.Fatalf("an answer it gave itself counted as its user's: %+v", habits)
	}
}

// Taken back: a question that no longer answers itself (its user changed a
// permission while it waited) waits for its user like any other.
func TestAHeldQuestionCanStopAnsweringItself(t *testing.T) {
	db := open(t)

	id, _ := db.RecordInvocation(1, "ask_coding_agent", `{}`, "Ask", "mutating")

	if err := db.AnswersItselfInQuietHours(id); err != nil {
		t.Fatal(err)
	}

	if err := db.NoLongerAnswersItself(id); err != nil {
		t.Fatal(err)
	}

	if moved, _ := db.AnswerItself(id); moved {
		t.Fatal("answered itself after being taken back")
	}

	if pending, _ := db.PendingInvocations(); len(pending) != 1 || pending[0].AnswersItself {
		t.Fatalf("%+v", pending)
	}
}
