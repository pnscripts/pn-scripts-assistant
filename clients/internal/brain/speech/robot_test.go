package speech

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// a second of a steady tone, which is what a vowel largely is.
func steadyTone(rate int, hz float64, seconds float64) []byte {
	samples := int(float64(rate) * seconds)
	out := make([]byte, samples*2)

	for i := 0; i < samples; i++ {
		v := math.Sin(2*math.Pi*hz*float64(i)/float64(rate)) * 8000
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(v)))
	}

	return out
}

func samplesOf(b []byte) []float64 {
	out := make([]float64, len(b)/2)

	for i := range out {
		out[i] = float64(int16(binary.LittleEndian.Uint16(b[i*2:])))
	}

	return out
}

/*
 * The robot is a machine that can still be understood.
 *
 * Both halves are the requirement. The old robot was espeak, which is
 * unmistakably a machine and is also the reason nobody could make out what it
 * said; a neural voice with no treatment is clear and is not a robot. What
 * this does is swing the volume without touching which sound is being made —
 * so the buzz is added and the vowels are left exactly where they were.
 */
func TestTheRobotBuzzesWithoutSwallowingTheWords(t *testing.T) {
	const rate = 22050

	original := steadyTone(rate, 220, 0.25)

	treated, err := io.ReadAll(robotise(bytes.NewReader(append([]byte(nil), original...)), rate))
	if err != nil {
		t.Fatal(err)
	}

	if len(treated) != len(original) {
		t.Fatalf("the treatment changed the length: %d bytes in, %d out", len(original), len(treated))
	}

	was := samplesOf(original)
	now := samplesOf(treated)

	/*
	 * The volume swings, which is the machine part.
	 *
	 * Measured as the ratio between the loudest and quietest stretch: the
	 * original tone is even, and the treated one rises and falls by the depth
	 * of the carrier.
	 */
	swing := func(v []float64) float64 {
		loudest, quietest := 0.0, math.MaxFloat64

		for i := 0; i+80 < len(v); i += 80 {
			var peak float64

			for _, x := range v[i : i+80] {
				if math.Abs(x) > peak {
					peak = math.Abs(x)
				}
			}

			if peak > loudest {
				loudest = peak
			}

			if peak < quietest {
				quietest = peak
			}
		}

		return loudest / math.Max(quietest, 1)
	}

	if swing(now) <= swing(was)*1.2 {
		t.Errorf("the treated sound is as even as the original — no machine in it: %.2f vs %.2f",
			swing(now), swing(was))
	}

	/*
	 * And every sample is still a scaled version of the one it came from,
	 * never inverted and never louder.
	 *
	 * That is what keeps the words: the shape of the wave decides which sound
	 * a listener hears, and this only changes how loud it is at each instant.
	 */
	for i := range was {
		/*
		 * Only where the sample is big enough for the ratio to mean anything.
		 *
		 * Near a zero crossing a sample may be 3, and rounding the result to a
		 * whole number moves it by a third — which is arithmetic on integers,
		 * not the treatment doing something wrong. Above a hundred the
		 * quantisation is under one per cent.
		 */
		if math.Abs(was[i]) < 100 {
			continue
		}

		gain := now[i] / was[i]

		/*
		 * Between the floor and full, and never above it.
		 *
		 * Never above is what keeps it from clipping: the loudest moment is
		 * the sound exactly as the voice made it, and everything else is a
		 * dip. Never below the floor is what keeps the words.
		 */
		if gain < RobotFloor-0.01 || gain > 1+0.01 {
			t.Fatalf("sample %d was scaled by %.3f, outside %.2f to 1", i, gain, RobotFloor)
		}
	}

	// Loud enough to be heard: a treatment that halves everything would be
	// clear and inaudible.
	var beforeSum, afterSum float64

	for i := range was {
		beforeSum += math.Abs(was[i])
		afterSum += math.Abs(now[i])
	}

	if afterSum < beforeSum*0.7 {
		t.Errorf("the treatment lost %.0f%% of the loudness", (1-afterSum/beforeSum)*100)
	}
}

/*
 * A read can end between the two bytes of a sample.
 *
 * The audio arrives in whatever sizes the pipe hands over, and dropping the
 * odd byte would shift every sample after it by half a sample — which turns
 * the rest of the sentence into noise. Checked by feeding it in awkward
 * pieces and requiring the same answer as one go.
 */
func TestAnOddByteAtTheEndOfAReadIsNotLost(t *testing.T) {
	const rate = 22050

	original := steadyTone(rate, 300, 0.05)

	whole, err := io.ReadAll(robotise(bytes.NewReader(append([]byte(nil), original...)), rate))
	if err != nil {
		t.Fatal(err)
	}

	// Three bytes at a time: every other read ends mid-sample.
	piecemeal, err := io.ReadAll(robotise(&awkward{from: append([]byte(nil), original...), at: 3}, rate))
	if err != nil {
		t.Fatal(err)
	}

	if len(whole) != len(piecemeal) {
		t.Fatalf("lengths differ: %d and %d", len(whole), len(piecemeal))
	}

	for i := range whole {
		if whole[i] != piecemeal[i] {
			t.Fatalf("byte %d differs when the audio arrives in pieces", i)
		}
	}
}

// awkward hands over a few bytes at a time, the way a pipe does.
type awkward struct {
	from []byte
	at   int
}

func (a *awkward) Read(into []byte) (int, error) {
	if len(a.from) == 0 {
		return 0, io.EOF
	}

	n := a.at

	if n > len(a.from) {
		n = len(a.from)
	}

	if n > len(into) {
		n = len(into)
	}

	copy(into, a.from[:n])
	a.from = a.from[n:]

	return n, nil
}
