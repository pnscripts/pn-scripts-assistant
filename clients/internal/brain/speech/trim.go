package speech

import (
	"encoding/binary"
	"os"
)

/*
 * Dropping the assistant's own voice from the front of an interrupted turn.
 *
 * When somebody talks over an answer, the recording holds both of them: the
 * assistant still speaking, then the person cutting across it. Handing all of
 * that to the recogniser asks it to transcribe two people at once, and what
 * comes back is a blend of the two — a sentence neither of them said. That is
 * the worst available outcome, because it is confident and it is wrong, and
 * nothing about it says a second voice was present.
 *
 * The detector already knows the moment it noticed, so everything before that
 * is thrown away and only what the person said is transcribed.
 */

// TrimTo drops everything before an offset, keeping the WAV header valid.
//
// A no-op when there is nothing to drop, which is the ordinary case: most
// turns are not interruptions.
func TrimTo(path string, from int64) error {
	const header = 44

	if from <= header {
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if int64(len(raw)) <= from {
		// The whole recording is before the cut, which means the interruption
		// was the last thing in it. Nothing usable, and truncating to nothing
		// would leave a file whisper refuses rather than an empty transcript.
		return nil
	}

	body := raw[from:]

	// Aligned to a whole 16-bit sample, or every sample after this point is
	// assembled from the wrong pair of bytes and the audio becomes noise.
	if len(body)%2 == 1 {
		body = body[:len(body)-1]
	}

	out := make([]byte, 0, header+len(body))
	out = append(out, raw[:header]...)
	out = append(out, body...)

	// Both lengths in the header describe the old file and have to be
	// rewritten, or the recogniser reads past the end and refuses it. Same
	// reason RepairWAV exists.
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	binary.LittleEndian.PutUint32(out[40:], uint32(len(body)))

	return os.WriteFile(path, out, 0o600)
}
