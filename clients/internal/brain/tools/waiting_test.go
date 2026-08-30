package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fakeQueue struct {
	actions  []QueueItem
	lessons  []QueueItem
	decided  []string
	failWith error
}

func (q *fakeQueue) PendingActions() ([]QueueItem, error) { return q.actions, nil }
func (q *fakeQueue) PendingLessons() ([]QueueItem, error) { return q.lessons, nil }

// Mirrors the real adapter, which names what it did rather than counting it.
func (q *fakeQueue) DecideAction(_ context.Context, id int64, approve bool) (string, error) {
	if q.failWith != nil {
		return "", q.failWith
	}

	q.decided = append(q.decided, fmt.Sprintf("action %d approve=%v", id, approve))

	for _, it := range q.actions {
		if it.ID == id {
			return "did: " + it.Summary, nil
		}
	}

	return fmt.Sprintf("did: action %d", id), nil
}

func (q *fakeQueue) DecideLesson(_ context.Context, id int64, keep bool) (string, error) {
	q.decided = append(q.decided, fmt.Sprintf("lesson %d keep=%v", id, keep))

	return fmt.Sprintf("remembered lesson %d", id), nil
}

/*
 * "Approve everything and remember everything" has to actually do it.
 *
 * Said out loud to a brain with seven things waiting, this produced "I will
 * approve everything and remember everything for you" and then nothing
 * whatsoever, because there was no tool for the queue and the model could only
 * talk about it. A promise it cannot keep is worse than a refusal.
 */
func TestApprovingEverythingDecidesEverything(t *testing.T) {
	q := &fakeQueue{
		actions: []QueueItem{{ID: 1, Summary: "create greetings.txt"}},
		lessons: []QueueItem{{ID: 7, Summary: "prefers a file called greetings.txt"}, {ID: 8}},
	}

	out, err := DecideWaiting{Queue: q}.Execute(context.Background(),
		json.RawMessage(`{"decision":"approve"}`))
	if err != nil {
		t.Fatal(err)
	}

	if len(q.decided) != 3 {
		t.Fatalf("decided %d of the 3 things waiting: %v", len(q.decided), q.decided)
	}

	for _, want := range []string{"action 1 approve=true", "lesson 7 keep=true", "lesson 8 keep=true"} {
		var found bool

		for _, got := range q.decided {
			if got == want {
				found = true
			}
		}

		if !found {
			t.Errorf("%q was left waiting", want)
		}
	}

	// Named, not counted: this is the record of what somebody agreed to
	// without reading it themselves.
	if !strings.Contains(out, "greetings.txt") {
		t.Errorf("the answer does not say what was approved: %q", out)
	}
}

// One item at a time, when that is what was asked for.
func TestDecidingASingleItem(t *testing.T) {
	q := &fakeQueue{
		actions: []QueueItem{{ID: 1}, {ID: 2}},
		lessons: []QueueItem{{ID: 7}},
	}

	if _, err := (DecideWaiting{Queue: q}).Execute(context.Background(),
		json.RawMessage(`{"decision":"reject","id":2}`)); err != nil {
		t.Fatal(err)
	}

	if len(q.decided) != 1 || q.decided[0] != "action 2 approve=false" {
		t.Fatalf("deciding one item touched %v", q.decided)
	}
}

// Only the memories, when that is what was asked for.
func TestDecidingOnlyOneQueue(t *testing.T) {
	q := &fakeQueue{
		actions: []QueueItem{{ID: 1}},
		lessons: []QueueItem{{ID: 7}},
	}

	if _, err := (DecideWaiting{Queue: q}).Execute(context.Background(),
		json.RawMessage(`{"decision":"approve","kind":"memories"}`)); err != nil {
		t.Fatal(err)
	}

	if len(q.decided) != 1 || q.decided[0] != "lesson 7 keep=true" {
		t.Fatalf("asked for memories only, decided %v", q.decided)
	}
}

/*
 * The words people and small models actually use.
 *
 * A 7B model told to approve something writes "accept", "yes", "keep" and
 * "remember" about as often as the word in the schema. Refusing those is a tool
 * that fails for no reason its owner can see — they said approve, and nothing
 * happened, again.
 */
func TestTheWordsAskedForAreUnderstood(t *testing.T) {
	for _, word := range []string{"approve", "accept", "yes", "keep", "remember", "ok", "Approve"} {
		if yes, err := readDecision(word); err != nil || !yes {
			t.Errorf("%q was not understood as yes: %v %v", word, yes, err)
		}
	}

	for _, word := range []string{"reject", "deny", "no", "discard", "forget"} {
		if yes, err := readDecision(word); err != nil || yes {
			t.Errorf("%q was not understood as no: %v %v", word, yes, err)
		}
	}

	if _, err := readDecision("maybe"); err == nil {
		t.Error("a word that is neither was taken as a decision")
	}
}

// An action that could not be carried out must not be reported as done.
func TestAFailureIsNotReportedAsSuccess(t *testing.T) {
	q := &fakeQueue{
		actions:  []QueueItem{{ID: 1, Summary: "create greetings.txt"}},
		failWith: fmt.Errorf("permission denied"),
	}

	out, err := DecideWaiting{Queue: q}.Execute(context.Background(),
		json.RawMessage(`{"decision":"approve","kind":"actions"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "permission denied") {
		t.Errorf("a failure was not reported back: %q", out)
	}
}
