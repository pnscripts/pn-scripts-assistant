/*
 * Package protocol is what this program says to itself across a network.
 *
 * One shape for everything that is reported — a task starting, a tool's
 * output, an approval being asked for — so that a phone, a window and an
 * execution node all read the same messages, and so that adding a new kind of
 * message is adding a constant rather than inventing a format.
 *
 * Structured, not prose. "It is working on step two" is a sentence somebody
 * has to parse; {"type":"TASK_STARTED","task":12} is a fact. The sentence is
 * the interface's business, and it can only write a good one if what it was
 * given was exact.
 *
 * Versioned from the first line, because the client, the server and an
 * execution node are updated on different days. A message from a newer
 * program should be refused with a sentence saying so, not half-understood.
 */
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

/*
 * Version is the shape of these messages, not the program's own version.
 *
 * It changes when a field changes meaning or goes away — never for an added
 * field, which older readers ignore and newer ones can do without. A reader
 * accepts anything up to its own version, which is what lets a phone that has
 * not been updated for a month still show what is happening.
 */
const Version = 1

// Kind is what happened. The set is closed: a kind nobody knows is a message
// nobody can act on, and silently ignoring it is how a client comes to show a
// task that never finishes.
type Kind string

const (
	// The life of a piece of work.
	TaskStarted   Kind = "TASK_STARTED"
	TaskCompleted Kind = "TASK_COMPLETED"
	TaskFailed    Kind = "TASK_FAILED"
	TaskCancelled Kind = "TASK_CANCELLED"

	// What was decided for it, each said once, as it is decided.
	AgentSelected  Kind = "AGENT_SELECTED"
	ModelSelected  Kind = "MODEL_SELECTED"
	DeviceSelected Kind = "DEVICE_SELECTED"

	// A step, and the tool it used.
	StepStarted   Kind = "STEP_STARTED"
	StepFinished  Kind = "STEP_FINISHED"
	ToolStarted   Kind = "TOOL_STARTED"
	ToolOutput    Kind = "TOOL_OUTPUT"
	ToolCompleted Kind = "TOOL_COMPLETED"

	// Waiting for its owner, which is the one kind a client must never miss.
	ApprovalRequired Kind = "APPROVAL_REQUIRED"
	ApprovalReceived Kind = "APPROVAL_RECEIVED"

	// Devices coming and going, and work waiting for one of them.
	DeviceConnected    Kind = "DEVICE_CONNECTED"
	DeviceDisconnected Kind = "DEVICE_DISCONNECTED"
	WaitingForDevice   Kind = "WAITING_FOR_DEVICE"

	// Progress that is worth saying and belongs to no step.
	ProgressUpdate Kind = "PROGRESS_UPDATE"
)

// known is every kind this version understands.
var known = map[Kind]bool{
	TaskStarted: true, TaskCompleted: true, TaskFailed: true, TaskCancelled: true,
	AgentSelected: true, ModelSelected: true, DeviceSelected: true,
	StepStarted: true, StepFinished: true,
	ToolStarted: true, ToolOutput: true, ToolCompleted: true,
	ApprovalRequired: true, ApprovalReceived: true,
	DeviceConnected: true, DeviceDisconnected: true, WaitingForDevice: true,
	ProgressUpdate: true,
}

// Known reports whether a kind is one this version can act on.
func (k Kind) Known() bool { return known[k] }

/*
 * Envelope is one thing that happened.
 *
 * Seq is the order it happened in, and it is the whole of how a client that
 * was away catches up: it says the last number it saw and is given everything
 * after it. Nothing else — not a timestamp, not a task's state — is reliable
 * for that, because two events can share a second and a state says where
 * something got to rather than how it got there.
 */
type Envelope struct {
	V   int       `json:"v"`
	Seq int64     `json:"seq,omitempty"`
	ID  string    `json:"id"`
	At  time.Time `json:"at"`

	Type Kind `json:"type"`

	// What it is about. A task and a step where there is one; a device when
	// the event is about a machine rather than about work.
	Task   int64  `json:"task,omitempty"`
	Step   int64  `json:"step,omitempty"`
	Device string `json:"device,omitempty"`

	// Said is the one line a person would read. Kept beside the data rather
	// than built from it, because the program that knows why something
	// happened is in a better position to say it than the one drawing it.
	Said string `json:"said,omitempty"`

	// Data is whatever else that kind carries. Raw, so a reader that does not
	// know the kind can still store and forward it.
	Data json.RawMessage `json:"data,omitempty"`
}

// New is an envelope of a kind, stamped and given an id.
func New(kind Kind, said string) Envelope {
	return Envelope{V: Version, ID: NewID("evt"), At: time.Now().UTC(), Type: kind, Said: said}
}

// About puts an envelope on a task and, where there is one, a step.
func (e Envelope) About(task, step int64) Envelope {
	e.Task, e.Step = task, step

	return e
}

// On names the device it happened on.
func (e Envelope) On(device string) Envelope {
	e.Device = device

	return e
}

// With attaches whatever else the kind carries. A value that cannot be
// written as JSON is left out rather than failing the event: the event
// happened whether or not its detail can be encoded.
func (e Envelope) With(data any) Envelope {
	if raw, err := json.Marshal(data); err == nil {
		e.Data = raw
	}

	return e
}

/*
 * Check is whether this can be acted on at all.
 *
 * Two reasons to refuse, and they are different: a message from a newer
 * protocol may mean something this program would get wrong, and a kind that
 * is not known is one it has no code for. Both are worth saying out loud —
 * "your phone is older than this brain" is a fixable sentence, where a
 * message quietly dropped is a bug report about something else entirely.
 */
func (e Envelope) Check() error {
	switch {
	case e.V <= 0:
		return fmt.Errorf("the message says no protocol version")
	case e.V > Version:
		return fmt.Errorf("the message speaks protocol %d and this understands %d: update this program", e.V, Version)
	case e.Type == "":
		return fmt.Errorf("the message says nothing about what happened")
	case !e.Type.Known():
		return fmt.Errorf("%q is not something this version knows about", e.Type)
	}

	return nil
}

/*
 * NewID is a short identifier with a word in front of it.
 *
 * The word is there for the person reading a log: evt_… and cmd_… say what
 * they are without anybody having to look them up, and an id pasted into a
 * question carries its own context.
 */
func NewID(kind string) string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		// Only when the system has no randomness at all, where a duplicate id
		// is the smallest of the problems. The time still orders it.
		return fmt.Sprintf("%s_%d", kind, time.Now().UnixNano())
	}

	return kind + "_" + hex.EncodeToString(raw)
}
