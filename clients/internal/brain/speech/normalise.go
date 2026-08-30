package speech

import (
	"encoding/binary"
	"fmt"
	"os"
)

/*
 * Making a quiet recording loud enough to be understood.
 *
 * There are two different thresholds in this program and they are further apart
 * than they look. The detector needs to know that somebody is speaking, which
 * it can tell from a signal a few times above the room. The recogniser needs to
 * make words out of it, and it wants a great deal more than that.
 *
 * The gap between them is exactly what "loud enough, but no words came back"
 * means, and it was most of what the microphone did for an afternoon. Measured
 * here: turns peaking at 2012 and 6392 were understood, and turns peaking at
 * 215, 282, 734 and 901 came back empty every time — the same voice, the same
 * room, a little further from the microphone.
 *
 * Scaling the samples up costs nothing and fixes it. This is not cleverness; it
 * is the volume knob that the recording never had.
 */

// TargetPeak is what a recording is raised to before it is transcribed.
//
// Short of full scale, because the peak is one sample and the rest of the
// waveform goes up with it — leaving headroom means the loud parts of a
// sentence do not flatten into a square wave, which whisper hears as worse than
// the quiet original.
const TargetPeak = 22000

/*
 * MostGain bounds the scaling, and LeastSignal decides whether to scale at all.
 *
 * The bound cannot be a plain number. The recordings that needed this peaked at
 * 215 and 282, and reaching a usable level from there takes eighty times, not
 * twelve — a cap of twelve leaves them exactly as unusable as they were, which
 * a test caught the moment it was written.
 *
 * What actually distinguishes a quiet voice from a loud silence is not the peak
 * but its distance from the rest of the recording. Speech towers over the gaps
 * between words; an empty room does not. So the gain is bounded generously and
 * refused entirely when the recording has no shape to it — amplifying that by
 * eighty makes a loud recording of an empty room, which the recogniser
 * obligingly invents something to hear in.
 */
const (
	MostGain = 80.0

	// LeastSignal is how far the loudest part must stand above the quiet parts
	// before this believes somebody spoke.
	LeastSignal = 3.0
)

/*
 * Normalise raises a recording towards a level the recogniser can work with.
 *
 * Reports the gain applied, for the record: a turn that needed eight times its
 * own volume to be understood is telling its owner something about where they
 * are sitting.
 */
func Normalise(path string) (float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 1, err
	}

	const header = 44

	if len(raw) <= header {
		return 1, nil
	}

	samples := raw[header:]
	count := len(samples) / 2

	if count == 0 {
		return 1, nil
	}

	peak, quiet := shape(samples, count)

	// Silence stays silence. Multiplying nothing is still nothing, and
	// pretending otherwise gives the recogniser noise to hallucinate over.
	if peak < 40 {
		return 1, nil
	}

	/*
	 * And so does noise, however loud.
	 *
	 * A recording where the loudest moment is barely above the quietest has no
	 * speech in it — a fan, a road, a room. Turning that up produces exactly
	 * the confident nonsense this is meant to avoid.
	 */
	if quiet > 0 && float64(peak)/float64(quiet) < LeastSignal {
		return 1, nil
	}

	gain := float64(TargetPeak) / float64(peak)

	if gain > MostGain {
		gain = MostGain
	}

	// Already loud enough; leave it exactly as it was.
	if gain <= 1.05 {
		return 1, nil
	}

	for i := 0; i < count; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(samples[i*2:]))) * gain

		// Clamped rather than allowed to wrap. An overflowed sample flips from
		// the top of the range to the bottom, which is a click loud enough to
		// be the only thing in the recording.
		switch {
		case v > 32767:
			v = 32767
		case v < -32768:
			v = -32768
		}

		binary.LittleEndian.PutUint16(samples[i*2:], uint16(int16(v)))
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return 1, fmt.Errorf("writing the louder copy: %w", err)
	}

	return gain, nil
}

/*
 * shape reports the loudest sample and a quiet one.
 *
 * The quiet one is a low percentile rather than the minimum, because every
 * waveform crosses zero and the minimum is therefore always zero — which would
 * make every recording look like it had infinite dynamic range.
 */
func shape(samples []byte, count int) (peak, quiet int) {
	// One bucket per 256 values is enough to find a percentile without sorting
	// a minute of audio.
	const buckets = 128

	var histogram [buckets]int

	for i := 0; i < count; i++ {
		v := int(int16(binary.LittleEndian.Uint16(samples[i*2:])))

		if v < 0 {
			v = -v
		}

		if v > peak {
			peak = v
		}

		histogram[v*(buckets-1)/32767]++
	}

	// The level below which four fifths of the samples fall: the body of the
	// recording rather than its loudest moment.
	want := count * 4 / 5
	seen := 0

	for i, n := range histogram {
		seen += n

		if seen >= want {
			quiet = i * 32767 / (buckets - 1)

			break
		}
	}

	// A floor of one, so the ratio below is never a division by zero.
	if quiet < 1 {
		quiet = 1
	}

	return peak, quiet
}
