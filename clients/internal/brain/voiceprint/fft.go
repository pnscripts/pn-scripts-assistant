package voiceprint

import "math"

/*
 * The one transform this needs, written out rather than depended on.
 *
 * A radix-2 Cooley-Tukey FFT over 512 real samples, which is thirty lines and
 * has no configuration to get wrong. The alternative is a dependency carrying
 * a general-purpose signal processing library for one call in one place, in a
 * program whose whole shape is one binary with nothing to install first.
 */

var (
	bitReversed [FFTSize]int
	twiddleCos  [FFTSize / 2]float64
	twiddleSin  [FFTSize / 2]float64
)

func init() {
	bits := 0

	for size := FFTSize; size > 1; size >>= 1 {
		bits++
	}

	for i := 0; i < FFTSize; i++ {
		reversed := 0

		for b := 0; b < bits; b++ {
			if i&(1<<b) != 0 {
				reversed |= 1 << (bits - 1 - b)
			}
		}

		bitReversed[i] = reversed
	}

	for i := range twiddleCos {
		angle := -2 * math.Pi * float64(i) / float64(FFTSize)
		twiddleCos[i] = math.Cos(angle)
		twiddleSin[i] = math.Sin(angle)
	}
}

/*
 * powerSpectrum fills power with |X(k)|² for the first half of the spectrum.
 *
 * The input is real, so the second half is the mirror of the first and carries
 * nothing — computing it would double the work to throw the answer away.
 */
func powerSpectrum(samples []float64, power []float64) {
	var re, im [FFTSize]float64

	for i := 0; i < FFTSize; i++ {
		re[bitReversed[i]] = samples[i]
	}

	for size := 2; size <= FFTSize; size <<= 1 {
		half := size / 2
		step := FFTSize / size

		for start := 0; start < FFTSize; start += size {
			for k := 0; k < half; k++ {
				c, s := twiddleCos[k*step], twiddleSin[k*step]

				i, j := start+k, start+k+half

				tr := re[j]*c - im[j]*s
				ti := re[j]*s + im[j]*c

				re[j] = re[i] - tr
				im[j] = im[i] - ti
				re[i] += tr
				im[i] += ti
			}
		}
	}

	for k := 0; k <= FFTSize/2; k++ {
		power[k] = re[k]*re[k] + im[k]*im[k]
	}
}
