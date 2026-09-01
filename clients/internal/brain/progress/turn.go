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
}

// StartedATurn records that a question is being answered.
func StartedATurn() {
	turns.mu.Lock()
	turns.running++
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

	turns.mu.Unlock()
}

// InATurn reports whether the brain is working on something it was asked.
func InATurn() bool {
	turns.mu.Lock()
	defer turns.mu.Unlock()

	return turns.running > 0
}
