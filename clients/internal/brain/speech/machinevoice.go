package speech

import "fmt"

/*
 * A machine voice made by how it speaks, not by what is done to it afterwards.
 *
 * Every previous attempt at the robot treated the sound: espeak, then a ring
 * modulator, then a comb filter and a pitch shift on top of it. All of them
 * traded away the one thing the voice is for. A modulator adds a buzz around
 * every sound; a comb adds notches; a resample moves the formants. Each is a
 * small loss of intelligibility, they compound, and the result was a robot
 * that had to be asked to repeat itself.
 *
 * What actually makes a voice read as a machine is not distortion. It is
 * delivery: no emotional contour, no variation between one sentence and the
 * next, an even pace that never hurries or trails off. People hear that as a
 * machine because people cannot do it — and it costs nothing, because the
 * phonemes are untouched. A flat voice is if anything easier to follow than
 * an expressive one.
 *
 * Piper exposes exactly those controls. So the robot is now the clearest
 * neural voice installed, speaking with its expression turned down and its
 * pace held even, and there is no treatment on the audio at all.
 */

/*
 * MachineNoise is how much variation is left in the delivery, and MachineWidth
 * how much in the length of each sound.
 *
 * Piper's defaults are 0.667 and 0.8, which is what makes it sound like a
 * person: every sentence lands slightly differently. Near zero it lands the
 * same way every time — the same pitch contour, the same emphasis, the same
 * timing — which is the whole of what a machine voice is.
 *
 * Not zero. At exactly zero some voices develop a flat buzz on held vowels,
 * and a trace of variation costs nothing audible while avoiding it.
 */
const (
	MachineNoise = 0.12
	MachineWidth = 0.10

	/*
	 * MachinePace is how much slower than the voice's own pace it speaks.
	 *
	 * A machine that is understood is one that is not in a hurry. Five per
	 * cent is not heard as slow; it is heard as deliberate, and it is the
	 * difference between catching a file path first time and asking for it
	 * again.
	 */
	MachinePace = 1.05
)

// machineArgs are the piper settings that make a voice sound like a machine.
//
// Returned as arguments rather than applied, so that the flags a synthesiser
// was started with are part of what identifies it — a warm one set up for the
// robot cannot then be handed a sentence meant for the ordinary voice.
func machineArgs() []string {
	return []string{
		"--noise_scale", fmt.Sprintf("%.3f", MachineNoise),
		"--noise_w", fmt.Sprintf("%.3f", MachineWidth),
		"--length_scale", fmt.Sprintf("%.3f", MachinePace),
	}
}
