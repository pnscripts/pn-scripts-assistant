package orchestrator

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

/*
 * A folder as it stood: every file's size and time, so what an agent changed
 * can be read from the folder itself afterwards rather than from what the
 * agent says. Its own events say what it meant to write; the folder says
 * what happened, for every agent alike, and also catches a change made by a
 * command it ran.
 */

// SnapshotMost is how many files a snapshot reads before it stops: enough
// for a project, not a home folder.
const SnapshotMost = 20000

// Snapshot is a folder's files by path.
type Snapshot struct {
	Files map[string]fileState

	// Partial is whether the folder had more files than SnapshotMost.
	Partial bool
}

type fileState struct {
	size int64
	mod  time.Time
}

// skipped are the folders whose contents are not anybody's to read here:
// version control's own and installed packages.
var skipped = map[string]bool{".git": true, "node_modules": true, ".venv": true, "__pycache__": true}

// Snap reads dir as it stands now.
func Snap(dir string) Snapshot {
	s := Snapshot{Files: map[string]fileState{}}

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path != dir && skipped[d.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if len(s.Files) >= SnapshotMost {
			s.Partial = true

			return filepath.SkipAll
		}

		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}

		s.Files[path] = fileState{size: info.Size(), mod: info.ModTime()}

		return nil
	})

	return s
}

// Change is one file an agent added, changed or removed.
type Change struct {
	Path string `json:"path"`
	What string `json:"what"` // added, changed, removed
}

// Changes is what differs between two snapshots of one folder, by path.
func Changes(before, after Snapshot) []Change {
	var out []Change

	for path, now := range after.Files {
		was, ok := before.Files[path]

		switch {
		case !ok:
			out = append(out, Change{Path: path, What: "added"})
		case was.size != now.size || !was.mod.Equal(now.mod):
			out = append(out, Change{Path: path, What: "changed"})
		}
	}

	for path := range before.Files {
		if _, ok := after.Files[path]; !ok {
			// A file only missing because the second read stopped early is
			// not a file that was removed.
			if after.Partial {
				if _, err := os.Lstat(path); err == nil {
					continue
				}
			}

			out = append(out, Change{Path: path, What: "removed"})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out
}
