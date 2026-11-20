package brain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * What another program asked for is answered on its card, not in words.
 *
 * "Approve everything waiting", said or typed, goes through the model and
 * the queue tool. A conversation can be started by any program on this
 * computer, so a program that asked the assistant to change something could
 * have said yes to its own question that way. The same rule as for its
 * permission questions: the card, and nothing else.
 */
func TestWhatAnotherProgramAskedForIsNotAnsweredInWords(t *testing.T) {
	b := testBrain(t)

	theirs := filepath.Join(t.TempDir(), "asked-by-a-program.txt")
	args, _ := json.Marshal(map[string]string{"path": theirs, "content": "nobody pressed Approve"})

	asked, err := b.DB.RecordInvocationAskedBy(0, "write_file", string(args), "Create "+theirs, "", "mutating", "Cursor 3.0")
	if err != nil {
		t.Fatal(err)
	}

	mine := filepath.Join(t.TempDir(), "asked-at-the-desk.txt")
	args, _ = json.Marshal(map[string]string{"path": mine, "content": "said yes in words"})

	own, err := b.DB.RecordInvocation(0, "write_file", string(args), "Create "+mine, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	actions, err := b.PendingActions()
	if err != nil {
		t.Fatal(err)
	}

	if len(actions) != 1 || actions[0].ID != own {
		t.Errorf("what is offered to be answered in words: %+v, want only the desk's own question %d", actions, own)
	}

	if _, err := b.DecideAction(context.Background(), asked, true); err == nil || !strings.Contains(err.Error(), "on its card") {
		t.Errorf("a question another program asked for was answered by its number: %v", err)
	}

	if _, err := os.Stat(theirs); err == nil {
		t.Error("the file another program asked for was written without its card")
	}

	waiting, _ := b.DB.PendingInvocations()
	if len(waiting) != 2 {
		t.Errorf("%d questions wait, want both", len(waiting))
	}

	// The card still answers it, and the desk's own question is answered
	// in words as before.
	if _, err := b.Decide(context.Background(), asked, false); err != nil {
		t.Errorf("the card could not refuse it: %v", err)
	}

	if said, err := b.DecideAction(context.Background(), own, true); err != nil || !strings.Contains(said, mine) {
		t.Errorf("the desk's own question was not answered in words: %q %v", said, err)
	}

	if _, err := os.Stat(mine); err != nil {
		t.Error("the desk's own file was not written")
	}
}
