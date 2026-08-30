package progress

import "testing"

/*
 * Work the brain gave itself is told apart from work somebody is waiting on.
 *
 * The core is coloured by this. Learning from the last conversation runs a
 * minute after every exchange, so without the distinction its owner opens the
 * program and finds it already amber and apparently thinking about a question
 * nobody asked.
 */
func TestBackgroundWorkIsMarkedAsSuch(t *testing.T) {
	Done()

	SetBackground("learning", "Learning from the last conversation")

	step := Now()

	if !step.Busy {
		t.Fatal("background work is not reported at all")
	}

	if !step.Background {
		t.Error("learning after a conversation is presented as work somebody is waiting for")
	}

	if step.Note != "Learning from the last conversation" {
		t.Errorf("it does not say what it is doing: %q", step.Note)
	}

	// And anything somebody is waiting on clears the flag again, so a turn
	// following background work is not mistaken for more of it.
	Set("thinking", "Thinking")

	if Now().Background {
		t.Error("a turn after background work was still marked as background")
	}

	Begin()

	if Now().Background {
		t.Error("beginning a turn left the background mark set")
	}

	Done()
}
