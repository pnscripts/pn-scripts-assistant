/*
 * Package places is the drives and folders the brain looks after.
 *
 * Learning used to be something you asked for, once, about one folder. That
 * works for one folder and falls apart at the scale this is actually used at:
 * work on an external drive, films on another, documents in a third, each
 * plugged in and unplugged through the week. Asking again after every change
 * is not something anybody does, so what the brain knew was whatever it had
 * been told months ago.
 *
 * A place is somewhere it looks after. When that place is attached and holds
 * something it has not seen, it learns it — a bounded amount at a time, in the
 * background, resuming where it stopped. When the place is not attached,
 * nothing happens and nothing is wrong: a drive in a drawer is the ordinary
 * state of a drive.
 *
 * Only new things cost anything. Everything already seen is filtered out
 * before any of the expensive work, because on this hardware each observation
 * is two to three seconds of embedding — so re-reading a drive of two thousand
 * documents to discover that nothing changed would be two hours of work for
 * no answer.
 */
package places

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

// ListFile records the places. It lives in the brain's own folder, so the list
// travels with the brain rather than with the machine.
const ListFile = "places.json"

// Kinds are what a place can be read for.
const (
	Projects  = "projects"
	Documents = "documents"
	Both      = "everything"
)

// Place is somewhere the brain watches, and what it has made of it.
type Place struct {
	Path string `json:"path"`

	// Name is what its owner calls it — "work", "the film drive" — because a
	// mount point is not something anybody recognises. Defaults to the folder's
	// own name.
	Name string `json:"name"`

	// Kind is projects, documents or everything.
	Kind string `json:"kind"`

	Added time.Time `json:"added"`

	// LastSeen is when the place was last reachable, which for a removable
	// drive is the answer to "when did I last plug this in".
	LastSeen time.Time `json:"last_seen,omitempty"`

	// LastLearned is when something new was last taken from it, and Learned is
	// how much has been taken in total.
	LastLearned time.Time `json:"last_learned,omitempty"`
	Learned     int       `json:"learned"`

	// Waiting is how much of this place is not yet looked at, as of the last
	// time it was counted. The number somebody actually wants: not how big the
	// drive is, but how much of it the brain has still to get through.
	Waiting int `json:"waiting"`

	// Reachable and Trouble describe right now, and are not stored.
	Reachable bool   `json:"reachable"`
	Trouble   string `json:"trouble,omitempty"`
}

// Never reports a place nothing has ever been learned from.
func (p Place) Never() bool { return p.LastLearned.IsZero() }

// List is the places as they were written down, without asking the disk
// anything. Whether each is attached is a separate question — see Status.
func List(root string) ([]Place, error) {
	raw, err := os.ReadFile(filepath.Join(root, ListFile))

	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var out []Place

	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("reading the list of places: %w", err)
	}

	return out, nil
}

func save(root string, list []Place) error {
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, ListFile), raw, 0o644)
}

/*
 * Watch adds a folder or a drive to the places the brain looks after.
 *
 * The path is checked for being a folder, but not for being reachable: adding
 * a drive that is not plugged in at this moment is a reasonable thing to do,
 * and refusing it would mean the list can only be built while every drive is
 * attached at once.
 */
func Watch(root, path, name, kind string) (Place, error) {
	path = strings.TrimSpace(path)

	if path == "" {
		return Place{}, errors.New("nowhere was given")
	}

	if !filepath.IsAbs(path) {
		return Place{}, errors.New("that has to be a full path, starting from the top of the disk")
	}

	path = filepath.Clean(path)

	if err := sensible(root, path); err != nil {
		return Place{}, err
	}

	list, err := List(root)
	if err != nil {
		return Place{}, err
	}

	for _, p := range list {
		if p.Path == path {
			return p, nil
		}
	}

	switch kind {
	case Projects, Documents, Both:
	default:
		kind = Both
	}

	if strings.TrimSpace(name) == "" {
		name = filepath.Base(path)
	}

	place := Place{Path: path, Name: name, Kind: kind, Added: time.Now().UTC()}

	if err := save(root, append(list, place)); err != nil {
		return Place{}, err
	}

	return place, nil
}

/*
 * sensible refuses the places that would be a mistake rather than a choice.
 *
 * The brain's own folder above all: everything in it is a description of what
 * the brain knows, so reading it back in would be the brain learning its own
 * memory — every pass producing new observations about the observations of the
 * pass before it.
 */
func sensible(root, path string) error {
	if root == "" {
		return nil
	}

	clean := filepath.Clean(root)

	if path == clean || within(clean, path) || within(path, clean) {
		return errors.New("that is the brain's own folder — it cannot learn from where it keeps itself")
	}

	if path == "/" {
		return errors.New("the whole disk is too much to look after; pick the folders that matter")
	}

	if home, err := os.UserHomeDir(); err == nil && path == filepath.Clean(home) {
		return errors.New("the whole home folder is too much; pick the folders inside it that matter")
	}

	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return errors.New("that is a file, not a folder")
	}

	return nil
}

func within(path, base string) bool {
	rel, err := filepath.Rel(base, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Forget stops looking after a place. Nothing is deleted — not the folder, and
// not what was learned from it, which is knowledge like any other.
func Forget(root, path string) error {
	list, err := List(root)
	if err != nil {
		return err
	}

	path = filepath.Clean(path)
	kept := make([]Place, 0, len(list))

	for _, p := range list {
		if p.Path != path {
			kept = append(kept, p)
		}
	}

	return save(root, kept)
}

// Rename changes what a place is called, which is the only thing about it that
// belongs to a person rather than to the disk.
func Rename(root, path, name string) error {
	list, err := List(root)
	if err != nil {
		return err
	}

	name = strings.TrimSpace(name)
	path = filepath.Clean(path)

	for i := range list {
		if list[i].Path == path {
			if name == "" {
				name = filepath.Base(path)
			}

			list[i].Name = name

			return save(root, list)
		}
	}

	return fmt.Errorf("%s is not one of the places", path)
}

// Status is every place with whether it is there right now.
func Status(root string) ([]Place, error) {
	list, err := List(root)
	if err != nil {
		return nil, err
	}

	for i := range list {
		list[i] = describe(list[i])
	}

	return list, nil
}

func describe(p Place) Place {
	info, err := os.Stat(p.Path)

	switch {
	case err == nil && info.IsDir():
		p.Reachable = true
		p.LastSeen = time.Now().UTC()

	case err == nil:
		p.Trouble = "that is a file, not a folder"

	case errors.Is(err, os.ErrNotExist):
		// The ordinary state of a drive that lives in a drawer, said as such
		// rather than as a fault.
		p.Trouble = "not attached right now"

	default:
		p.Trouble = err.Error()
	}

	return p
}

// Note writes back what a pass over a place found. Kept here so the file is
// only written in one place, whatever asked for the pass.
func Note(root string, done Place) error {
	list, err := List(root)
	if err != nil {
		return err
	}

	for i := range list {
		if list[i].Path != done.Path {
			continue
		}

		list[i].LastSeen = done.LastSeen
		list[i].LastLearned = done.LastLearned
		list[i].Learned = done.Learned
		list[i].Waiting = done.Waiting

		return save(root, list)
	}

	return nil
}

// Short is a path with the home folder written the way people say it.
func Short(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home) {
		return path
	}

	return "~" + strings.TrimPrefix(path, home)
}
