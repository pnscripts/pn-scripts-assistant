// Package paths finds where the brain keeps its data.
//
// The rule that shaped this: nothing may hardcode a drive, a mount point or a
// machine. The brain lives on an external disk that can be unplugged and
// reattached at a different path, moved to another computer, or replaced
// entirely — and it has already survived three project renames. So the data
// root is found at run time by looking for a marker file, never by remembering
// a path someone typed once.
package paths

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Marker is the file that identifies a directory as a brain data root.
const Marker = ".brain-root.json"

// Root describes a located data root.
type Root struct {
	Path    string `json:"-"`
	ID      string `json:"id"`
	Schema  int    `json:"schema"`
	Created string `json:"created"`
}

// DatabasePath is where the SQLite file lives inside a root.
func (r Root) DatabasePath() string { return filepath.Join(r.Path, "brain.sqlite") }

// SearchPaths are the places a drive normally appears, in the order they are
// tried. PN_BRAIN_DATA_ROOT overrides all of it for anyone who wants to be
// explicit, which includes the tests.
func SearchPaths() []string {
	var out []string

	if extra := os.Getenv("PN_BRAIN_SEARCH_PATHS"); extra != "" {
		out = append(out, filepath.SplitList(extra)...)
	}

	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".local", "share", "pn-brain"))
	}

	/*
	 * And wherever this system puts a drive somebody has plugged in.
	 *
	 * The whole point of keeping the brain on an external drive is carrying it
	 * to another computer, and that computer does not have to be running the
	 * same system as this one. Searching only the Linux mount points meant the
	 * drive was invisible on macOS and Windows — where the program would not
	 * report a problem but quietly start a brand new empty brain in the home
	 * folder, with everything that had been learned sitting unread on the disk
	 * plugged into it.
	 */
	out = append(out, removableMounts()...)

	return out
}

/*
 * removableMounts is where each system shows a drive that was plugged in.
 *
 * By convention rather than by asking the system, deliberately: this runs at
 * startup before anything else works, and a glob that finds nothing is a much
 * better failure than a mount-table parser that cannot run.
 */
func removableMounts() []string {
	var out []string

	switch runtime.GOOS {
	case "darwin":
		// Everything except the boot disk appears here, including network
		// shares and disk images.
		matches, _ := filepath.Glob("/Volumes/*")
		out = append(out, matches...)

	case "windows":
		/*
		 * Every letter that answers, which is the only way to ask.
		 *
		 * A drive letter is assigned when the disk is plugged in and is not
		 * predictable — the same stick is D: on one machine and F: on the
		 * next. Twenty-four Stat calls at startup is nothing, and it means the
		 * drive is found wherever it landed.
		 */
		for letter := 'C'; letter <= 'Z'; letter++ {
			root := string(letter) + `:\`

			if _, err := os.Stat(root); err == nil {
				out = append(out, root)
			}
		}

	default:
		// Linux desktops mount removable drives under /media/<user>/<label>
		// and /run/media/<user>/<label>; /mnt is the manual convention. The
		// user is globbed rather than assumed, so a drive written on one
		// machine is still found under somebody else's name on another.
		for _, base := range []string{"/media", "/run/media", "/mnt"} {
			matches, _ := filepath.Glob(filepath.Join(base, "*", "*"))
			out = append(out, matches...)

			direct, _ := filepath.Glob(filepath.Join(base, "*"))
			out = append(out, direct...)
		}
	}

	return out
}

// Find locates an existing data root, or reports that there is none.
func Find() (Root, error) {
	if explicit := os.Getenv("PN_BRAIN_DATA_ROOT"); explicit != "" {
		return read(explicit)
	}

	for _, dir := range SearchPaths() {
		// A data root is either the directory itself or a PN-BRAIN-DATA inside
		// it, because a drive holds other things too.
		for _, candidate := range []string{dir, filepath.Join(dir, "PN-BRAIN-DATA")} {
			if _, err := os.Stat(filepath.Join(candidate, Marker)); err == nil {
				return read(candidate)
			}
		}
	}

	return Root{}, ErrNotFound
}

// ErrNotFound means no data root exists yet; the caller decides whether to
// create one, which is a decision that usually belongs to a person.
var ErrNotFound = errors.New("no brain data root found")

func read(dir string) (Root, error) {
	raw, err := os.ReadFile(filepath.Join(dir, Marker))
	if err != nil {
		return Root{}, fmt.Errorf("reading marker in %s: %w", dir, err)
	}

	var r Root

	if err := json.Unmarshal(raw, &r); err != nil {
		return Root{}, fmt.Errorf("parsing marker in %s: %w", dir, err)
	}

	r.Path = dir

	return r, nil
}

// Create marks dir as a data root, creating it if needed.
func Create(dir string) (Root, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Root{}, fmt.Errorf("creating %s: %w", dir, err)
	}

	if existing, err := read(dir); err == nil {
		return existing, nil
	}

	r := Root{
		Path:    dir,
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Schema:  1,
		Created: time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Root{}, err
	}

	if err := os.WriteFile(filepath.Join(dir, Marker), raw, 0o644); err != nil {
		return Root{}, fmt.Errorf("writing marker: %w", err)
	}

	return r, nil
}

// FindOrCreate locates a root, falling back to the per-user directory.
//
// The fallback is deliberately the user's own home rather than a prompt: a
// brain that refuses to start because no drive is attached is worse than one
// that starts small and can be moved later.
func FindOrCreate() (Root, error) {
	if r, err := Find(); err == nil {
		return r, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Root{}, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Root{}, err
	}

	return Create(filepath.Join(home, ".local", "share", "pn-brain"))
}
