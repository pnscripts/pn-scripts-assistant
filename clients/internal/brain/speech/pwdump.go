package speech

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

/*
 * Reading pw-dump, which does not always hand back one document.
 *
 * It is documented as printing a JSON array of every object PipeWire knows
 * about, and usually does. Under churn — a stream starting or stopping while
 * it runs, which on this machine happens every time the brain speaks — it
 * prints more than one array, back to back, and a straight Unmarshal of the
 * whole output fails with "invalid character '[' after top-level value".
 *
 * That was the microphone list returning a 500 at random: the page could not
 * name a single input, and the fault looked exactly like a broken microphone
 * rather than a parser meeting a second array. Which is the worst kind of bug
 * this program has, because "it cannot hear me" already has five causes and
 * this one was not on the list.
 *
 * So the output is read as a stream of documents and the objects joined.
 *
 * Typed as raw messages rather than a shape, because the two callers want very
 * different fields out of it and neither wants to know that the other exists.
 */
func dumpObjects(raw []byte) ([]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	var (
		all  []json.RawMessage
		read bool
	)

	for {
		var batch []json.RawMessage

		if err := decoder.Decode(&batch); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			/*
			 * Nothing readable at all is an error; a later array that is
			 * truncated is not, and what was read is still worth having.
			 *
			 * Judged on whether a document was read rather than on how many
			 * objects came out of it, because an empty listing is a real
			 * answer — a machine playing nothing — and reporting that as a
			 * failure would have the brain claim its sound card was broken
			 * every time the room was quiet.
			 */
			if !read {
				return nil, err
			}

			break
		}

		read = true

		all = append(all, batch...)
	}

	if !read {
		return nil, errors.New("pipewire said nothing at all")
	}

	return all, nil
}
