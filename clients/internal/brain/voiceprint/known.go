package voiceprint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
 * Whose voice the brain knows.
 *
 * One person: the owner. Not a register of everybody in the house, because
 * what is being asked is a single question — is this the person I work for, or
 * is it the room — and answering it needs one voiceprint, not a directory of
 * them.
 *
 * This is biometric data about a person, so where it lives matters. It stays
 * in the brain's own folder, which is the folder the brain stops and asks
 * before reading; it never leaves the machine; it is a list of numbers from
 * which no audio can be recovered; and it can be deleted in one press, which
 * is the only property that makes the rest of it acceptable.
 */

// KnownFile is where the owner's voiceprint is kept, inside the voice folder.
const KnownFile = "known.json"

/*
 * SamplesWanted is how many recordings make a voiceprint worth trusting.
 *
 * One is a recording of a sentence, and carries how that sentence happened to
 * be said. Several, averaged, carry what is the same across them, which is the
 * speaker. Three is where it stops improving quickly.
 */
const SamplesWanted = 3

// Known is the owner's voiceprint and how it was arrived at.
type Known struct {
	// Print is the average of the samples, at unit length.
	Print []float32 `json:"print"`

	// Samples is how many recordings went into it, and When each was taken.
	Samples int         `json:"samples"`
	When    []time.Time `json:"when,omitempty"`

	/*
	 * Alone is how alike the samples were to each other.
	 *
	 * The honest measure of whether the enrolment is any good: three
	 * recordings of one person in one room should sit close together, and if
	 * they do not then something else was in the room and the print describes
	 * a blend of two people.
	 */
	Agreement float64 `json:"agreement"`
}

// Enrolled reports whether the brain has been taught a voice.
func Enrolled(root string) bool {
	known, err := Load(root)

	return err == nil && known != nil && len(known.Print) == Dimensions
}

// Load reads the voiceprint, or nothing if none has been taught.
func Load(root string) (*Known, error) {
	raw, err := os.ReadFile(filepath.Join(Folder(root), KnownFile))

	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var known Known

	if err := json.Unmarshal(raw, &known); err != nil {
		return nil, fmt.Errorf("reading the voiceprint: %w", err)
	}

	return &known, nil
}

var writing sync.Mutex

/*
 * Learn adds one recording to what the brain knows of the owner's voice.
 *
 * Averaged rather than replaced, so the print gets steadier with each sample
 * instead of becoming whatever was said last. The average of unit vectors is
 * not itself a unit vector, so it is scaled back — otherwise every comparison
 * would drift as samples accumulate.
 */
func Learn(root string, samples []float64) (*Known, error) {
	print, err := Of(samples)
	if err != nil {
		return nil, err
	}

	writing.Lock()
	defer writing.Unlock()

	known, err := Load(root)
	if err != nil {
		return nil, err
	}

	if known == nil {
		known = &Known{Print: make([]float32, Dimensions)}
	}

	/*
	 * How alike this one is to what is already known.
	 *
	 * Reported rather than enforced: somebody enrolling in a noisy room
	 * deserves to be told the samples disagree, not to have the third one
	 * silently refused.
	 */
	if known.Samples > 0 {
		known.Agreement = (known.Agreement*float64(known.Samples-1) +
			Alike(known.Print, print)) / float64(known.Samples)
	}

	sum := make([]float32, Dimensions)

	for i := range sum {
		sum[i] = known.Print[i]*float32(known.Samples) + print[i]
	}

	known.Samples++
	known.When = append(known.When, time.Now().UTC())
	known.Print = normalise(sum)

	return known, save(root, known)
}

// Forget removes the voiceprint. One press, and nothing about the owner's
// voice is left on the machine.
func Forget(root string) error {
	err := os.Remove(filepath.Join(Folder(root), KnownFile))

	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func save(root string, known *Known) error {
	if err := os.MkdirAll(Folder(root), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return err
	}

	// Readable only by its owner: it describes their voice.
	return os.WriteFile(filepath.Join(Folder(root), KnownFile), raw, 0o600)
}

/*
 * Recognises says whether a recording is the owner, and how sure it is.
 *
 * Three answers rather than two. Yes and no are obvious; the third is "there
 * is no voiceprint, or the recording is too short to judge", and it must not
 * be collapsed into either of them — a brain that treats "cannot tell" as
 * "not you" stops listening to its owner the moment the model is missing.
 */
type Verdict struct {
	// Judged is false when there was nothing to compare, or too little sound.
	Judged bool

	// Owner is whether it was the person the brain was taught.
	Owner bool

	// Alike is the cosine, kept for the interface: a number somebody can watch
	// while deciding where to put the line.
	Alike float64

	// Why explains a Judged of false, in words.
	Why string
}

/*
 * Recognise compares a recording against the enrolled voice.
 *
 * atLeast is where the line falls between the owner and somebody else. It is a
 * setting rather than a constant because it depends on the room and the
 * microphone: measured here, two recordings of one voice sit between 0.68 and
 * 0.91 and two different voices between -0.06 and 0.36, so halfway is a wide
 * margin on both sides — but a different room narrows it.
 */
func Recognise(root string, samples []float64, atLeast float64) Verdict {
	known, err := Load(root)

	if err != nil || known == nil || len(known.Print) != Dimensions {
		return Verdict{Why: "no voice has been taught to this brain yet"}
	}

	print, err := Of(samples)
	if err != nil {
		return Verdict{Why: err.Error()}
	}

	alike := Alike(known.Print, print)

	return Verdict{Judged: true, Owner: alike >= atLeast, Alike: alike}
}
