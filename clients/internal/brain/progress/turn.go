package progress

import "sync"

/*
 * Whether a question is being worked on, kept apart from what is happening now.
 *
 * The listening loop and the agent write to one current step and overlap: the
 * microphone reopens the moment an answer starts being written, so while the
 * brain works on a question the current step alternates between "answering"
 * and "listening" several times a second. Anything reading the current step to
 * decide whether the brain is busy therefore flickers, and a person watching
 * it cannot tell a long task from an idle room — which is exactly the question
 * they are asking when a turn takes a minute.
 *
 * A turn is a fact about the conversation, not about the microphone. Counted
 * rather than flagged, because a spoken turn can start while a typed one is
 * still finishing and a single boolean would have the first to end declare
 * both over.
 */
var turns struct {
	mu      sync.Mutex
	running int

	// started is closed when the next turn starts, and over when the count
	// next comes back to nothing. Both are made only when somebody asks, so a
	// program with nothing waiting on them keeps no channels at all.
	started chan struct{}
	over    chan struct{}
}

// StartedATurn records that a question is being answered.
func StartedATurn() {
	turns.mu.Lock()
	turns.running++

	if turns.started != nil {
		close(turns.started)
		turns.started = nil
	}

	turns.mu.Unlock()
}

// FinishedATurn records that one has finished. Safe to call more times than
// StartedATurn: the count never goes below zero, so a stray call cannot leave
// the interface insisting work is still going on.
func FinishedATurn() {
	turns.mu.Lock()

	if turns.running > 0 {
		turns.running--
	}

	if turns.running == 0 && turns.over != nil {
		close(turns.over)
		turns.over = nil
	}

	turns.mu.Unlock()
}

// InATurn reports whether the brain is working on something it was asked.
func InATurn() bool {
	turns.mu.Lock()
	defer turns.mu.Unlock()

	return turns.running > 0
}

/*
 * NextTurn is closed the moment a question next starts being answered.
 *
 * For work nobody is waiting on, so it can stop what it is doing rather than
 * find out at its next step. A model call on a processor runs for minutes, and
 * "between steps" was the fourteen minutes somebody waited for an answer while
 * the brain went on reading its own guides.
 *
 * Ask for it before looking at InATurn, not after: a turn that starts between
 * the two is then still seen, by one or the other.
 */
func NextTurn() <-chan struct{} {
	turns.mu.Lock()
	defer turns.mu.Unlock()

	if turns.started == nil {
		turns.started = make(chan struct{})
	}

	return turns.started
}

// NoTurn is closed when no question is being answered: at once when none is,
// otherwise the moment the last one finishes.
func NoTurn() <-chan struct{} {
	turns.mu.Lock()
	defer turns.mu.Unlock()

	if turns.running == 0 {
		done := make(chan struct{})
		close(done)

		return done
	}

	if turns.over == nil {
		turns.over = make(chan struct{})
	}

	return turns.over
}
