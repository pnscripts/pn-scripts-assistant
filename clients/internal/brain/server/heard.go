package server

import (
	"fmt"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/wake"
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

	/*
	 * Whose voice it was, when the brain has been taught one.
	 *
	 * KnownVoice is false when there was nothing to compare against or too
	 * little sound to judge — which must not be collapsed into "not you", or a
	 * brain whose model is missing stops answering its owner.
	 */
	KnownVoice bool    `json:"known_voice"`
	Owner      bool    `json:"owner"`
	Voice      float64 `json:"voice"`

	/*
	 * What this machine itself was playing at the time.
	 *
	 * A different fact from the two above and it settles a different question.
	 * A voiceprint says "that was not you", which leaves open whether it was a
	 * person at all; this says a browser was playing a video while it was
	 * heard, which usually finishes the sentence.
	 */
	MachinePlaying bool     `json:"machine_playing"`
	Playing        []string `json:"playing,omitempty"`

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
		/*
		 * "No name in it" is true and, when the recogniser mangled the name,
		 * useless. Said plainly, the difference is between "you did not
		 * address me" and "you did, and I did not recognise my own name" —
		 * two problems, two fixes, and one of them is not the microphone.
		 */
		if near := wake.NearMiss(h.Text, wakeWord); near != "" {
			return fmt.Sprintf("no name in it — %q was close but not close enough", near)
		}

		return "no name in it"
	}
}

/*
 * And the same record, in the shape the tool that answers for it wants.
 *
 * Converted here rather than shared as a type, so the log of what a microphone
 * heard is not a type the tools package can reach into — it is a record of
 * somebody's room.
 */
func (s *Server) recentlyHeard() []tools.Overheard {
	heard := s.heard.recent()

	out := make([]tools.Overheard, 0, len(heard))

	// recent() is already newest first, which is the order the account wants:
	// the thing being asked about is nearly always the last thing said.
	for _, h := range heard {
		out = append(out, tools.Overheard{
			Text:           h.Text,
			Addressed:      h.Addressed,
			Why:            h.Why,
			KnownVoice:     h.KnownVoice,
			Owner:          h.Owner,
			Voice:          h.Voice,
			MachinePlaying: h.MachinePlaying,
			Playing:        h.Playing,
			PeakRMS:        h.PeakRMS,
			NoiseFloor:     h.NoiseFloor,
			SpokeForMS:     h.SpokeForMS,
			Ago:            brain.Ago(h.At),
		})
	}

	return out
}
