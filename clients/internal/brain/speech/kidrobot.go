package speech

/*
 * Making the robot a small one.
 *
 * A machine voice and a *little* machine voice are different characters, and
 * the difference is almost entirely pitch. The words come from a neural voice
 * built on an adult, so the pitch has to be moved afterwards — which for a
 * stream of samples means reading them out faster than they were written and
 * interpolating between them.
 *
 * That also shortens the sound, so it speaks faster as well as higher. Both
 * happening together is what a small machine sounds like: quick and bright
 * rather than slow and deep, and it is why a chipmunk sounds young rather than
 * merely high. Overdone it becomes a squeak with no words in it, which is the
 * same failure the old espeak robot had at the other end of the scale.
 */

/*
 * KidPitch is how much faster the samples are read than they were written.
 *
 * A quarter again. Enough that nobody would call it an adult, small enough
 * that every consonant survives — past about 1.4 the formants have moved so
 * far that "s" and "f" stop being distinguishable, and a robot that cannot say
 * "first" is a costume rather than a voice.
 */
const KidPitch = 1.26

/*
 * pitchUp reads samples at a fractional step, interpolating between them.
 *
 * Linear interpolation rather than anything better, deliberately. The sound is
 * about to have a ring modulator and a comb filter put on it, both of which
 * add far more to the spectrum than the interpolation error takes away, and
 * this runs on the same four cores the model is using.
 */
type pitchUp struct {
	step float64

	// at is where in the input the next output sample sits, and held is the
	// last input sample, so interpolation can span a read boundary.
	at   float64
	held float64
	have bool
}

/*
 * next produces output samples from a run of input samples.
 *
 * Written as "give me everything you can from this" rather than one sample at
 * a time, because the caller has a buffer of one and a buffer of the other and
 * the arithmetic for which input a given output falls between is the whole of
 * the problem.
 */
func (p *pitchUp) next(in []float64, out []float64) (used, made int) {
	if p.step <= 0 {
		p.step = KidPitch
	}

	for made < len(out) {
		// Which input sample the next output falls after.
		index := int(p.at)

		if index+1 >= len(in) {
			break
		}

		fraction := p.at - float64(index)

		out[made] = in[index]*(1-fraction) + in[index+1]*fraction

		made++

		p.at += p.step
	}

	/*
	 * What has been consumed, keeping the sample the next read will
	 * interpolate from.
	 *
	 * Dropping it would put a discontinuity at every read boundary — a click,
	 * several times a second, which is the same fault the carrier phase and
	 * the delay line each had to be carried across reads to avoid.
	 */
	used = int(p.at)

	if used > 0 {
		if used >= len(in) {
			used = len(in)
		}

		p.held = in[used-1]
		p.have = true
		p.at -= float64(used)
	}

	return used, made
}
