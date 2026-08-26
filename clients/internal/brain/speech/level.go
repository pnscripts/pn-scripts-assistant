package speech

import (
	"encoding/binary"
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

// Explain turns a level and a transcript into something worth showing.
func Explain(level Level, transcript string) string {
	switch {
	case transcript != "":
		return ""
	case level.Silent:
		return "That input is silent — no sound reached it at all. " +
			"Pick a different microphone from the list beside the button."
	default:
		return "Sound came through, but no words were made out. " +
			"Try speaking closer, or a little louder."
	}
}
