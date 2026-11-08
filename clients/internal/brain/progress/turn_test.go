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

// NextTurn is how background work learns, mid-call, that somebody asked
// something: closed when a turn starts, and only then.
func TestNextTurnIsClosedWhenAQuestionStarts(t *testing.T) {
	for InATurn() {
		FinishedATurn()
	}

	next := NextTurn()

	select {
	case <-next:
		t.Fatal("closed before any question was asked")
	default:
	}

	// Finishing a turn that was never started is not a question.
	FinishedATurn()

	select {
	case <-next:
		t.Fatal("closed by a turn ending")
	default:
	}

	StartedATurn()

	select {
	case <-next:
	default:
		t.Fatal("not closed when a question started")
	}

	// Asked again, it waits for the next question, not this one.
	again := NextTurn()

	select {
	case <-again:
		t.Fatal("a fresh wait was already over")
	default:
	}

	FinishedATurn()
}

// NoTurn wakes background work the moment it may carry on: when the last of
// several overlapping turns ends, not the first.
func TestNoTurnWaitsForTheLastQuestion(t *testing.T) {
	for InATurn() {
		FinishedATurn()
	}

	select {
	case <-NoTurn():
	default:
		t.Fatal("not over with no question running")
	}

	StartedATurn()
	StartedATurn()

	over := NoTurn()

	FinishedATurn()

	select {
	case <-over:
		t.Fatal("over while a second question was still being answered")
	default:
	}

	FinishedATurn()

	select {
	case <-over:
	default:
		t.Fatal("not over when the last question finished")
	}
}

/*
 * Background work ending must not end the answer.
 *
 * Reading a folder set the current step, a question began meanwhile, and the
 * reading's Done then cleared the question's step: the page showed an idle
 * brain halfway through an answer.
 */
func TestBackgroundWorkEndingLeavesTheAnswerAlone(t *testing.T) {
	Done()

	SetBackground("learning", "Reading Documents")
	Begin()
	Set("thinking", "Thinking")

	BackgroundDone()

	step := Now()
	if !step.Busy || step.Kind != "thinking" {
		t.Fatalf("the answer's step was cleared by background work: %+v", step)
	}

	Done()

	// On its own, background work does clear what it set.
	SetBackground("learning", "Reading Documents")
	BackgroundDone()

	if Now().Busy {
		t.Error("background work left its step behind")
	}
}

// Aside never writes over a step somebody is waiting on, and otherwise says
// what the background work is doing.
func TestAsideLeavesTheAnswerOnScreen(t *testing.T) {
	Done()

	Begin()
	Set("thinking", "Thinking")

	Aside("learning", "3 new things from Documents")

	if step := Now(); step.Kind != "thinking" || step.Note != "Thinking" || step.Background {
		t.Fatalf("background work wrote over the answer: %+v", step)
	}

	Done()

	Aside("learning", "Reading Documents")

	if step := Now(); !step.Busy || !step.Background || step.Note != "Reading Documents" {
		t.Errorf("background work on its own was not shown: %+v", step)
	}

	// One background note may replace another.
	Aside("learning", "2 new things from Documents")

	if step := Now(); step.Note != "2 new things from Documents" {
		t.Errorf("the newer background note was not shown: %+v", step)
	}

	BackgroundDone()
}
