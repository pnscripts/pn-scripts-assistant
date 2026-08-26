package speech

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// Level describes how loud a recording was.
//
// Reported alongside a transcript because "heard nothing" has two very
// different causes: a microphone that captured silence, and a microphone that
// captured a room nobody spoke in. Told apart, the first sends somebody to
// their audio settings and the second tells them to speak up. Conflated, both
// look like broken software.
type Level struct {
	Peak   int     `json:"peak"`
	RMS    int     `json:"rms"`
	Silent bool    `json:"silent"`
	Ratio  float64 `json:"ratio"`
}

// SilenceThreshold is the peak below which a recording holds no sound at all.
//
// Well under speech, comfortably above the noise floor of an input with nothing
// connected — measured at peak 374 on this machine's empty analog jack.
const SilenceThreshold = 600

// MeasureWAV reads a 16-bit PCM WAV and reports how loud it was.
func MeasureWAV(path string) (Level, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Level{}, err
	}

	// 44 bytes is the standard header; anything shorter holds no samples.
	const header = 44

	if len(raw) <= header {
		return Level{Silent: true}, nil
	}

	samples := raw[header:]
	count := len(samples) / 2

	if count == 0 {
		return Level{Silent: true}, nil
	}

	var peak int
	var sum float64

	for i := 0; i < count; i++ {
		v := int16(binary.LittleEndian.Uint16(samples[i*2:]))

		magnitude := int(v)
		if magnitude < 0 {
			magnitude = -magnitude
		}

		if magnitude > peak {
			peak = magnitude
		}

		sum += float64(v) * float64(v)
	}

	rms := int(math.Sqrt(sum / float64(count)))

	return Level{
		Peak:   peak,
		RMS:    rms,
		Silent: peak < SilenceThreshold,
		Ratio:  float64(peak) / 32767,
	}, nil
}

// SpeechPeak is roughly where a spoken word registers.
//
// Below it, what was captured is almost certainly room tone: a fan, a hard
// drive, traffic. This is the distinction that matters when nothing was
// understood, because "the microphone heard the room" and "the microphone
// heard you and whisper failed" send somebody to completely different places.
//
// Not a threshold anything is rejected on — only how the result is explained.
const SpeechPeak = 8000

// Explain turns a level and a transcript into something worth showing.
//
// It quotes the actual number. Somebody debugging a microphone needs to know
// whether speaking louder changes anything, and a sentence that reads the same
// at every volume tells them nothing.
func Explain(level Level, transcript string) string {
	if transcript != "" {
		return ""
	}

	percent := int(level.Ratio * 100)

	switch {
	case level.Silent:
		return "That input is silent — nothing reached it at all. " +
			"Pick a different microphone in the Engine panel."
	case level.Peak < SpeechPeak:
		return fmt.Sprintf(
			"Only room noise came through (peak %d%%). Nothing sounded like speech, "+
				"so either the window closed before you spoke, or your voice is not "+
				"reaching that input. Speaking should push this well past 25%%.", percent)
	default:
		return fmt.Sprintf(
			"Speech-level sound came through (peak %d%%) but no words were made out. "+
				"Try speaking a little more clearly, or closer to the microphone.", percent)
	}
}
