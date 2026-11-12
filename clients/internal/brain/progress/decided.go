package progress

import (
	"strings"
	"time"
)

/*
 * A question that has been decided stops waiting on screen.
 *
 * The loop stops for a decision with a "waiting" step, and the step stayed
 * the newest piece of work in the history until something else happened.
 * Decided after the turn had ended, by a press in another window, by a
 * spoken yes, or by the quiet hours answering a held question by itself,
 * it went on reading "Waiting for you" in red for as long as the program
 * had nothing else to do: a picture of a question nobody had left to
 * answer.
 *
 * The step is not removed. That a question was put, and how long it waited,
 * is part of what happened; it is said again as settled.
 */

// KindDecided is the kind a waiting step becomes once its question is
// decided.
const KindDecided = "decided"

// How a question was decided.
const (
	Approved = "approved"
	Refused  = "refused"
	ByItself = "itself"
)

// waitingFor is how the loop introduces a question, and decidedAs how the
// same line reads once it is decided. The page says each in its language
// (status.js STEP_KEYS).
const waitingFor = "Waiting for you: "

var decidedAs = map[string]string{
	Approved: "Approved: ",
	Refused:  "Refused: ",
	ByItself: "Started by itself: ",
}

/*
 * Waiting is Set("waiting", note) for a step that waits on one stored
 * question, so the decision can find it later. Two questions that read the
 * same are still two rows: Set would fold the second into the first, and
 * deciding one would then settle, or leave, the other.
 */
func Waiting(note string, invocation int64) {
	current.mu.Lock()

	if !current.busy {
		current.busy = true
		current.started = time.Now()
	}

	current.kind = "waiting"
	current.note = note
	current.tool = ""
	current.background = false

	model := current.model
	current.mu.Unlock()

	rememberFor("waiting", note, "", model, false, invocation)
}

/*
 * Decided settles the step that waits on this question, saying how it was
 * decided. It reports whether there was such a step still waiting: none
 * when the question was asked before the program started, its step has
 * fallen out of the history, or it was decided already.
 */
func Decided(invocation int64, how string) bool {
	said, ok := decidedAs[how]
	if invocation == 0 || !ok {
		return false
	}

	history.mu.Lock()
	defer history.mu.Unlock()

	found := false

	for i := range history.entries {
		e := &history.entries[i]

		if e.invocation != invocation || e.Kind != "waiting" {
			continue
		}

		if e.Took == 0 {
			e.Took = time.Since(e.At).Seconds()
		}

		e.Kind = KindDecided
		e.Decided = how
		e.Note = said + strings.TrimPrefix(e.Note, waitingFor)
		found = true
	}

	return found
}
