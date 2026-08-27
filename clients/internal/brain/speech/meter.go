package speech

import (
	"sync"
	"time"
)

// The live audio level.
//
// This exists so the interface can respond to sound that is really there. The
// obvious alternative — animating the display from a timer whenever the brain
// is in the "speaking" state — looks almost the same and is worth nothing: it
// would move identically whether the voice was working, silent, or saying
// something completely different. A meter that reads the actual samples tells
// you the sound is real, and stops when it stops.
//
// Two sources publish here. The microphone level comes from the voice detector,
// which is already measuring it to decide when a turn has ended, so this costs
// nothing extra. The voice level comes from the synthesiser's own output.

// LevelReference is the RMS treated as a full-scale reading.
//
// It is an RMS, not a peak, so it is not comparable to SpeechPeak in level.go:
// the same speech measures far lower here than its peaks do there.
//
// Samples are 16-bit, so the arithmetic maximum is 32767, but nothing speaks
// anywhere near that. Measured on this machine: room tone sits around RMS 679,
// ordinary speech peaks in the low thousands. 6000 puts normal talking in the
// upper half of the range without clipping the loud moments off.
const LevelReference = 6000.0

// LevelFreshness is how long a reading is believed.
//
// Both publishers write continuously while they are running and simply stop
// when they finish, so silence is expressed by absence. Without an expiry the
// display would freeze holding the last level of the last word forever.
const LevelFreshness = 300 * time.Millisecond

// Reading is the current audio level and what is producing it.
type Reading struct {
	// Source is "mic" while the microphone is open, "voice" while the brain
	// is speaking, and empty when nothing is moving.
	Source string `json:"source"`
	// Level is 0 to 1, relative to LevelReference.
	Level float64 `json:"level"`
	// Floor is the level this room is never quieter than, on the same scale.
	//
	// Sent because a display that does not know it will show the fan, the
	// drive and the traffic outside as though they were somebody talking. It
	// is measured rather than assumed: the voice detector works out what this
	// room's silence sounds like at the start of every turn, because it has to
	// in order to know when a sentence has ended. Zero when nothing has been
	// measured yet, and for the brain's own voice, which has no room in it.
	Floor float64 `json:"floor"`
}

// A reading from one source.
type slot struct {
	level   float64
	floor   float64
	written time.Time
}

func (s slot) fresh() bool {
	return !s.written.IsZero() && time.Since(s.written) <= LevelFreshness
}

// The two sources are kept apart rather than sharing one slot.
//
// They were sharing one, and it was wrong in a way that mattered. Both publish
// continuously, so whichever wrote last won: while the brain was speaking with
// the microphone also open, the reading flipped between "voice" and "mic" many
// times a second. The display flickered between two colours, and — worse —
// anything asking "is the brain talking right now?" got the wrong answer about
// half the time it asked. That question is the one the microphone uses to avoid
// transcribing the brain's own voice, so it has to be answerable.
var meter struct {
	mu    sync.RWMutex
	mic   slot
	voice slot
}

// publishLevel records a measured level. rms is in raw sample units.
func publishLevel(source string, rms float64) {
	publishLevelWithFloor(source, rms, 0)
}

// publishLevelWithFloor also records what silence measures in this room.
func publishLevelWithFloor(source string, rms, floor float64) {
	entry := slot{
		level:   clampLevel(rms / LevelReference),
		floor:   clampLevel(floor / LevelReference),
		written: time.Now(),
	}

	meter.mu.Lock()
	defer meter.mu.Unlock()

	switch source {
	case "voice":
		meter.voice = entry
	case "mic":
		meter.mic = entry
	}
}

func clampLevel(v float64) float64 {
	if v > 1 {
		return 1
	}

	if v < 0 {
		return 0
	}

	return v
}

// clearLevel marks a source as finished, so the display falls silent at once
// rather than waiting out LevelFreshness.
func clearLevel(source string) {
	meter.mu.Lock()
	defer meter.mu.Unlock()

	switch source {
	case "voice":
		meter.voice = slot{}
	case "mic":
		meter.mic = slot{}
	}
}

// Speaking reports whether the brain's own voice is coming out of the speakers
// at this moment.
//
// Measured, not estimated. The alternative — working out how long a sentence
// ought to take from its length — was tried in this project and was wrong in
// both directions: too short and the microphone transcribed the brain, too long
// and every exchange dragged.
func Speaking() bool {
	meter.mu.RLock()
	defer meter.mu.RUnlock()

	return meter.voice.fresh()
}

// LiveLevel reports what is currently being heard or said.
//
// Named apart from the Level type in level.go, which measures a finished
// recording rather than sound in flight.
func LiveLevel() Reading {
	meter.mu.RLock()
	defer meter.mu.RUnlock()

	// The voice wins when both are live. While the brain is talking, that is
	// the thing worth showing, and it is also the answer that stops the
	// display flickering between the two.
	if meter.voice.fresh() {
		return Reading{Source: "voice", Level: meter.voice.level, Floor: meter.voice.floor}
	}

	if meter.mic.fresh() {
		return Reading{Source: "mic", Level: meter.mic.level, Floor: meter.mic.floor}
	}

	return Reading{}
}
