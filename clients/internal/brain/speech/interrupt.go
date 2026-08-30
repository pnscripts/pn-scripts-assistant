package speech

import (
	"context"
	"sync"
)

/*
 * Being interrupted.
 *
 * The thing that separates a conversation from a transaction. Every exchange
 * here used to be strictly in turn — listen, think, speak, and only then listen
 * again — so an answer that had started was going to be finished whatever its
 * owner did about it. On a machine where an answer can run to a minute of
 * speech, that means sitting through a wrong answer to its end before being
 * able to say so.
 *
 * The Realtime API calls this barge-in and does three things when the person
 * starts talking: stop playing, throw away what was not yet played, and abandon
 * the rest of the response. This does the same three.
 *
 * The hard half is telling their voice from its own. A microphone in front of
 * a speaker hears both, and there is no echo cancellation on this machine, so
 * the level it hears while speaking is measured and the person has to be
 * clearly above it. On headphones that measured level is just the room, so
 * anything said interrupts; on speakers it is the brain's own voice, and
 * talking over it means talking over it.
 */

var talking struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	interrupted bool
}

// speakingStarted registers the utterance that can be cut short.
func speakingStarted(cancel context.CancelFunc) {
	talking.mu.Lock()
	talking.cancel = cancel
	talking.mu.Unlock()
}

// speakingStopped forgets it, so a later interruption cancels nothing.
func speakingStopped() {
	talking.mu.Lock()
	talking.cancel = nil
	talking.mu.Unlock()
}

/*
 * Interrupt stops the voice at once.
 *
 * Cancelling the context kills the synthesiser and the player together, which
 * is why each utterance is given one: the alternative is letting the sentence
 * finish, and a barge-in that takes effect at the end of the sentence is not a
 * barge-in.
 */
func Interrupt() {
	talking.mu.Lock()
	defer talking.mu.Unlock()

	talking.interrupted = true

	if talking.cancel != nil {
		talking.cancel()
		talking.cancel = nil
	}
}

/*
 * Interrupted reports that the person cut in, until the next turn clears it.
 *
 * Read by whatever is producing the answer, so that the rest of it is abandoned
 * rather than queued behind the part that was cut off. Stopping the sound and
 * then saying the remaining four sentences anyway would be worse than not
 * stopping at all.
 */
func Interrupted() bool {
	talking.mu.Lock()
	defer talking.mu.Unlock()

	return talking.interrupted
}

// ClearInterrupt starts a new turn listening again.
func ClearInterrupt() {
	talking.mu.Lock()
	talking.interrupted = false
	talking.mu.Unlock()
}
