package speech

import (
	"encoding/binary"
	"io"
	"math"
)

/*
 * Making a voice sound like a machine without making it hard to follow.
 *
 * The robot used to be espeak — a formant synthesiser, which is genuinely a
 * machine talking and is also the reason nobody could make out what it said.
 * Its owner asked for a robot that is clear, which are not opposites: the
 * words and the timbre come from different places. So the words come from a
 * neural voice, which is as clear as speech gets on this machine, and the
 * timbre is put on afterwards.
 *
 * What is put on is amplitude modulation — the volume swung up and down sixty
 * times a second. That is the oldest robot in broadcasting and it works
 * because it adds a metallic buzz around every sound without touching which
 * sound it is: the formants that carry the vowels are untouched, so every word
 * arrives as the neural voice said it.
 *
 * Depth is the whole argument. At full depth the carrier swamps the voice and
 * it is a Dalek — unmistakably a machine and half of it unintelligible, which
 * is the thing being fixed. Held well below that it reads as a machine and
 * stays as clear as the voice underneath.
 */

/*
 * RobotHz is how fast the volume is swung, and RobotFloor is how quiet it gets
 * at the bottom of each swing.
 *
 * Sixty is low enough to buzz rather than whistle and high enough not to be
 * heard as the voice wobbling.
 *
 * The floor is the whole argument, and it is written as a floor rather than as
 * a depth so that it says what it does: at its quietest the sound drops to
 * this much of full, and it never goes above full, so nothing clips. At 1.0
 * there is no robot at all; near 0 the carrier swamps the voice and it is a
 * Dalek — unmistakably a machine and half of it unintelligible, which is the
 * thing being fixed. Just under a half reads as a machine and leaves every
 * word where the neural voice put it.
 *
 * One number, deliberately, because it is the one somebody will want to move.
 */
const (
	RobotHz    = 60.0
	RobotFloor = 0.45
)

/*
 * robotise wraps a stream of sixteen-bit samples with the treatment.
 *
 * A reader rather than a buffer, because the audio is played as it is
 * generated — piper writes the first words while it is still working out the
 * last, and holding the whole utterance to process it would put the delay back
 * that streaming was there to remove.
 */
func robotise(from io.Reader, rate int) io.Reader {
	if rate <= 0 {
		rate = 22050
	}

	return &robot{from: from, step: 2 * math.Pi * RobotHz / float64(rate)}
}

type robot struct {
	from io.Reader
	step float64

	// phase carries across reads, since a chunk boundary is not a boundary in
	// the sound — restarting the carrier at every read would click.
	phase float64

	// odd holds back the second half of a sample when a read ends between its
	// two bytes, so it can be treated with its other half next time.
	odd    byte
	hasOdd bool
}

/*
 * Read treats whole samples and never half of one.
 *
 * The audio arrives in whatever sizes the pipe hands over, so a read can end
 * between the two bytes of a sample. The first version of this kept that byte
 * and treated it on the next read — which cannot work, because the byte had
 * already been handed to the player untreated and there is no going back to
 * it. The rest of the sentence then came out shifted by half a sample, which
 * is noise.
 *
 * So a trailing half sample is held back instead: not returned at all this
 * time, and put at the front of the next read. The caller sees one byte fewer
 * and hears nothing missing.
 */
func (r *robot) Read(into []byte) (int, error) {
	if len(into) < 2 {
		// Two bytes is one sample; anything smaller cannot be treated and
		// would loop for ever holding a byte back.
		return 0, io.ErrShortBuffer
	}

	for {
		at := 0

		if r.hasOdd {
			into[0] = r.odd
			r.hasOdd = false
			at = 1
		}

		n, err := r.from.Read(into[at:])
		total := at + n

		if total == 0 {
			if err != nil {
				return 0, err
			}

			// A read that returned nothing without failing. Ask again rather
			// than passing an empty read up, which some callers spin on.
			continue
		}

		/*
		 * The last byte, when there is an odd number of them.
		 *
		 * Held over unless this is the end of the sound — at the end there is
		 * no next read to join it to, and half a sample is better handed on
		 * untreated than dropped.
		 */
		if total%2 == 1 && err == nil {
			r.odd = into[total-1]
			r.hasOdd = true
			total--

			if total == 0 {
				continue
			}
		}

		for i := 0; i+1 < total; i += 2 {
			sample := int16(binary.LittleEndian.Uint16(into[i:]))

			binary.LittleEndian.PutUint16(into[i:], uint16(r.shape(sample)))
		}

		return total, err
	}
}

// shape applies the carrier to one sample.
func (r *robot) shape(sample int16) int16 {
	// A carrier that runs between the floor and full, rather than either side
	// of full — so the loudest moment is the sound as the voice made it and
	// nothing has to be clipped back into range.
	gain := RobotFloor + (1-RobotFloor)*(0.5+0.5*math.Sin(r.phase))

	r.phase += r.step

	// Kept inside one turn so the number stays small enough for the sine to
	// remain accurate over a long utterance.
	if r.phase > 2*math.Pi {
		r.phase -= 2 * math.Pi
	}

	/*
	 * Rounded, not truncated.
	 *
	 * Truncation always moves a sample towards zero, which across a whole
	 * utterance is a small constant pull towards silence and, on the quietest
	 * samples, a large one. Rounding costs nothing and leaves the error
	 * even-handed.
	 */
	out := math.Round(float64(sample) * gain)

	// Clipping cannot happen while the carrier never exceeds one, but the
	// arithmetic says so rather than the comment alone.
	if out > math.MaxInt16 {
		return math.MaxInt16
	}

	if out < math.MinInt16 {
		return math.MinInt16
	}

	return int16(out)
}
