package progress

import "testing"

/*
 * A turn is a fact about the conversation, not about the microphone.
 *
 * The current step alternates between answering and listening several times a
 * second while the brain works, because the microphone reopens as soon as an
 * answer starts being written. Anything reading that to decide whether the
 * brain is busy flickers, and a person watching cannot tell a long task from an
 * idle room.
 */
func TestATurnOutlastsWhateverTheMicrophoneIsDoing(t *testing.T) {
	for InATurn() {
		FinishedATurn()
	}

	if InATurn() {
		t.Fatal("a turn was already running before the test started")
	}

	StartedATurn()

	// Everything the listening loop does to the current step, while a turn is
	// in flight. None of it may end the turn.
	Set("listening", "Listening")
	Set("transcribing", "Making out the words")
	Done()

	if !InATurn() {
		t.Error("the microphone ended the turn it was talking over")
	}

	FinishedATurn()

	if InATurn() {
		t.Error("the turn did not end when it finished")
	}

	// A spoken turn can begin while a typed one is still finishing, so the
	// first to end must not declare both over.
	StartedATurn()
	StartedATurn()
	FinishedATurn()

	if !InATurn() {
		t.Error("one turn ending ended the other")
	}

	FinishedATurn()

	// And a stray call cannot leave the interface insisting work goes on.
	FinishedATurn()

	if InATurn() {
		t.Error("the count went below zero and stuck")
	}
}
