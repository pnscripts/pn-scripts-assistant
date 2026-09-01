package progress

import (
	"sync"
	"time"
)

/*
 * The last few things the brain did, kept so they can be shown as they happen.
 *
 * The step on its own answers "what is it doing now", which is the right
 * question while somebody waits and the wrong one afterwards: a turn on this
 * machine is listening, then making out words, then thinking, then a tool,
 * then speaking, and each of those replaces the last without leaving a trace.
 * Somebody who looked away for ten seconds has no way to find out what
 * happened in them, and a program that spent four minutes in a tool looks
 * identical to one that spent four minutes stuck.
 *
 * A short history rather than a log. This is for watching work go past, not
 * for auditing it — what happened in the last minute or two, at the size that
 * fits on screen without scrolling.
 */

// HowManyRecent is how much history to keep.
//
// Enough for a whole turn including its tools, and small enough that the
// oldest entry is still something that just happened.
const HowManyRecent = 40

// Entry is one thing the brain did, and how long it took.
type Entry struct {
	// id identifies this entry so late detail can find it. Not sent to the
	// page, which has no use for it.
	id int64

	Kind string `json:"kind"`
	Note string `json:"note"`
	Tool string `json:"tool"`

	// When it started, so the interface can say how long ago.
	At time.Time `json:"at"`

	// How long it lasted, in seconds. Zero while it is still running.
	Took float64 `json:"took"`

	// Background is work nobody is waiting on, shown more quietly.
	Background bool `json:"background"`

	// Model is which model was answering, when one was.
	Model string `json:"model"`

	/*
	 * Detail is what actually happened, in specifics.
	 *
	 * The kind and the note say a tool ran and roughly which; this says what
	 * it did — the query that was searched for, the words that were heard, how
	 * many results came back, how big the file was. Without it the panel is a
	 * list of state names, and a list of state names tells somebody watching
	 * almost nothing they did not already know: "Thinking" for four minutes is
	 * the same picture whether it is working or wedged.
	 *
	 * Several lines because a step has more than one thing worth saying: a
	 * search is a query and then a count of what came back, and both are
	 * interesting at different moments.
	 */
	Detail []string `json:"detail"`
}

var history = struct {
	mu      sync.RWMutex
	entries []Entry

	// nextID numbers entries so a detail arriving late can still find the step
	// it belongs to. See DetailOn.
	nextID int64
}{}

/*
 * remember records a step, closing off the one before it.
 *
 * Called from the setters rather than by polling, so a step shorter than the
 * interface's refresh is still recorded. A tool that returns in eighty
 * milliseconds is exactly the kind of thing a poll misses and exactly the kind
 * of thing worth seeing, because it is the difference between a tool that ran
 * and a tool that was never called.
 */
func remember(kind, note, tool, model string, background bool) {
	history.mu.Lock()
	defer history.mu.Unlock()

	now := time.Now()

	// Close the previous entry, so its duration is known.
	if n := len(history.entries); n > 0 && history.entries[n-1].Took == 0 {
		history.entries[n-1].Took = now.Sub(history.entries[n-1].At).Seconds()
	}

	/*
	 * The same step repeated is not a new entry.
	 *
	 * Speaking a long answer sets "speaking" once per sentence, and a feed
	 * that showed each of them would be a wall of one word. What somebody
	 * watching wants is that it is still speaking, which the running duration
	 * already says.
	 */
	if n := len(history.entries); n > 0 {
		last := history.entries[n-1]
		if last.Kind == kind && last.Note == note && last.Tool == tool {
			history.entries[n-1].Took = 0

			return
		}
	}

	history.nextID++

	history.entries = append(history.entries, Entry{
		id:         history.nextID,
		Kind:       kind,
		Note:       note,
		Tool:       tool,
		At:         now,
		Background: background,
		Model:      model,
	})

	if len(history.entries) > HowManyRecent {
		history.entries = history.entries[len(history.entries)-HowManyRecent:]
	}
}

// finish closes the last entry when the brain goes idle.
func finish() {
	history.mu.Lock()
	defer history.mu.Unlock()

	if n := len(history.entries); n > 0 && history.entries[n-1].Took == 0 {
		history.entries[n-1].Took = time.Since(history.entries[n-1].At).Seconds()
	}
}

// Recent is the last few things the brain did, oldest first.
func Recent() []Entry {
	history.mu.RLock()
	defer history.mu.RUnlock()

	out := make([]Entry, len(history.entries))
	copy(out, history.entries)

	// The one still running has no duration yet; report how long so far, so a
	// step that has been going four minutes says so rather than saying nothing.
	if n := len(out); n > 0 && out[n-1].Took == 0 {
		out[n-1].Took = time.Since(out[n-1].At).Seconds()
	}

	return out
}

/*
 * Detail adds a line of specifics to whatever is running now.
 *
 * Attached to the current step rather than recorded on its own, so that the
 * query belongs visibly to the search and the transcript to the listening. A
 * flat stream of unattached lines would be a log, and a log is what somebody
 * reads afterwards; this is for watching.
 *
 * Silently ignored when nothing is running. Detail arriving with no step to
 * attach it to means the work finished first, and inventing an entry for it
 * would put a line in the panel that never had a duration or a kind.
 */
func Detail(line string) {
	if line == "" {
		return
	}

	history.mu.Lock()
	defer history.mu.Unlock()

	n := len(history.entries)
	if n == 0 {
		return
	}

	// A handful at most. This is a panel, not a transcript, and a step that
	// reports forty lines pushes everything else off the screen.
	const mostDetail = 4

	if len(history.entries[n-1].Detail) >= mostDetail {
		return
	}

	// The same line twice says nothing the first did not.
	for _, had := range history.entries[n-1].Detail {
		if had == line {
			return
		}
	}

	history.entries[n-1].Detail = append(history.entries[n-1].Detail, line)
}

/*
 * Mark names the step running now, so detail can be attached to it later.
 *
 * A model call outlives the step that started it: it begins under "Thinking",
 * and by the time it returns the brain has moved on to answering, then to
 * speaking, then back to listening. Detail attached at that point lands on
 * whichever step happens to be last, which put a model's running time under
 * "Listening" — a step that had not been running and could not have taken it.
 */
func Mark() int64 {
	history.mu.RLock()
	defer history.mu.RUnlock()

	if n := len(history.entries); n > 0 {
		return history.entries[n-1].id
	}

	return 0
}

// DetailOn attaches a line to a particular step, however long ago it started.
//
// Ignored when that step has already fallen out of the history, which is the
// right outcome: the work it described is off the screen and a line about it
// would attach to something else.
/*
 * QuietenOn demotes a step once it turns out nobody was waiting on it.
 *
 * A listening turn looks identical whether somebody spoke or a chair creaked:
 * the level crossed the threshold, so the microphone opened, recorded, and
 * handed the audio to the recogniser. Only afterwards, when the recogniser
 * returns nothing, is it known that nothing was said — and by then the step
 * has already been announced as though it were part of a conversation.
 *
 * The bar is deliberately low, because failing to hear somebody is far worse
 * than listening to a door. What should not happen is the room being told that
 * the assistant heard a voice each time it checked one and found none.
 *
 * Demoted rather than deleted: it still belongs in the record of what the
 * microphone made of the room, which is the answer to "I said its name and
 * nothing happened".
 */
/*
 * reword changes what the last entry says, when it is the same step still
 * running.
 *
 * Appending would be wrong: a job reporting its progress is one step, and a
 * row per report buries every other thing the brain did — two thousand three
 * hundred documents produced two thousand three hundred rows saying the same
 * sentence with a different number.
 *
 * Only the last entry, and only when the kind matches. Anything else has moved
 * on, and rewriting a finished step would be rewriting history rather than
 * reporting the present.
 */
func reword(kind, note string) {
	fresh := true

	history.mu.Lock()

	if n := len(history.entries); n > 0 && history.entries[n-1].Kind == kind {
		history.entries[n-1].Note = note
		fresh = false
	}

	history.mu.Unlock()

	// A different step, or the first one: that is a new entry, and remember
	// takes the lock itself.
	if fresh {
		remember(kind, note, "", "", false)
	}
}

func QuietenOn(mark int64) {
	if mark == 0 {
		return
	}

	history.mu.Lock()
	defer history.mu.Unlock()

	for i := range history.entries {
		if history.entries[i].id == mark {
			history.entries[i].Background = true

			return
		}
	}
}

func DetailOn(mark int64, line string) {
	if line == "" || mark == 0 {
		return
	}

	history.mu.Lock()
	defer history.mu.Unlock()

	for i := range history.entries {
		if history.entries[i].id != mark {
			continue
		}

		const mostDetail = 4

		if len(history.entries[i].Detail) >= mostDetail {
			return
		}

		for _, had := range history.entries[i].Detail {
			if had == line {
				return
			}
		}

		history.entries[i].Detail = append(history.entries[i].Detail, line)

		return
	}
}
