/*
 * Package undo keeps the previous version of anything the brain overwrites.
 *
 * "Put it back" was the one word in the interruption vocabulary that meant
 * nothing. The brain could be stopped mid-answer, its last exchange could be
 * forgotten and a whole conversation thrown away — but a file it had written
 * over was simply gone, because nothing had taken a copy first. There was
 * nothing to put back.
 *
 * So a copy is taken before every write, which costs a file read and a file
 * write against an action that was already approved and is already touching
 * the disk. The copies live in the brain's own folder rather than beside the
 * original: a hidden .bak next to somebody's source file is litter in their
 * project, turns up in their editor and their git status, and is one more
 * thing for them to clean up after an assistant.
 *
 * Bounded on purpose. This is a way back from the last hour of a conversation,
 * not a version control system — a program that quietly grows a second copy of
 * every file it has ever touched is a program that fills a disk.
 */
package undo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// FolderName is where the copies live, inside the brain's own folder.
	FolderName = "changes"

	// IndexName lists them, so what is kept is readable without opening every
	// file to find out what it was.
	IndexName = "changes.json"

	// HowMany is how many changes are kept. Enough to cover a session's work,
	// not enough to become a filing system.
	HowMany = 60

	// HowLong is how long one is worth keeping. Past this, whatever was
	// written is either right or has been dealt with some other way.
	HowLong = 14 * 24 * time.Hour
)

// Change is one thing that was overwritten, and where the previous version
// went.
type Change struct {
	ID string `json:"id"`

	// Path is the file that was changed.
	Path string `json:"path"`

	At time.Time `json:"at"`

	// What was done, in the words the person would use: written, edited.
	What string `json:"what"`

	// Kept is the copy inside the brain's folder, or empty when the file did
	// not exist before — which is a change that is undone by deleting it.
	Kept string `json:"kept,omitempty"`

	// Bytes is how big the previous version was, for saying what putting it
	// back would restore.
	Bytes int64 `json:"bytes"`

	// New marks a file the brain created, so putting it back means removing
	// it rather than restoring something.
	New bool `json:"new,omitempty"`
}

var writing sync.Mutex

/*
 * Keep copies what is there now, before it is written over.
 *
 * Called with the path about to change. A file that does not exist yet is
 * recorded too, as a change whose undo is deletion — otherwise "put it back"
 * would silently do nothing about the file the brain had just created, which
 * is the most likely thing anybody wants put back.
 *
 * Best effort by design: failing to keep a copy must not stop the write. The
 * cost of that is being unable to undo one change; the cost of the other way
 * round is a tool that refuses to work because its safety net is full.
 */
func Keep(root, path, what string) {
	if root == "" || path == "" {
		return
	}

	writing.Lock()
	defer writing.Unlock()

	change := Change{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Path: path,
		At:   time.Now().UTC(),
		What: what,
	}

	body, err := os.ReadFile(path)

	switch {
	case errors.Is(err, os.ErrNotExist):
		change.New = true

	case err != nil:
		return

	default:
		change.Bytes = int64(len(body))

		folder := filepath.Join(root, FolderName)

		if err := os.MkdirAll(folder, 0o755); err != nil {
			return
		}

		kept := filepath.Join(folder, change.ID)

		// The mode of the original, since a copy of a private key readable by
		// everybody is a worse outcome than no copy.
		mode := os.FileMode(0o600)

		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}

		if err := os.WriteFile(kept, body, mode); err != nil {
			return
		}

		change.Kept = kept
	}

	list := append(load(root), change)

	save(root, tidy(root, list))
}

// List is what can be put back, newest first.
func List(root string) []Change {
	writing.Lock()
	defer writing.Unlock()

	list := load(root)

	sort.Slice(list, func(i, j int) bool { return list[i].At.After(list[j].At) })

	return list
}

/*
 * PutBack restores one change.
 *
 * The current contents are kept first, so putting something back is itself
 * undoable — somebody who restores the wrong version has not lost the work
 * they were restoring over, which is the difference between an undo and a
 * second mistake.
 */
func PutBack(root, id string) (Change, error) {
	for _, change := range List(root) {
		if id != "" && change.ID != id {
			continue
		}

		Keep(root, change.Path, "put back")

		if change.New {
			if err := os.Remove(change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return change, fmt.Errorf("removing %s: %w", change.Path, err)
			}

			return change, nil
		}

		body, err := os.ReadFile(change.Kept)
		if err != nil {
			return change, fmt.Errorf("the kept copy of %s cannot be read: %w", change.Path, err)
		}

		mode := os.FileMode(0o644)

		if info, err := os.Stat(change.Kept); err == nil {
			mode = info.Mode().Perm()
		}

		if err := os.WriteFile(change.Path, body, mode); err != nil {
			return change, fmt.Errorf("putting %s back: %w", change.Path, err)
		}

		return change, nil
	}

	return Change{}, errors.New("there is nothing recorded to put back")
}

// Latest is the most recent change to a particular file, or to anything.
func Latest(root, path string) (Change, bool) {
	for _, change := range List(root) {
		if path == "" || strings.EqualFold(change.Path, path) {
			return change, true
		}
	}

	return Change{}, false
}

func load(root string) []Change {
	raw, err := os.ReadFile(filepath.Join(root, IndexName))
	if err != nil {
		return nil
	}

	var list []Change

	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}

	return list
}

func save(root string, list []Change) {
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(filepath.Join(root, IndexName), raw, 0o644)
}

/*
 * tidy drops what is too old or too much, and deletes the copies with it.
 *
 * Both halves matter: forgetting the record while leaving the file behind
 * would grow a folder of copies nothing refers to, which is the same disk
 * filling up with the record removed.
 */
func tidy(root string, list []Change) []Change {
	sort.Slice(list, func(i, j int) bool { return list[i].At.After(list[j].At) })

	kept := make([]Change, 0, len(list))

	for i, change := range list {
		tooOld := time.Since(change.At) > HowLong
		tooMany := i >= HowMany

		if tooOld || tooMany {
			if change.Kept != "" {
				os.Remove(change.Kept)
			}

			continue
		}

		kept = append(kept, change)
	}

	return kept
}
