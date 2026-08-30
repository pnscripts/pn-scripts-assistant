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
	// How long this turn has been going, in seconds.
	Seconds float64 `json:"seconds"`
	// How many model calls this turn has taken, which is what makes a slow
	// turn slow and is worth being able to see.
	Round int `json:"round"`

	/*
	 * Background is work the brain gave itself.
	 *
	 * It has to be told apart from work somebody is waiting on, because the
	 * core is coloured by this: learning from the last conversation runs a
	 * minute after every exchange, and without the distinction the brain sits
	 * there amber and apparently thinking about a question nobody asked. Its
	 * owner opens the program and finds it already busy with them.
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
	round      int
	background bool
	model      string
	started    time.Time
}

// Begin marks the start of a turn.
func Begin() {
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
	current.background = false
	current.mu.Unlock()
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
	current.background = true
	current.mu.Unlock()
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
	current.mu.Unlock()
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
		Seconds:    time.Since(current.started).Seconds(),
		Round:      current.round,
		Background: current.background,
		Model:      current.model,
	}
}
