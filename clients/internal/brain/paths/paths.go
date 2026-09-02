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

	// Borrowed marks a root that was named by PN_BRAIN_DATA_ROOT for this run
	// only. It must never become what this machine remembers. See Find.
	Borrowed bool `json:"-"`

	/*
	 * MovedFrom is where this same brain was the last time it was opened,
	 * when that is somewhere else.
	 *
	 * The whole point of keeping the brain on a drive is carrying it, and the
	 * drive lands at a different path on every machine — /media/you/LABEL on
	 * one, /Volumes/LABEL on another, E:\ on a third. Everything the brain has
	 * learned about files records where they were, so after the journey those
	 * memories are accurate and useless at the same time: they name real files
	 * by a path that does not exist here.
	 *
	 * Nothing is done about it automatically. It is reported, because rewriting
	 * a thousand memories is not something to do to somebody without asking.
	 */
	MovedFrom string `json:"-"`
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
	out = append(out, mounts()...)

	return out
}

/*
 * mounts is removableMounts, replaceable so a test can describe a machine other
 * than the one it is running on.
 *
 * Without this a test about a drive being absent finds the real drive in this
 * machine's own /media and passes or fails for reasons that have nothing to do
 * with what it is checking.
 */
var mounts = removableMounts

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
		r, err := read(explicit)
		if err != nil {
			return Root{}, err
		}

		/*
		 * Borrowed for this run, and not written down.
		 *
		 * Marked so nothing further up records it as the brain this machine
		 * uses. The variable exists for tests and for anybody who wants to
		 * open a brain once, and both of those are temporary by definition —
		 * but FindOrCreate remembers whatever it found, so one run with the
		 * variable set made a scratch folder the permanent answer for the
		 * whole machine.
		 *
		 * That is not a hypothetical. It happened here, and the symptom is
		 * about as bad as symptoms get: the program opened afterwards with an
		 * empty brain, asking for a name as though it had never been run,
		 * while a thousand facts sat unread on the drive it was ignoring.
		 * Nothing was damaged and nothing said anything was wrong.
		 */
		r.Borrowed = true

		return r, nil
	}

	/*
	 * What was chosen beats what is nearest.
	 *
	 * The search looks in the home folder before any drive, so on a machine
	 * that had ever run this before, picking a drive in setup did nothing at
	 * all: the folder was created there and then never looked at again,
	 * because the home one kept being found first. Two brains, and the one
	 * somebody explicitly asked for was the one silently ignored.
	 */
	if chosen, ok := LastKnown(); ok {
		if r, err := read(chosen); err == nil {
			r.Path = chosen

			return r, nil
		}
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

/*
 * Choose records where the brain should live from now on.
 *
 * Creating the folder is not choosing it. Setup used to do only the first, so
 * a drive picked on the first step was made and then never used — the search
 * order decided instead, and the search order does not know what anybody
 * asked for.
 */
func Choose(dir string) (Root, error) {
	r, err := Create(dir)
	if err != nil {
		return Root{}, err
	}

	Remember(r)

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
		/*
		 * Noticed before it is written down, because writing it down is what
		 * destroys the evidence.
		 *
		 * The same brain — same id — found somewhere other than where this
		 * machine last saw it means the drive has travelled.
		 */
		if was, id, ok := lastKnown(); ok && id != "" && id == r.ID && was != r.Path {
			r.MovedFrom = was
		}

		Remember(r)

		return r, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Root{}, err
	}

	/*
	 * Finding nothing means one of two opposite things.
	 *
	 * It means "first run" on a machine that has never had this, and it means
	 * "the drive is not plugged in" on a machine that has. They were treated
	 * as the same, so starting without the drive did what a first run does and
	 * created a new empty brain — and the only symptom of losing everything
	 * you had was that it introduced itself.
	 *
	 * The difference is knowable: if this machine has seen a brain before, it
	 * is remembered. Nothing is created over the top of that; the caller is
	 * told where the brain went instead.
	 */
	if away, ok := LastKnown(); ok {
		return Root{}, &AwayError{Path: away}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Root{}, err
	}

	created, err := Create(filepath.Join(home, ".local", "share", "pn-brain"))
	if err == nil {
		Remember(created)
	}

	return created, err
}

/*
 * AwayError says the brain is somewhere this machine cannot currently reach.
 *
 * Its own type because the caller has to do something specific with it —
 * naming the drive somebody needs to plug in is the entire value, and a
 * flattened string would lose it.
 */
type AwayError struct{ Path string }

func (e *AwayError) Error() string {
	return "the brain is kept at " + e.Path + ", which is not available right now"
}

// pointerFile records the last root that was actually used on this machine.
func pointerFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "pn-brain", "last-root.json"), nil
}

/*
 * Remember notes where the brain was, so its absence can be told from its
 * never having existed.
 *
 * Best effort on purpose: failing to write this must never stop the program
 * starting. The cost of losing it is that one future start mistakes a missing
 * drive for a first run, which is the behaviour that existed anyway.
 */
func Remember(r Root) {
	/*
	 * Except a root that was only borrowed for this run.
	 *
	 * Refused here as well as at the call sites, because there is one pointer
	 * file for the whole machine and everything about somebody's brain hangs
	 * off it. A guard that has to be remembered at each caller is a guard that
	 * gets forgotten at the next one.
	 */
	if r.Borrowed {
		return
	}

	path, err := pointerFile()
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	body, err := json.Marshal(map[string]string{"path": r.Path, "id": r.ID})
	if err != nil {
		return
	}

	_ = os.WriteFile(path, body, 0o600)
}

// LastKnown reports the root this machine used before, when that place is not
// there any more. A remembered root that still exists is not interesting: Find
// would have returned it.
func LastKnown() (string, bool) {
	path, _, ok := lastKnown()

	return path, ok
}

// lastKnown is the whole of what was written down: where, and which brain.
//
// The identity matters for telling two situations apart that look identical
// from the path alone — the same brain arriving at a new mount point, and a
// different brain being opened deliberately.
func lastKnown() (path, id string, ok bool) {
	file, err := pointerFile()
	if err != nil {
		return "", "", false
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		return "", "", false
	}

	var noted struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}

	if err := json.Unmarshal(raw, &noted); err != nil || noted.Path == "" {
		return "", "", false
	}

	return noted.Path, noted.ID, true
}

/*
 * Forget drops the pointer, for somebody who has decided to start again.
 *
 * Without this the refusal is permanent: a drive that is genuinely gone would
 * block every future start with an offer to plug in something that no longer
 * exists.
 */
func Forget() {
	if path, err := pointerFile(); err == nil {
		_ = os.Remove(path)
	}
}
