package server

import (
	"sync"
	"time"
)

/*
 * The last few things the microphone made of the room.
 *
 * "I said its name and it did not answer" is one sentence covering four
 * different faults, and from outside they are the same silence: the room never
 * crossed the speech threshold; it crossed it and whisper returned nothing;
 * whisper returned words but not the name; or the name was there and the match
 * failed. Telling them apart needs the transcript and the numbers, and neither
 * existed anywhere — the log recorded no turns at all.
 *
 * Held in memory and nowhere else. This is a record of a room with people in
 * it, which is the last thing that should be written to disk on a machine
 * whose entire promise is that nothing about its owner leaves it. It is capped,
 * it is lost on restart, and that is the intended lifetime.
 */

// Kept small on purpose: enough to see what just happened, not enough to be a
// transcript of an evening.
const heardKept = 12

// Overheard is one turn, as the microphone and the transcriber made of it.
type Overheard struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`

	// Addressed is whether it was acted on, and Why says what decided that.
	Addressed bool   `json:"addressed"`
	Why       string `json:"why"`

	// What the detector measured.
	HeardSpeech bool `json:"heard_speech"`
	PeakRMS     int  `json:"peak_rms"`
	NoiseFloor  int  `json:"noise_floor"`
	Threshold   int  `json:"threshold"`
	SpokeForMS  int  `json:"spoke_for_ms"`
}

type heardLog struct {
	mu    sync.Mutex
	turns []Overheard
}

func (h *heardLog) add(turn Overheard) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.turns = append(h.turns, turn)

	if len(h.turns) > heardKept {
		h.turns = h.turns[len(h.turns)-heardKept:]
	}
}

// recent returns the turns newest first.
func (h *heardLog) recent() []Overheard {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Overheard, 0, len(h.turns))

	for i := len(h.turns) - 1; i >= 0; i-- {
		out = append(out, h.turns[i])
	}

	return out
}

// why says, in one line, what happened to a turn.
//
// Written here rather than in the page because the page would have to know the
// thresholds to say anything true about them, and then there would be two
// places that had to agree about what counts as being heard.
func why(h Overheard, addressed bool, wakeWord string) string {
	switch {
	case !h.HeardSpeech:
		return "nothing crossed the threshold"
	case h.Text == "":
		return "loud enough, but no words came back"
	case addressed:
		return "addressed"
	case wakeWord == "":
		return "addressed"
	default:
		return "no name in it"
	}
}
