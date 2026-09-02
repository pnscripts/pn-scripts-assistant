/*
 * Package copies keeps the brain in more than one place.
 *
 * The brain lives on a drive that gets carried around, and a drive that gets
 * carried around is a drive that gets dropped, lost, or plugged into a machine
 * that decides to reformat it. Everything it has ever learned is one object on
 * one disk, and until this existed there was no answer to that beyond hoping.
 *
 * One rule holds the whole design up: a copy is a copy until somebody chooses
 * it. Only one place is ever the brain — the one with the .brain-root.json
 * marker in it — and copies are written with a different marker so nothing
 * that goes looking for the brain can find one and start using it by accident.
 * That single rule removes the entire problem this kind of feature normally
 * has: two places that both think they are the real one, quietly growing apart
 * on different machines, with no honest way to put them back together.
 *
 * So there is no merging here, and no clever resolution of who is newer,
 * because there is never anything to resolve. Copies only ever flow outward
 * from the brain. Making one of them the brain is a thing a person does, on
 * purpose, having been shown what is in it and how old it is.
 */
package copies

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListFile records where the copies are kept. It lives in the brain's own
// folder, so the list travels with the brain rather than with the machine.
const ListFile = "copies.json"

/*
 * Marker is what identifies a folder as a copy — deliberately not the marker
 * that identifies the brain itself.
 *
 * This is the safety catch. paths.Find goes looking for .brain-root.json in
 * every mounted drive, and if a copy carried that file, plugging in the backup
 * drive on a machine whose own drive was not attached would silently start the
 * brain from a copy — which then accepts a day of conversation that the real
 * one will never have, and now there genuinely are two brains.
 */
const Marker = ".brain-copy.json"

// Copy is one place a copy of the brain is kept, and what is in it.
type Copy struct {
	Path string `json:"path"`

	// From the copy's own marker: what was written, and when.
	At    time.Time `json:"at"`
	Bytes int64     `json:"bytes"`
	Facts int       `json:"facts"`

	// BrainID is the identity of the brain this was copied from, so a copy of
	// a different brain is recognisable as one.
	BrainID string `json:"brain_id"`

	// Reachable is whether the folder is there right now. A copy on a drive
	// that is not plugged in is the ordinary case, not a fault.
	Reachable bool `json:"reachable"`

	// Trouble is why this copy cannot be written to, in words meant for the
	// person who has to fix it.
	Trouble string `json:"trouble"`
}

// Never is what a copy that has never been written reports as its age.
func (c Copy) Never() bool { return c.At.IsZero() }

/*
 * Places lists the folders that are meant to hold a copy.
 *
 * The list is what somebody asked for; whether each one is reachable is a
 * separate question answered by Status. Keeping those apart matters: a copy on
 * an unplugged drive must still appear, or the interface forgets the backup
 * exists the moment it is not attached, which is exactly when somebody goes
 * looking for it.
 */
func Places(root string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, ListFile))

	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var places []string

	if err := json.Unmarshal(raw, &places); err != nil {
		return nil, fmt.Errorf("reading the list of copies: %w", err)
	}

	return places, nil
}

func savePlaces(root string, places []string) error {
	sort.Strings(places)

	raw, err := json.MarshalIndent(places, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, ListFile), raw, 0o644)
}

/*
 * Keep adds a folder to the list of places a copy is kept.
 *
 * Everything refused here is refused because it would produce a copy that is
 * not a copy of anything useful.
 */
func Keep(root, dir string) error {
	dir = strings.TrimSpace(dir)

	if dir == "" {
		return errors.New("nowhere was given")
	}

	if !filepath.IsAbs(dir) {
		return errors.New("that has to be a full path, starting from the top of the disk")
	}

	dir = filepath.Clean(dir)

	if err := checkDestination(root, dir); err != nil {
		return err
	}

	places, err := Places(root)
	if err != nil {
		return err
	}

	for _, p := range places {
		if p == dir {
			return nil
		}
	}

	return savePlaces(root, append(places, dir))
}

/*
 * checkDestination is every way a folder is the wrong place for a copy.
 *
 * Separate from Keep so the same answers are given when a copy is written, not
 * only when it is added: a folder that was fine last week can be somebody
 * else's brain today.
 */
func checkDestination(root, dir string) error {
	if root != "" {
		clean := filepath.Clean(root)

		if dir == clean {
			return errors.New("that is where the brain already is")
		}

		/*
		 * And not inside it, in either direction.
		 *
		 * A copy inside the brain's own folder is not a backup of anything —
		 * whatever destroys the original takes it too — and it grows without
		 * end, because each copy then includes the copy before it. A folder
		 * that contains the brain is the same fault the other way round.
		 */
		if within(dir, clean) || within(clean, dir) {
			return errors.New("a copy cannot live inside the brain's own folder, or hold it")
		}
	}

	/*
	 * And never on top of another brain.
	 *
	 * A folder carrying .brain-root.json is a working brain, possibly the only
	 * one somebody has. Writing a copy over it would replace everything it
	 * knows with everything this one knows, and nothing about the request
	 * "keep a copy here" says that is wanted.
	 */
	if _, err := os.Stat(filepath.Join(dir, ".brain-root.json")); err == nil {
		return errors.New("there is already a brain in that folder — pick somewhere else, or move that one first")
	}

	return nil
}

// within reports whether path is inside base.
func within(path, base string) bool {
	rel, err := filepath.Rel(base, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Stop takes a folder off the list.
//
// The files are left exactly where they are. Somebody who no longer wants the
// brain copied to a drive has said nothing about wanting the copy on it
// destroyed, and that copy may be the only other one that exists.
func Stop(root, dir string) error {
	places, err := Places(root)
	if err != nil {
		return err
	}

	kept := places[:0]

	for _, p := range places {
		if p != filepath.Clean(dir) {
			kept = append(kept, p)
		}
	}

	return savePlaces(root, kept)
}

/*
 * Status is every place on the list, with what is actually in it.
 *
 * Read from each copy's own marker rather than from a record kept here,
 * because the copy is the thing being described and it may have been written
 * by a different machine, or replaced, or emptied by somebody tidying up a
 * drive. What the folder says about itself is the only account worth showing.
 */
func Status(root string) ([]Copy, error) {
	places, err := Places(root)
	if err != nil {
		return nil, err
	}

	out := make([]Copy, 0, len(places))

	for _, place := range places {
		out = append(out, describe(place))
	}

	return out, nil
}

func describe(dir string) Copy {
	c := Copy{Path: dir}

	info, err := os.Stat(dir)

	switch {
	case err == nil && !info.IsDir():
		c.Trouble = "that is a file, not a folder"

		return c

	case errors.Is(err, os.ErrNotExist):
		/*
		 * Missing means one of two opposite things, and they must not be shown
		 * the same way.
		 *
		 * A folder that is not there because the drive is not plugged in is
		 * the ordinary state of a backup drive and nothing is wrong. A folder
		 * that is not there on a drive that is attached has simply not been
		 * made yet, and the next copy will make it. Telling somebody to plug
		 * in a drive that is already plugged in sends them to look for a fault
		 * that does not exist.
		 */
		if _, up := os.Stat(filepath.Dir(dir)); up == nil {
			c.Reachable = true
			c.Trouble = "nothing has been copied here yet"

			return c
		}

		c.Trouble = "not there right now — the drive is probably not plugged in"

		return c

	case err != nil:
		c.Trouble = err.Error()

		return c
	}

	c.Reachable = true

	raw, err := os.ReadFile(filepath.Join(dir, Marker))
	if err != nil {
		// Reachable but never written to, which is what a copy looks like
		// between being added and the first copy finishing.
		return c
	}

	var noted struct {
		At      time.Time `json:"at"`
		Bytes   int64     `json:"bytes"`
		Facts   int       `json:"facts"`
		BrainID string    `json:"brain_id"`
	}

	if err := json.Unmarshal(raw, &noted); err != nil {
		c.Trouble = "there is a copy here but its record cannot be read"

		return c
	}

	c.At = noted.At
	c.Bytes = noted.Bytes
	c.Facts = noted.Facts
	c.BrainID = noted.BrainID

	return c
}
