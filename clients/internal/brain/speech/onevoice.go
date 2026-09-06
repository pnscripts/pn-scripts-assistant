package speech

import "context"

/*
 * One thing said at a time.
 *
 * Nothing enforced this. An answer is spoken sentence by sentence from a queue,
 * which is orderly on its own — but the queue is not the only thing that
 * speaks. A greeting on opening, a reminder falling due, a line said straight
 * from the interface, and the answer to the previous turn still finishing all
 * arrive by different paths, and any two of them landing together produced two
 * voices over each other saying different sentences.
 *
 * Which is not merely untidy. The microphone hears the room, so two voices at
 * once is what the recogniser is handed while somebody is trying to talk over
 * them; and the canceller subtracts one copy of what was played, not two
 * overlapping ones.
 *
 * So speaking is serialised. It is a lock rather than a queue on purpose: a
 * queue would hold a reminder for however long an answer takes and then say it
 * into a silence nobody is waiting in. Waiting a sentence is right; waiting an
 * answer is not, and the caller can give up by cancelling.
 */
var oneVoice = make(chan struct{}, 1)

/*
 * waitToSpeak takes the turn to speak, or gives up if the caller does.
 *
 * Returns whether the turn was taken. A caller that did not take it must not
 * release it — which is why this returns a bool rather than a function that
 * might be nil.
 */
func waitToSpeak(ctx context.Context) bool {
	select {
	case oneVoice <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// doneSpeaking hands the turn on.
func doneSpeaking() {
	select {
	case <-oneVoice:
	default:
		// Released twice, which would mean a bug rather than a race. Doing
		// nothing is better than blocking the next thing that wants to speak.
	}
}

/*
 * SpeakingNow reports whether something is being said.
 *
 * Distinct from the interruption bookkeeping, which records what to cancel.
 * This answers "would starting now overlap", which is a different question and
 * the one the queue cares about.
 */
func SpeakingNow() bool { return len(oneVoice) > 0 }
