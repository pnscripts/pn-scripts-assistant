package speech

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func quietWav(t *testing.T, path string, peak int, samples int) {
	t.Helper()

	raw := make([]byte, 44+samples*2)
	copy(raw, "RIFF....WAVEfmt ")

	// A slow sine, so the peak is real and the rest of it is not.
	for i := 0; i < samples; i++ {
		v := int16(float64(peak) * sineish(i))
		binary.LittleEndian.PutUint16(raw[44+i*2:], uint16(v))
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func sineish(i int) float64 {
	// A triangle, which needs no imports and has a real peak.
	period := 100
	x := float64(i%period)/float64(period)*4 - 2

	if x > 1 {
		x = 2 - x
	}

	if x < -1 {
		x = -2 - x
	}

	return x
}

func peakOf(t *testing.T, path string) int {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	peak := 0

	for i := 44; i+1 < len(raw); i += 2 {
		v := int(int16(binary.LittleEndian.Uint16(raw[i:])))
		if v < 0 {
			v = -v
		}

		if v > peak {
			peak = v
		}
	}

	return peak
}

/*
 * A recording loud enough to detect and too quiet to understand.
 *
 * This is the gap that produced "loud enough, but no words came back" all
 * afternoon: peaks of 2012 and 6392 were understood, peaks of 215 and 282 came
 * back empty, from the same voice in the same room a little further away.
 */
func TestAQuietRecordingIsTurnedUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiet.wav")

	quietWav(t, path, 280, 4000)

	gain, err := Normalise(path)
	if err != nil {
		t.Fatal(err)
	}

	if gain <= 1 {
		t.Fatalf("a recording peaking at 280 was left alone (gain %.2f)", gain)
	}

	after := peakOf(t, path)

	if after < TargetPeak/2 {
		t.Errorf("turned up to %d, which is still too quiet to make words from", after)
	}

	if after > 32767 {
		t.Errorf("turned up to %d, past what a sample can hold", after)
	}
}

// Something already loud is left exactly as it was.
func TestALoudRecordingIsNotTouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loud.wav")

	quietWav(t, path, 24000, 4000)

	before := peakOf(t, path)

	gain, err := Normalise(path)
	if err != nil {
		t.Fatal(err)
	}

	if gain != 1 {
		t.Errorf("a loud recording was scaled by %.2f", gain)
	}

	if after := peakOf(t, path); after != before {
		t.Errorf("the samples changed: %d became %d", before, after)
	}
}

/*
 * Silence stays silence.
 *
 * Amplifying an empty room by twelve makes a loud recording of an empty room,
 * and the recogniser obligingly invents something to hear in it. That is worse
 * than saying nothing was heard, because it cannot be told from a real answer.
 */
func TestSilenceIsNotAmplifiedIntoWords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "silent.wav")

	quietWav(t, path, 12, 4000)

	gain, err := Normalise(path)
	if err != nil {
		t.Fatal(err)
	}

	if gain != 1 {
		t.Errorf("near-silence was amplified by %.2f", gain)
	}
}

// The gain is bounded, so nothing is stretched past what it can carry.
func TestTheGainIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "faint.wav")

	quietWav(t, path, 60, 4000)

	gain, err := Normalise(path)
	if err != nil {
		t.Fatal(err)
	}

	if gain > MostGain+0.01 {
		t.Errorf("scaled by %.2f, past the limit of %.2f", gain, MostGain)
	}

	if p := peakOf(t, path); p > 32767 {
		t.Errorf("a sample reached %d", p)
	}
}
