package voiceprint

import (
	"math"
	"testing"
)

/*
 * The features have to match what the model was trained on, exactly.
 *
 * Nothing here is a choice — eighty mel bins over twenty-five-millisecond
 * windows every ten, Kaldi's window and Kaldi's mel scale — and features that
 * are nearly right do not produce nearly-right embeddings. They produce
 * confident nonsense, because the model has no way to report that its input
 * looks unfamiliar.
 */
func TestTheFilterbankHasTheShapeTheModelWants(t *testing.T) {
	// A second of a vowel-ish tone.
	samples := make([]float64, SampleRate)

	for i := range samples {
		samples[i] = 6000 * math.Sin(2*math.Pi*220*float64(i)/SampleRate)
	}

	features := Fbank(samples)

	// 25ms windows every 10ms over one second.
	if want := 1 + (SampleRate-WindowSamples)/StepSamples; len(features) != want {
		t.Errorf("%d frames, want %d", len(features), want)
	}

	for i, row := range features {
		if len(row) != Bins {
			t.Fatalf("frame %d has %d bins, want %d", i, len(row), Bins)
		}
	}

	/*
	 * A 220Hz tone puts its energy in the low bins and leaves the high ones
	 * near the floor. If the mel scale or the FFT were wrong this is the check
	 * that fails: the peak lands somewhere else entirely.
	 */
	middle := features[len(features)/2]

	loudest := 0

	for b, v := range middle {
		if v > middle[loudest] {
			loudest = b
		}
	}

	if loudest > 12 {
		t.Errorf("a 220Hz tone peaks at mel bin %d, which is far too high", loudest)
	}

	if middle[Bins-1] >= middle[loudest] {
		t.Error("the top of the spectrum is as loud as the tone, so the filters are wrong")
	}
}

/*
 * Mean normalisation removes what every frame has in common.
 *
 * Which is the microphone and the room. Without it a voiceprint enrolled here
 * describes this microphone as much as the person, and the same voice through
 * a different one lands somewhere else.
 */
func TestMeanNormalisationRemovesTheRoom(t *testing.T) {
	features := [][]float64{
		make([]float64, Bins),
		make([]float64, Bins),
		make([]float64, Bins),
	}

	for b := 0; b < Bins; b++ {
		features[0][b] = 1 + float64(b)
		features[1][b] = 2 + float64(b)
		features[2][b] = 3 + float64(b)
	}

	MeanNormalise(features)

	for b := 0; b < Bins; b++ {
		var mean float64

		for _, row := range features {
			mean += row[b]
		}

		if math.Abs(mean/3) > 1e-9 {
			t.Fatalf("bin %d still averages %.6f", b, mean/3)
		}
	}

	// And the differences between frames survive, or it would have removed the
	// speech along with the room.
	if features[2][0]-features[0][0] != 2 {
		t.Errorf("the shape between frames changed: %v", features)
	}
}

/*
 * The transform against a signal whose answer is known.
 *
 * A pure tone at a bin centre puts all its energy in that bin and none
 * anywhere else, which catches a bit-reversal or twiddle-factor mistake that
 * would otherwise only show up as embeddings that cluster slightly wrongly.
 */
func TestTheTransformPutsAToneWhereItBelongs(t *testing.T) {
	const bin = 40

	samples := make([]float64, FFTSize)

	for i := range samples {
		samples[i] = math.Cos(2 * math.Pi * float64(bin) * float64(i) / FFTSize)
	}

	power := make([]float64, FFTSize/2+1)
	powerSpectrum(samples, power)

	loudest := 0

	for k, v := range power {
		if v > power[loudest] {
			loudest = k
		}
	}

	if loudest != bin {
		t.Errorf("a tone at bin %d came out at bin %d", bin, loudest)
	}

	// Everything else is essentially nothing.
	for k, v := range power {
		if k == bin {
			continue
		}

		if v > power[bin]*1e-6 {
			t.Errorf("bin %d holds %.3g of the tone's %.3g", k, v, power[bin])

			break
		}
	}
}
