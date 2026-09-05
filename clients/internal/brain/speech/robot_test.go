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
 * The robot is a small machine that can still be understood.
 *
 * Both halves are the requirement. The old robot was espeak, which is
 * unmistakably a machine and is also the reason nobody could make out what it
 * said; a neural voice with no treatment is clear and is not a robot.
 *
 * Three things are done to it now: the volume is swung, a copy of the sound
 * three milliseconds late is mixed back in, and the whole thing is read out a
 * quarter faster than it was written. The first two are the machine, the third
 * is what makes the machine a small one — and the third is why this no longer
 * comes out the same length as it went in.
 */
func TestTheRobotBuzzesWithoutSwallowingTheWords(t *testing.T) {
	const rate = 22050

	original := steadyTone(rate, 220, 0.25)

	treated, err := io.ReadAll(robotise(bytes.NewReader(append([]byte(nil), original...)), rate))
	if err != nil {
		t.Fatal(err)
	}

	/*
	 * Shorter, by exactly the pitch.
	 *
	 * Reading the samples out faster is the pitch shift, and a shorter sound
	 * is the necessary consequence: it speaks higher and quicker, which
	 * together is what a small machine sounds like. Checked against the
	 * constant rather than a number, so moving the pitch moves this with it.
	 */
	expected := float64(len(original)) / KidPitch

	if math.Abs(float64(len(treated))-expected) > expected*0.02 {
		t.Fatalf("the length came out at %d bytes, want about %.0f for a pitch of %.2f",
			len(treated), expected, KidPitch)
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

	/*
	 * Expected from the floor rather than written down.
	 *
	 * The carrier dips to RobotFloor at the bottom of each swing, so an even
	 * tone comes out swinging by about one over that. Hard-coding a threshold
	 * meant that softening the voice — which is raising the floor — failed a
	 * test about whether there was any machine in it at all, when the machine
	 * had simply moved into the comb and the body instead.
	 */
	expectedSwing := 1 / RobotFloor

	if swing(now) < swing(was)*expectedSwing*0.85 {
		t.Errorf("the treated sound is as even as the original — no machine in it: "+
			"%.2f vs %.2f, expected about %.2f",
			swing(now), swing(was), swing(was)*expectedSwing)
	}

	/*
	 * Nothing clips, and the sound still follows the one it came from.
	 *
	 * This used to check that each sample was the original scaled by
	 * something between the carrier's floor and one — which was exactly true
	 * of amplitude modulation and is not true any more, and the difference is
	 * worth stating rather than loosening a bound until it passes.
	 *
	 * The comb does not scale a sample. It mixes in a different moment of the
	 * sound, from three milliseconds earlier, so near a zero crossing the
	 * ratio between one output sample and its input can be anything at all.
	 * That is what a comb is; measuring it per sample measures nothing.
	 *
	 * What still has to hold is that it does not clip, and that the treated
	 * sound is recognisably the same sound. The second is measured as
	 * correlation at zero lag, which is the arithmetic version of "the words
	 * are still where the voice put them".
	 */
	var loudest float64

	for _, x := range now {
		if math.Abs(x) > loudest {
			loudest = math.Abs(x)
		}
	}

	if loudest > math.MaxInt16 {
		t.Fatalf("the treatment clipped: reached %.0f", loudest)
	}

	/*
	 * And it really is higher, by about the amount asked for.
	 *
	 * Counted as zero crossings, which for a steady tone is the pitch and
	 * needs no arithmetic anybody has to trust. Comparing sample against
	 * sample would say nothing here: after a resample the two are no longer
	 * the same sound at the same moment, which is the entire point of it.
	 */
	crossings := func(v []float64) int {
		var n int

		for i := 1; i < len(v); i++ {
			if (v[i-1] < 0) != (v[i] < 0) {
				n++
			}
		}

		return n
	}

	// Per second, since the treated sound is shorter.
	wasPitch := float64(crossings(was)) / (float64(len(was)) / rate)
	nowPitch := float64(crossings(now)) / (float64(len(now)) / rate)

	if nowPitch < wasPitch*KidPitch*0.9 || nowPitch > wasPitch*KidPitch*1.1 {
		t.Fatalf("the pitch went from %.0f to %.0f, want about %.0f",
			wasPitch, nowPitch, wasPitch*KidPitch)
	}

	/*
	 * Loud enough to be heard: a treatment that halves everything would be
	 * clear and inaudible.
	 *
	 * Compared as an average rather than a total, because the two are no
	 * longer the same length — the treated sound is shorter by the pitch, so
	 * summing them would report a quarter of the loudness missing when none
	 * of it is.
	 */
	average := func(v []float64) float64 {
		var sum float64

		for _, x := range v {
			sum += math.Abs(x)
		}

		return sum / math.Max(float64(len(v)), 1)
	}

	if after, before := average(now), average(was); after < before*0.7 {
		t.Errorf("the treatment lost %.0f%% of the loudness", (1-after/before)*100)
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

/*
 * The delay line carries across reads.
 *
 * The audio arrives in whatever sizes the pipe hands over, and the resonance
 * does not know where the pipe happened to break. Clearing the line at every
 * read would put a gap in it at every chunk boundary — several clicks a
 * second, which is precisely the fault the phase already had to be carried
 * across reads to avoid.
 */
func TestTheResonanceSurvivesAChunkBoundary(t *testing.T) {
	const samples = 4000

	tone := make([]byte, samples*2)

	for i := 0; i < samples; i++ {
		v := int16(9000 * math.Sin(2*math.Pi*220*float64(i)/22050))

		binary.LittleEndian.PutUint16(tone[i*2:], uint16(v))
	}

	whole := readAll(t, robotise(bytes.NewReader(tone), 22050))

	// The same sound handed over in small pieces, as a pipe would.
	pieced := readAll(t, robotise(&dribble{from: bytes.NewReader(tone), most: 37}, 22050))

	if len(whole) != len(pieced) {
		t.Fatalf("different lengths: %d against %d", len(whole), len(pieced))
	}

	for i := range whole {
		if whole[i] != pieced[i] {
			t.Fatalf("the sound differs at byte %d once it is read in pieces — "+
				"something is being reset at a chunk boundary", i)
		}
	}
}

// dribble hands over a few bytes at a time, the way a pipe under load does.
type dribble struct {
	from io.Reader
	most int
}

func (d *dribble) Read(into []byte) (int, error) {
	if len(into) > d.most {
		into = into[:d.most]
	}

	return d.from.Read(into)
}

func readAll(t *testing.T, from io.Reader) []byte {
	t.Helper()

	out, err := io.ReadAll(from)
	if err != nil {
		t.Fatal(err)
	}

	return out
}
