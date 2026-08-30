package speech

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

/*
 * Repairing the header of a recording that was stopped rather than finished.
 *
 * A WAV file says how long it is twice, in the RIFF header and again in the
 * data chunk, and neither can be written until the recording ends — so a
 * recorder writes placeholders and goes back to fix them when it closes.
 *
 * This program never lets it close. A turn ends when somebody stops talking,
 * which is decided here, and the recorder is killed on the spot. The audio is
 * all present and the two lengths still say the file is sixteen bytes long.
 *
 * Nothing in this program noticed, because everything here reads the samples
 * directly and ignores those numbers. whisper does not ignore them: it reads
 * the header, concludes the file holds nothing, and answers "Invalid request"
 * — which arrived as "loud enough, but no words came back", for an entire
 * afternoon, on thirteen recordings out of fourteen.
 */

// RepairWAV makes the lengths in a recording's header match the file.
//
// Reports whether anything needed fixing.
func RepairWAV(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	const smallest = 44

	if len(raw) < smallest {
		return false, fmt.Errorf("%s is too short to be a recording", path)
	}

	if !bytes.Equal(raw[0:4], []byte("RIFF")) || !bytes.Equal(raw[8:12], []byte("WAVE")) {
		return false, fmt.Errorf("%s is not a WAV file", path)
	}

	changed := false

	// The RIFF length covers everything after the first eight bytes.
	if want := uint32(len(raw) - 8); binary.LittleEndian.Uint32(raw[4:]) != want {
		binary.LittleEndian.PutUint32(raw[4:], want)

		changed = true
	}

	/*
	 * And the data chunk's own length.
	 *
	 * Found by walking the chunks rather than assuming it begins at byte 36.
	 * A recorder is free to put a LIST or a fact chunk before it, and pw-record
	 * does not today — but a header repaired at the wrong offset writes a
	 * length into the middle of the audio, which is a worse fault than the one
	 * being fixed and would be blamed on the microphone.
	 */
	for at := 12; at+8 <= len(raw); {
		size := binary.LittleEndian.Uint32(raw[at+4:])

		if bytes.Equal(raw[at:at+4], []byte("data")) {
			if want := uint32(len(raw) - (at + 8)); size != want {
				binary.LittleEndian.PutUint32(raw[at+4:], want)

				changed = true
			}

			break
		}

		// Chunks are padded to an even length.
		next := at + 8 + int(size)
		if size%2 == 1 {
			next++
		}

		if next <= at {
			break
		}

		at = next
	}

	if !changed {
		return false, nil
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return false, fmt.Errorf("writing the repaired header: %w", err)
	}

	return true, nil
}
