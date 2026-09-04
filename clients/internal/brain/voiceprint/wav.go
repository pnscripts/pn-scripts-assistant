package voiceprint

import (
	"encoding/binary"
	"fmt"
	"os"
)

/*
 * Reading a recording back off the disk.
 *
 * The turns are written by pw-record as sixteen-bit mono at sixteen kilohertz,
 * which is exactly what the model wants — so this only has to find the audio
 * inside the file rather than convert anything.
 */

// FromFile reads the samples out of a 16-bit mono WAV.
func FromFile(path string) ([]float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(raw) < 44 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, fmt.Errorf("%s is not a WAV file", path)
	}

	/*
	 * Walked chunk by chunk rather than assuming the audio starts at byte 44.
	 *
	 * That is true of the simplest possible file and not of one written while
	 * recording: a writer that does not know the length in advance leaves a
	 * LIST chunk or an extended format block in front of the data, and
	 * everything read from a fixed offset is then noise.
	 */
	at := 12

	for at+8 <= len(raw) {
		id := string(raw[at : at+4])
		size := int(binary.LittleEndian.Uint32(raw[at+4 : at+8]))

		if id == "data" {
			body := raw[at+8:]

			if size >= 0 && size < len(body) {
				body = body[:size]
			}

			out := make([]float64, len(body)/2)

			for i := range out {
				out[i] = float64(int16(binary.LittleEndian.Uint16(body[i*2:])))
			}

			return out, nil
		}

		// Chunks are padded to an even length, and the pad byte is not counted
		// in the size — missing it walks into the middle of the next header.
		at += 8 + size + size%2
	}

	return nil, fmt.Errorf("%s has no audio in it", path)
}

/*
 * Judge reads a recording and says whose voice it is.
 *
 * The whole question in one call, because every caller wants the same three
 * things and none of them wants to know about filterbanks.
 */
func Judge(root, path string, atLeast float64) Verdict {
	samples, err := FromFile(path)
	if err != nil {
		return Verdict{Why: err.Error()}
	}

	return Recognise(root, samples, atLeast)
}
