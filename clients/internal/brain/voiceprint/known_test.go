package voiceprint

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// a voiceprint pointing in a chosen direction, so the arithmetic around the
// model can be checked without the model.
func pointing(at int) []float32 {
	print := make([]float32, Dimensions)
	print[at] = 1

	return print
}

func brainAt(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	if err := os.MkdirAll(Folder(root), 0o755); err != nil {
		t.Fatal(err)
	}

	return root
}

/*
 * "I cannot tell" is a third answer and must never become "not you".
 *
 * A brain that treats an unanswerable question as a refusal stops listening to
 * its owner the moment a file is missing or somebody says something short —
 * which is the one failure that would make this feature worse than not having
 * it.
 */
func TestNotKnowingIsNotTheSameAsNotYou(t *testing.T) {
	root := brainAt(t)

	// Nothing taught yet.
	v := Recognise(root, make([]float64, SampleRate*2), 0.5)

	if v.Judged {
		t.Error("it judged a voice with nothing to compare against")
	}

	if v.Owner {
		t.Error("it claimed to recognise its owner having never been taught")
	}

	if v.Why == "" {
		t.Error("it gave no reason for being unable to tell")
	}
}

/*
 * A recording too short to describe a speaker is refused rather than guessed.
 *
 * Half a word is enough for a confident number and not enough for a true one.
 */
func TestTooLittleSoundIsRefused(t *testing.T) {
	_, err := Of(make([]float64, SampleRate/4))

	if err == nil {
		t.Fatal("it measured a voice from a quarter of a second")
	}

	if got := err.Error(); got == "" {
		t.Error("it refused without saying why")
	}
}

/*
 * Samples are averaged, so the print settles rather than becoming whatever was
 * said last — and the average of unit vectors is scaled back to unit length,
 * or every comparison would drift as samples accumulate.
 */
func TestTeachingAveragesAndStaysComparable(t *testing.T) {
	root := brainAt(t)

	// Two prints a little apart, combined by hand the way Learn does.
	first, second := pointing(0), make([]float32, Dimensions)
	second[0], second[1] = 0.8, 0.6

	known := &Known{Print: first, Samples: 1}

	sum := make([]float32, Dimensions)

	for i := range sum {
		sum[i] = known.Print[i]*float32(known.Samples) + second[i]
	}

	known.Samples++
	known.Print = normalise(sum)

	var length float64

	for _, v := range known.Print {
		length += float64(v) * float64(v)
	}

	if math.Abs(math.Sqrt(length)-1) > 1e-6 {
		t.Errorf("the averaged print has length %.6f, not 1", math.Sqrt(length))
	}

	// It sits between the two it was made from, nearer neither.
	toFirst, toSecond := Alike(known.Print, first), Alike(known.Print, second)

	if toFirst < 0.9 || toSecond < 0.9 {
		t.Errorf("the average is not close to both: %.2f and %.2f", toFirst, toSecond)
	}

	if err := save(root, known); err != nil {
		t.Fatal(err)
	}

	// It survives being written down and read back.
	back, err := Load(root)
	if err != nil || back == nil {
		t.Fatalf("reading it back: %v", err)
	}

	if back.Samples != 2 || Alike(back.Print, known.Print) < 0.999 {
		t.Errorf("what came back is not what was written: %+v", back.Samples)
	}
}

/*
 * Forgetting leaves nothing behind.
 *
 * This is biometric data about a person, and the only thing that makes keeping
 * it acceptable is that one press removes it.
 */
func TestForgettingLeavesNothing(t *testing.T) {
	root := brainAt(t)

	if err := save(root, &Known{Print: pointing(3), Samples: 1}); err != nil {
		t.Fatal(err)
	}

	if !Enrolled(root) {
		t.Fatal("it did not record the voice it was taught")
	}

	if err := Forget(root); err != nil {
		t.Fatal(err)
	}

	if Enrolled(root) {
		t.Error("it still knows a voice after being told to forget")
	}

	if _, err := os.Stat(filepath.Join(Folder(root), KnownFile)); !os.IsNotExist(err) {
		t.Error("the file is still on the disk")
	}

	// And forgetting twice is not an error.
	if err := Forget(root); err != nil {
		t.Errorf("forgetting nothing failed: %v", err)
	}
}

// Two prints pointing the same way are the same voice; at right angles they
// are unrelated. The comparison is the whole decision, so it is worth pinning.
func TestHowAlikeIsMeasured(t *testing.T) {
	if got := Alike(pointing(0), pointing(0)); math.Abs(got-1) > 1e-6 {
		t.Errorf("a print against itself is %.3f, not 1", got)
	}

	if got := Alike(pointing(0), pointing(1)); math.Abs(got) > 1e-6 {
		t.Errorf("two unrelated prints are %.3f, not 0", got)
	}

	if got := Alike(pointing(0), nil); got != 0 {
		t.Errorf("comparing against nothing gave %.3f", got)
	}
}
