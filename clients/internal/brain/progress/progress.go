// Package progress reports what the brain is doing while it does it.
//
// A reply on this machine can take minutes: the model runs on the processor,
// and a turn that uses a tool is several model calls with work in between. For
// most of that the interface had nothing to say, so the honest question from
// somebody watching it — is this working, or is it stuck? — had no answer.
//
// This is deliberately a single current step rather than a log. What somebody
// waiting wants is what is happening now and how long it has been going, and a
// scrolling list of everything that has happened is a worse answer to that than
// one line.
package progress

import (
	"sync"
	"time"
)

// Step is what the brain is doing at this moment.
type Step struct {
	// Busy is false when there is nothing in flight.
	Busy bool `json:"busy"`
	// What kind of work: "thinking", "tool", "speaking", "listening".
	Kind string `json:"kind"`
	// A short line for a person: "Reading /etc/hosts".
	Note string `json:"note"`

	/*
	 * Tool is which tool is running, when one is.
	 *
	 * Carried separately from the note because they are read by different
	 * audiences. The note is written for a person and names the thing being
	 * acted on — "Read /etc/hosts" — while anything deciding what to *do*
	 * about the step needs to know it is read_file, and cannot recover that
	 * from prose. The interface guessed by taking the note's first word, which
	 * gave "read", matched nothing, and meant the spoken line announcing a slow
	 * tool never once played.
	 */
	Tool string `json:"tool"`

	/*
	 * SoFar is the answer as it is being written.
	 *
	 * The reply used to reach the page in one piece at the end, so a turn
	 * taking a minute showed nothing for a minute — and on a spoken turn the
	 * assistant talked the whole time while the transcript beside it sat
	 * empty. From the outside that is indistinguishable from a program that
	 * has stopped, which is the doubt every other part of this panel exists to
	 * remove.
	 */
	SoFar string `json:"so_far"`
	// How long this turn has been going, in seconds.
	Seconds float64 `json:"seconds"`
	// How many model calls this turn has taken, which is what makes a slow
	// turn slow and is worth being able to see.
	Round int `json:"round"`

	/*
	 * Background is anything nobody is waiting on.
	 *
	 * Two things qualify, and the core is coloured by this. The first is work
	 * the brain gave itself: learning from the last conversation runs a minute
	 * after every exchange, and without the distinction the brain sits there
	 * amber and apparently thinking about a question nobody asked, so its
	 * owner opens the program and finds it already busy with them.
	 *
	 * The second is listening, which is what it does whenever it is doing
	 * nothing else. Reporting that as work would pin the core amber and run a
	 * clock for the entire time the program is open — the same false "it is
	 * thinking" the first case was written to prevent, arrived at from the
	 * other direction.
	 *
	 * Still reported, and still shown — just not as though somebody is waiting
	 * for it.
	 */
	Background bool `json:"background"`

	/*
	 * Model is which model this turn is being answered by.
	 *
	 * Reported because it changes during a session and the change is the whole
	 * point: small talk goes to a quick model and work goes to the one that
	 * can use tools, and somebody watching an answer take its time deserves to
	 * know which of them they are waiting for.
	 */
	Model string `json:"model"`
}

var current struct {
	mu         sync.RWMutex
	busy       bool
	kind       string
	note       string
	tool       string
	soFar      string
	round      int
	background bool
	model      string
	started    time.Time
}

// Begin marks the start of a turn.
// Begin also clears the draft: a new turn is a new answer.
//
// Cleared here and in Done rather than on every step, because the listening
// loop and the turn being answered write to this at the same time — a spoken
// conversation is transcribing the next thing while the last one is still
// being written — and clearing on each step meant the loop wiped the answer
// mid-sentence, every time, so the page never saw one.
func Begin() {
	current.mu.Lock()
	current.soFar = ""
	current.mu.Unlock()

	current.mu.Lock()
	current.busy = true
	current.kind = "thinking"
	current.note = "Thinking"
	current.round = 0
	current.background = false
	current.model = ""
	current.started = time.Now()
	current.mu.Unlock()
}

// UsingModel records which model is answering this turn.
func UsingModel(name string) {
	current.mu.Lock()
	current.model = name
	current.mu.Unlock()
}

// Set records what is happening now.
func Set(kind, note string) {
	current.mu.Lock()

	// Beginning is not required: a turn that starts with speech or listening
	// should still show a clock rather than a stopwatch that has not started.
	if !current.busy {
		current.busy = true
		current.started = time.Now()
	}

	current.kind = kind
	current.note = note
	current.tool = ""
	current.background = false

	model := current.model
	current.mu.Unlock()

	remember(kind, note, "", model, false)
}

// SetTool reports a named tool starting, so that what is listening for it can
// match on the name rather than guessing from the summary.
func SetTool(name, note string) {
	current.mu.Lock()

	if !current.busy {
		current.busy = true
		current.started = time.Now()
	}

	current.kind = "tool"
	current.note = note
	current.tool = name
	current.background = false
	model := current.model
	current.mu.Unlock()

	remember("tool", note, name, model, false)
}

/*
 * SetBackground records work the brain gave itself.
 *
 * The same as Set except that nothing is waiting on it, which is what stops the
 * core from turning the colour it uses for "I am working on what you asked".
 * Used by the learning worker, which runs after every conversation and would
 * otherwise leave the brain looking permanently busy.
 */
func SetBackground(kind, note string) {
	current.mu.Lock()

	if !current.busy {
		current.busy = true
		current.started = time.Now()
	}

	current.kind = kind
	current.note = note
	current.tool = ""
	current.background = true
	model := current.model
	current.mu.Unlock()

	remember(kind, note, "", model, true)
}

// Round counts a model call.
func Round(n int) {
	current.mu.Lock()
	current.round = n
	current.mu.Unlock()
}

// Done marks the end of a turn.
func Done() {
	current.mu.Lock()
	current.busy = false
	current.kind = ""
	current.note = ""
	current.tool = ""
	current.soFar = ""
	current.mu.Unlock()

	finish()
}

// Answering reports whether a reply is being worked on.
//
// Distinct from Busy, which is true for background work as well. Anything that
// is not what somebody is waiting for should give way to something that is, and
// it needs to be able to tell the difference.
func Answering() bool {
	current.mu.RLock()
	defer current.mu.RUnlock()

	return current.busy && (current.kind == "thinking" || current.kind == "tool" || current.kind == "waiting")
}

// Now reports the current step.
func Now() Step {
	current.mu.RLock()
	defer current.mu.RUnlock()

	if !current.busy {
		return Step{}
	}

	return Step{
		Busy:       true,
		Kind:       current.kind,
		Note:       current.note,
		Tool:       current.tool,
		SoFar:      current.soFar,
		Seconds:    time.Since(current.started).Seconds(),
		Round:      current.round,
		Background: current.background,
		Model:      current.model,
	}
}

/*
 * Writing records the answer as it is produced.
 *
 * Called with everything written so far rather than each new piece, so a
 * dropped update costs nothing: the next one carries the whole thing. The page
 * polls, and a poll that misses a fragment would otherwise leave a hole in the
 * middle of a sentence.
 */
func Writing(text string) {
	current.mu.Lock()
	current.soFar = text
	current.mu.Unlock()
}
