/*
 * Package voiceprint tells one speaker's voice from another's.
 *
 * The brain hears a room, not a person. A television, somebody else in the
 * house, a video playing on the machine's own speakers — all of it arrives at
 * the microphone as speech, and every test up to this one asks only whether
 * speech happened: loud enough, modulated like a voice, addressed by name.
 * None of them can tell whose voice it is, which is the question actually
 * being asked when somebody says "listen to me, not to that".
 *
 * So this measures the voice itself. A model turns a few seconds of audio into
 * five hundred and twelve numbers describing the speaker rather than the
 * words, and two recordings of the same person land close together in that
 * space while two different people do not. Enrol once, compare ever after.
 *
 * This file is the half that happens before the model: turning samples into
 * the features it was trained on. It is written to match Kaldi's filterbank
 * exactly, because that is what the model was trained against and features
 * that are nearly right produce embeddings that are confidently wrong.
 */
package voiceprint

import (
	"math"
)

/*
 * The shape of the features, all of it fixed by the model.
 *
 * These are not choices. A model trained on eighty mel bins over
 * twenty-five-millisecond windows every ten milliseconds cannot be fed
 * anything else, and feeding it something close produces numbers that look
 * like an answer.
 */
const (
	SampleRate = 16000

	// Bins is the number of mel filters, which is the model's input width.
	Bins = 80

	// WindowSamples is 25ms and StepSamples is 10ms at 16kHz.
	WindowSamples = 400
	StepSamples   = 160

	// FFTSize is the next power of two that holds a window.
	FFTSize = 512

	// LowHz and HighHz bound the filterbank. Kaldi's defaults: everything from
	// twenty hertz to the Nyquist limit.
	LowHz  = 20.0
	HighHz = SampleRate / 2

	// Preemphasis lifts the high frequencies before the window, as Kaldi does.
	Preemphasis = 0.97
)

/*
 * Fbank turns samples into the log mel filterbank the model expects.
 *
 * samples are mono, sixteen kilohertz, scaled to whole numbers the way a
 * sixteen-bit recording arrives — Kaldi works in that scale and the log at the
 * end is not scale-invariant, so passing normalised floats would shift every
 * feature by a constant and quietly move every voice in the same direction.
 */
func Fbank(samples []float64) [][]float64 {
	if len(samples) < WindowSamples {
		return nil
	}

	melBank := melFilters()
	window := poveyWindow()

	frames := 1 + (len(samples)-WindowSamples)/StepSamples
	out := make([][]float64, 0, frames)

	buffer := make([]float64, FFTSize)
	power := make([]float64, FFTSize/2+1)

	for f := 0; f < frames; f++ {
		frame := samples[f*StepSamples : f*StepSamples+WindowSamples]

		/*
		 * Kaldi's order, which matters: the mean is removed first, then the
		 * pre-emphasis filter, then the window. Doing any of them out of turn
		 * changes the result by a little at every frame, which is exactly the
		 * kind of error that produces plausible embeddings.
		 */
		var mean float64

		for _, s := range frame {
			mean += s
		}

		mean /= float64(len(frame))

		for i := range buffer {
			buffer[i] = 0
		}

		for i := WindowSamples - 1; i > 0; i-- {
			buffer[i] = (frame[i] - mean) - Preemphasis*(frame[i-1]-mean)
		}

		// The first sample has nothing before it; Kaldi repeats it.
		buffer[0] = (frame[0] - mean) * (1 - Preemphasis)

		for i := 0; i < WindowSamples; i++ {
			buffer[i] *= window[i]
		}

		powerSpectrum(buffer, power)

		row := make([]float64, Bins)

		for b := 0; b < Bins; b++ {
			var sum float64

			for i, weight := range melBank[b] {
				sum += weight * power[melStart[b]+i]
			}

			// Kaldi's floor, so silence does not become negative infinity.
			row[b] = math.Log(math.Max(sum, 1.1920928955078125e-07))
		}

		out = append(out, row)
	}

	return out
}

/*
 * MeanNormalise subtracts each feature's average over the utterance.
 *
 * What the model was trained with, and it is doing real work rather than
 * tidying: it removes whatever the microphone and the room add to every frame
 * alike, so the same voice through a different microphone still lands in the
 * same place. Without it a voiceprint enrolled on this machine's microphone
 * describes the microphone as much as the person.
 */
func MeanNormalise(features [][]float64) {
	if len(features) == 0 {
		return
	}

	for b := 0; b < Bins; b++ {
		var mean float64

		for _, row := range features {
			mean += row[b]
		}

		mean /= float64(len(features))

		for _, row := range features {
			row[b] -= mean
		}
	}
}

// poveyWindow is Kaldi's default window: a Hann raised to the power 0.85.
func poveyWindow() []float64 {
	w := make([]float64, WindowSamples)

	for i := range w {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(WindowSamples-1))
		w[i] = math.Pow(hann, 0.85)
	}

	return w
}

// melStart records where each filter begins in the spectrum, so the sum above
// does not have to walk the bins it knows are zero.
var melStart [Bins]int

/*
 * melFilters builds the triangular filterbank, spaced evenly in mel.
 *
 * The mel scale is the one Kaldi uses — 1127·ln(1 + f/700) — and not the
 * several other definitions in circulation, each of which would place every
 * filter slightly differently.
 */
func melFilters() [][]float64 {
	toMel := func(hz float64) float64 { return 1127 * math.Log(1+hz/700) }
	fromMel := func(mel float64) float64 { return 700 * (math.Exp(mel/1127) - 1) }

	lowMel, highMel := toMel(LowHz), toMel(HighHz)
	step := (highMel - lowMel) / float64(Bins+1)

	bank := make([][]float64, Bins)
	binWidth := float64(SampleRate) / float64(FFTSize)

	for b := 0; b < Bins; b++ {
		left := fromMel(lowMel + float64(b)*step)
		centre := fromMel(lowMel + float64(b+1)*step)
		right := fromMel(lowMel + float64(b+2)*step)

		first, last := -1, -1
		var weights []float64

		for i := 0; i <= FFTSize/2; i++ {
			hz := float64(i) * binWidth

			var weight float64

			switch {
			case hz > left && hz <= centre:
				weight = (hz - left) / (centre - left)
			case hz > centre && hz < right:
				weight = (right - hz) / (right - centre)
			}

			if weight > 0 {
				if first < 0 {
					first = i
				}

				last = i
			}

			if first >= 0 {
				weights = append(weights, weight)
			}
		}

		if first < 0 {
			first = 0
		}

		if last >= first && len(weights) > last-first+1 {
			weights = weights[:last-first+1]
		}

		melStart[b] = first
		bank[b] = weights
	}

	return bank
}
