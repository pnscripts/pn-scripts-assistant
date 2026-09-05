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
	RobotFloor = 0.86
)

/*
 * Softness, which is the point of the numbers above being where they are.
 *
 * A machine voice can be hard or soft and the difference is not volume: it is
 * how much of the treatment sits on top of the words. A deep carrier, a strong
 * comb and a bright top end make a machine that sounds like it is shouting
 * through a grille. Held back, the same three make one that sounds built and
 * calm — which is what somebody wants from a thing that lives in their room
 * and talks to them at four in the morning.
 *
 * RobotSoftness is a plain low-pass: each sample is dragged towards the one
 * before it. That takes the edge off the consonants, which is where all the
 * harshness in a modulated voice lives, and it is one multiply per sample.
 *
 * Too much and it is speech through a wall. A third is enough to round it.
 */
const RobotSoftness = 0.30

/*
 * RobotBody is how much of the low end is taken out.
 *
 * The other half of sounding like a small machine, and the half that is
 * usually missed. A child's voice is not simply an adult's played faster — it
 * has less body, because there is less of the person making it, and a robot
 * the size of a person's arm has a chassis to match. Taking the bottom out is
 * what makes a voice sound like it is coming from something small rather than
 * from somebody large speaking quickly.
 *
 * A one-pole high-pass: the running average is subtracted, so what is left is
 * everything that moves faster than it. Gentle — past about half it stops
 * being a small chassis and starts being a telephone.
 */
const RobotBody = 0.22

/*
 * And a short delay mixed back in, which is where the metal comes from.
 *
 * Amplitude modulation alone buzzes. What makes something sound built rather
 * than merely buzzing is resonance: a copy of the sound arriving a few
 * milliseconds late reinforces some frequencies and cancels others, and the
 * ear reads that comb of peaks and notches as a small metal room. It is the
 * oldest trick for making a voice sound like it is coming out of a machine,
 * and it costs one buffer and one addition per sample.
 *
 * Three and a half milliseconds. Long enough for the comb to fall inside the
 * voice's own range and be heard as timbre; short enough that it is not heard
 * as an echo, which would be two voices rather than one machine.
 *
 * Mixed at a third. Past about a half the notches start eating consonants and
 * the words go — which is the whole thing this design refuses to trade away.
 */
const (
	RobotDelayMS = 3.5
	RobotMix     = 0.18

	/*
	 * A second, faster swing, a fifth as deep.
	 *
	 * One carrier is a buzz at a single pitch, and a single pitch is what
	 * makes a ring modulator sound like a effect rather than a thing. A second
	 * at an interval the ear does not resolve as a note adds the grain that
	 * reads as a mechanism — and at a fifth of the depth it cannot touch the
	 * words.
	 */
	RobotShimmerHz = 187.0
	RobotShimmer   = 0.02
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

	delay := int(float64(rate) * RobotDelayMS / 1000)

	if delay < 1 {
		delay = 1
	}

	return &robot{
		from:    from,
		step:    2 * math.Pi * RobotHz / float64(rate),
		shimmer: 2 * math.Pi * RobotShimmerHz / float64(rate),
		echo:    make([]float64, delay),
	}
}

type robot struct {
	from    io.Reader
	step    float64
	shimmer float64

	// phase carries across reads, since a chunk boundary is not a boundary in
	// the sound — restarting the carrier at every read would click.
	phase     float64
	shimmerAt float64

	/*
	 * echo is the last few milliseconds of sound, for the comb.
	 *
	 * A ring buffer, and it carries across reads for the same reason the phase
	 * does: the delay line does not know where the pipe happened to break, and
	 * clearing it every read would put a gap in the resonance at every chunk
	 * boundary — which is a click, several times a second.
	 */
	echo   []float64
	echoAt int

	// odd holds back the second half of a sample when a read ends between its
	// two bytes, so it can be treated with its other half next time.
	odd    byte
	hasOdd bool

	// pitch reads the samples out faster than they were written, which is what
	// makes the machine a small one. See kidrobot.go.
	pitch pitchUp

	// soft is the last sample out, for the low-pass that rounds the edges off,
	// and low is the running average the high-pass subtracts to thin the body.
	soft float64
	low  float64

	// The buffers, kept rather than allocated per read: this runs for every
	// sentence the assistant says, on a machine whose processor the model is
	// already using all of.
	raw   []byte
	in    []float64
	out   []float64
	ended bool
}

/*
 * Read fills the buffer with treated sound, reading as much as it needs.
 *
 * It used to treat the caller's buffer in place, one sample at a time, because
 * the treatment was a gain and a gain does not change how many samples there
 * are. Raising the pitch does: a quarter again as many samples go in as come
 * out, so the input has to be gathered separately and the output built from
 * it.
 *
 * The awkward part is unchanged and is worth restating. Audio arrives in
 * whatever sizes the pipe hands over, so a read can end between the two bytes
 * of a sample. A half sample is held back rather than passed on — not returned
 * at all this time, and put at the front of the next read — because a byte
 * already handed to the player cannot be taken back, and treating it late
 * shifts the rest of the sentence by half a sample, which is noise.
 */
func (r *robot) Read(into []byte) (int, error) {
	if len(into) < 2 {
		// Two bytes is one sample; anything smaller cannot be treated and
		// would loop for ever holding a byte back.
		return 0, io.ErrShortBuffer
	}

	want := len(into) / 2

	if cap(r.out) < want {
		r.out = make([]float64, want)
	}

	out := r.out[:want]

	for {
		if err := r.fill(want); err != nil && len(r.in) < 2 {
			return 0, err
		}

		used, made := r.pitch.next(r.in, out)

		// Everything consumed stays consumed, whether or not it produced
		// anything: the interpolator has taken what it needed from it.
		if used > 0 {
			r.in = append(r.in[:0], r.in[used:]...)
		}

		if made == 0 {
			if r.ended {
				return 0, io.EOF
			}

			continue
		}

		for i := 0; i < made; i++ {
			binary.LittleEndian.PutUint16(into[i*2:], uint16(r.shape(out[i])))
		}

		return made * 2, nil
	}
}

/*
 * fill gathers enough input for one buffer of output.
 *
 * A quarter again as much as is wanted, plus a sample: the interpolator needs
 * the one after the last position it reads from, and running one short would
 * make it produce nothing and ask again for ever.
 */
func (r *robot) fill(want int) error {
	need := int(float64(want)*KidPitch) + 2

	for len(r.in) < need && !r.ended {
		if cap(r.raw) == 0 {
			r.raw = make([]byte, 8192)
		}

		at := 0

		if r.hasOdd {
			r.raw[0] = r.odd
			r.hasOdd = false
			at = 1
		}

		n, err := r.from.Read(r.raw[at:])
		total := at + n

		if total%2 == 1 && err == nil {
			r.odd = r.raw[total-1]
			r.hasOdd = true
			total--
		}

		for i := 0; i+1 < total; i += 2 {
			r.in = append(r.in, float64(int16(binary.LittleEndian.Uint16(r.raw[i:]))))
		}

		if err != nil {
			r.ended = true

			return err
		}
	}

	return nil
}

// shape applies the carrier and the comb to one sample.
func (r *robot) shape(sample float64) int16 {
	// A carrier that runs between the floor and full, rather than either side
	// of full — so the loudest moment is the sound as the voice made it and
	// nothing has to be clipped back into range.
	gain := RobotFloor + (1-RobotFloor)*(0.5+0.5*math.Sin(r.phase))

	// And the finer grain over the top of it, shallow enough that it is heard
	// as texture rather than as a second note.
	gain *= 1 - RobotShimmer*(0.5+0.5*math.Sin(r.shimmerAt))

	r.phase += r.step
	r.shimmerAt += r.shimmer

	// Kept inside one turn so the numbers stay small enough for the sines to
	// remain accurate over a long utterance.
	if r.phase > 2*math.Pi {
		r.phase -= 2 * math.Pi
	}

	if r.shimmerAt > 2*math.Pi {
		r.shimmerAt -= 2 * math.Pi
	}

	/*
	 * Rounded, not truncated.
	 *
	 * Truncation always moves a sample towards zero, which across a whole
	 * utterance is a small constant pull towards silence and, on the quietest
	 * samples, a large one. Rounding costs nothing and leaves the error
	 * even-handed.
	 */
	swung := sample * gain

	/*
	 * The comb: the sound plus a copy of itself from a moment ago.
	 *
	 * Scaled by the square root of the two parts rather than by their sum.
	 * Dividing by 1+mix assumes the delayed copy always arrives in phase and
	 * adds, which is the one case it mostly does not: across a real voice the
	 * two are uncorrelated as often as not, and that normalisation took nearly
	 * half the loudness with it — a robot noticeably quieter than the other
	 * voices, which is a fault dressed as a character.
	 *
	 * The rare in-phase moment can now reach past full scale, and the clamp
	 * below is what catches it. That is the right way round: one clamped
	 * sample is inaudible, and a voice at half the level of every other is not.
	 */
	combed := (swung + RobotMix*r.echo[r.echoAt]) / math.Sqrt(1+RobotMix*RobotMix)

	// What goes into the delay line is the swung sound, not the combed one:
	// feeding the output back would make a resonator that rings on its own,
	// and a robot that hums between sentences is a fault, not a character.
	r.echo[r.echoAt] = swung

	r.echoAt++

	if r.echoAt == len(r.echo) {
		r.echoAt = 0
	}

	/*
	 * And the edge taken off.
	 *
	 * Each sample dragged towards the one before it, which is the plainest
	 * low-pass there is. The harshness in a modulated voice is all in the
	 * consonants, and this is what makes the difference between a machine
	 * talking and a machine shouting through a grille.
	 */
	r.soft += (combed - r.soft) * (1 - RobotSoftness)

	/*
	 * And the body taken out, which is what makes it small.
	 *
	 * The running average is everything slower than the ear hears as pitch;
	 * subtracting a part of it leaves a voice with less chest in it. Scaled
	 * back up afterwards, because thinning a sound also quietens it and a
	 * small machine should not also be a distant one.
	 */
	r.low += (r.soft - r.low) * 0.02

	out := math.Round((r.soft - RobotBody*r.low) / (1 - RobotBody))

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
