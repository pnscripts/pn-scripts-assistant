package voiceprint

import "math"

/*
 * What a voiceprint is, and how two are compared.
 *
 * In its own file because it belongs to every build: the model needs cgo and
 * is not in the portable ones, but the shape of the answer and the arithmetic
 * over it are the same everywhere, and a build that cannot measure a voice can
 * still read a print that travelled with the brain.
 */

// Where the parts live, under the brain's own folder.
const (
	ModelName   = "voiceprint.onnx"
	RuntimeName = "libonnxruntime.so"
	FolderName  = "voice"

	// Dimensions is the size of one voiceprint, fixed by the model.
	Dimensions = 512

	// LeastSeconds is the shortest recording worth judging. Below about a
	// second there is not enough voice in it to describe a speaker, and a
	// confident answer from half a word is worse than no answer.
	LeastSeconds = 1.0
)

/*
 * normalise scales a print to unit length.
 *
 * Which turns the comparison below into a plain dot product, and — more to the
 * point — removes loudness from the answer. A voice recorded close to the
 * microphone and the same voice across the room differ in length and not in
 * direction, and it is the direction that says who is talking.
 */
func normalise(print []float32) []float32 {
	var sum float64

	for _, v := range print {
		sum += float64(v) * float64(v)
	}

	length := math.Sqrt(sum)

	if length == 0 {
		return print
	}

	for i := range print {
		print[i] = float32(float64(print[i]) / length)
	}

	return print
}

/*
 * Alike is how close two voiceprints are, from -1 to 1.
 *
 * The cosine between them. One is the same recording; around zero is two
 * unrelated voices. Where the line falls between "the same person" and
 * "somebody else" is not a property of the model — it depends on the room and
 * the microphone — which is why it is a setting rather than a constant here.
 */
func Alike(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var sum float64

	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}

	return sum
}
