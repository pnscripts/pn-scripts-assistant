/*
 * Package provision is what a piece of work needs on this machine, and how it
 * gets there.
 *
 * The machine parts were the first version of this: a fixed list of what the
 * assistant itself needs, each with a check and an installer. An organisation
 * that hires a game developer, a book writer or a backend engineer needs more
 * than the assistant does — an engine, a runtime, a converter — and the same
 * rules have to hold for all of it, so this is that list made general rather
 * than a second list beside it. The parts are recipes here too.
 *
 * A recipe is declared, never composed. It says how to find the thing and read
 * its version, where it comes from, what it costs and under what licence, how
 * big it is, where it would go, and the one fixed way to install it — or that
 * it cannot be installed from here and what a person has to do instead. Nothing
 * an agent writes becomes a download address or an install command: the only
 * installs that exist are the ones written in this package, and every one of
 * them still needs its owner to say yes.
 */
package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Kinds of thing a recipe provides, for grouping in the interface.
const (
	Engine  = "engine"
	Runtime = "runtime"
	Tool    = "tool"
	Viewer  = "viewer"
	Part    = "part"
)

// Recipe is one thing a piece of work may need.
type Recipe struct {
	ID    string
	Title string
	Kind  string

	// Why is what it is for, in general. A package says why it wants it.
	Why string

	// Commands are the names it answers to on PATH, and Places the folders a
	// single-file download tends to end up in, which a program started from a
	// desktop menu does not have on its PATH.
	Commands []string
	Places   []string

	// VersionArgs are run against what was found, and VersionPattern's first
	// group is the version in what comes back.
	VersionArgs    []string
	VersionPattern *regexp.Regexp

	// Find replaces all of the above, for something found another way — an
	// editor installed by its own hub, a folder rather than a command.
	Find func() (path, version string, ok bool)

	/*
	 * Licensed says whether what was found may actually be used: "active",
	 * "inactive" with why, or "unknown". Only for software its owner
	 * licenses — Unity is installed and still refuses to work unattended
	 * until its owner has signed in, and "installed" is not "usable".
	 */
	Licensed func(path string) (state, why string)

	// Where it comes from, what it costs and under what terms, in words a
	// person reads before agreeing to it.
	Source  string
	Licence string
	Cost    string
	Size    string

	// InstallTo is where it would go, answered before installing.
	InstallTo func() string

	// Install is the one fixed way to put it here. Nil means it cannot be
	// installed from this program, and Manual says what a person does.
	Install func(ctx context.Context, w io.Writer) error
	Manual  string

	// NeedsRoot is an install that will ask for a password.
	NeedsRoot bool

	// Remove takes back what Install did, where that is this program's to
	// take back. Nil for anything the system or another program owns.
	Remove func(w io.Writer) error

	// Smoke is a harmless run that proves it works, not only that it is
	// there: the arguments after the program's own path.
	Smoke []string
}

// Installable says whether this program can put it here.
func (r Recipe) Installable() bool { return r.Install != nil }

// Status is what was found, against what was wanted.
type Status struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Why   string `json:"why,omitempty"`

	Present bool   `json:"present"`
	Path    string `json:"path,omitempty"`

	/*
	 * Blocked is here and unusable: no executable bit, or a folder this user
	 * may not read. Present stays false — it cannot be run — but the remedy
	 * is not the one absence calls for, and a reinstall over somebody else's
	 * file is the wrong answer. Problem says what to do instead.
	 */
	Blocked bool `json:"blocked,omitempty"`

	Version string `json:"version,omitempty"`

	// Wanted is the version rule it was checked against, and Compatible the
	// answer. Something present in the wrong version is not a pass.
	Wanted     string `json:"wanted,omitempty"`
	Compatible bool   `json:"compatible"`
	Problem    string `json:"problem,omitempty"`

	// Licence is "active", "inactive" or "unknown" for licensed software,
	// and empty for everything else; LicenceWhy says what it said.
	LicenceState string `json:"licence_state,omitempty"`
	LicenceWhy   string `json:"licence_why,omitempty"`

	Installable bool   `json:"installable"`
	NeedsRoot   bool   `json:"needs_root,omitempty"`
	Manual      string `json:"manual,omitempty"`
	Source      string `json:"source,omitempty"`
	Licence     string `json:"licence,omitempty"`
	Cost        string `json:"cost,omitempty"`
	Size        string `json:"size,omitempty"`
	InstallTo   string `json:"install_to,omitempty"`
	Removable   bool   `json:"removable,omitempty"`
}

// Ready is present, in a version that will do, and — for licensed software
// — licensed for this use. Unknown is not ready: nothing is claimed that was
// not seen.
func (s Status) Ready() bool {
	return s.Present && s.Compatible && (s.LicenceState == "" || s.LicenceState == LicenceActive)
}

// Licence states.
const (
	LicenceActive   = "active"
	LicenceInactive = "inactive"
	LicenceUnknown  = "unknown"
)

/*
 * Check finds it and weighs its version against a rule.
 *
 * Cheap and without side effects beyond asking the program its version, so it
 * can be run every time a proposal is shown rather than remembered from last
 * week — a thing somebody installed or removed in the meantime is exactly what
 * a remembered answer gets wrong.
 */
func (r Recipe) Check(want string) Status {
	s := Status{
		ID: r.ID, Title: r.Title, Kind: r.Kind, Why: r.Why, Wanted: want,
		Installable: r.Installable(), NeedsRoot: r.NeedsRoot, Manual: r.Manual,
		Source: r.Source, Licence: r.Licence, Cost: r.Cost, Size: r.Size,
		Removable: r.Remove != nil,
	}

	if r.InstallTo != nil {
		s.InstallTo = r.InstallTo()
	}

	s.Path, s.Version, s.Present = r.find()

	if !s.Present {
		/*
		 * Absent and unusable are different answers.
		 *
		 * A program sitting in a folder without its executable bit, or in a
		 * folder this user may not read, was reported as "not on this
		 * machine" — so somebody was told to install what they already had.
		 * The difference is knowable, and the action is different: one is an
		 * install, the other is a chmod or a folder somebody else owns.
		 */
		if at, why := r.blocked(); why != "" {
			s.Path, s.Problem, s.Blocked = at, why, true

			return s
		}

		s.Problem = "not on this machine"

		return s
	}

	rule, err := ParseRule(want)
	if err != nil {
		s.Problem = err.Error()

		return s
	}

	switch {
	case rule.Empty():
		s.Compatible = true
	case s.Version == "":
		// Present and unable to say which version. Not called compatible:
		// a rule that is not checked is not a rule.
		s.Problem = "present, but it would not say which version it is"
	case rule.Allows(s.Version):
		s.Compatible = true
	default:
		s.Problem = fmt.Sprintf("version %s is here, and %s is needed", s.Version, want)
	}

	if r.Licensed != nil && s.Compatible {
		s.LicenceState, s.LicenceWhy = r.Licensed(s.Path)

		switch s.LicenceState {
		case LicenceInactive:
			s.Problem = "installed, but not licensed for this use: " + s.LicenceWhy
		case LicenceUnknown:
			s.Problem = "installed; whether it is licensed could not be told: " + s.LicenceWhy
		}
	}

	return s
}

func (r Recipe) find() (path, version string, ok bool) {
	if r.Find != nil {
		return r.Find()
	}

	path = r.locate()
	if path == "" {
		return "", "", false
	}

	return path, r.versionOf(path), true
}

/*
 * blocked is a program that is here and cannot be used, and why.
 *
 * Looked for only after the ordinary search has failed, so it costs nothing in
 * the usual case. Two reasons are worth telling apart because the remedy
 * differs: no executable bit, and a folder that cannot be read at all.
 */
func (r Recipe) blocked() (path, why string) {
	for _, dir := range r.folders() {
		for _, name := range r.Commands {
			at := filepath.Join(dir, name)

			info, err := os.Stat(at)

			switch {
			case err == nil && !info.IsDir() && info.Mode()&0o111 == 0:
				return at, "found at " + at + ", but it is not executable — chmod +x " + at

			case errors.Is(err, fs.ErrPermission):
				return at, "found at " + at + ", but this user may not read it"
			}
		}

		if _, err := os.ReadDir(dir); errors.Is(err, fs.ErrPermission) {
			return dir, "it may be in " + dir + ", which this user may not read"
		}
	}

	return "", ""
}

func (r Recipe) locate() string {
	for _, name := range r.Commands {
		if at, err := exec.LookPath(name); err == nil {
			return at
		}
	}

	for _, dir := range r.folders() {
		for _, name := range r.Commands {
			at := filepath.Join(dir, name)

			if info, err := os.Stat(at); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return at
			}
		}
	}

	return ""
}

// folders are the Places with ~ meaning this user's home.
func (r Recipe) folders() []string {
	home, _ := os.UserHomeDir()

	dirs := make([]string, 0, len(r.Places))

	for _, dir := range r.Places {
		if strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}

			dir = filepath.Join(home, dir[2:])
		}

		dirs = append(dirs, dir)
	}

	return dirs
}

// HowLongAVersionTakes bounds asking a program which version it is. Long
// enough for an engine that loads a little before it answers.
const HowLongAVersionTakes = 15 * time.Second

func (r Recipe) versionOf(path string) string {
	if len(r.VersionArgs) == 0 {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), HowLongAVersionTakes)
	defer cancel()

	out, _ := exec.CommandContext(ctx, path, r.VersionArgs...).CombinedOutput()

	text := strings.TrimSpace(string(out))

	if r.VersionPattern != nil {
		if m := r.VersionPattern.FindStringSubmatch(text); len(m) > 1 {
			return m[1]
		}

		return ""
	}

	if m := anyVersion.FindString(text); m != "" {
		return m
	}

	return ""
}

var anyVersion = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

/*
 * Carry out installs a recipe, then proves it.
 *
 * Proved three ways, in order, because each catches something the one before
 * cannot: it can be found, it is a version the rule allows, and the smoke run
 * exits cleanly. An installer that returned without error and left nothing
 * usable is a thing that happens, and reporting success for it sends somebody
 * looking for a feature that is not there.
 *
 * The caller has already had its owner's yes. Nothing here asks.
 */
func Carry(ctx context.Context, r Recipe, want string, w io.Writer) (Status, error) {
	if r.Install == nil {
		return r.Check(want), fmt.Errorf("%s cannot be installed from here: %s", r.Title, orSaid(r.Manual))
	}

	if err := r.Install(ctx, w); err != nil {
		return r.Check(want), fmt.Errorf("installing %s: %w", r.Title, err)
	}

	after := r.Check(want)

	if !after.Present {
		return after, fmt.Errorf("%s finished installing and is still not there", r.Title)
	}

	if !after.Compatible {
		return after, fmt.Errorf("%s is installed, but %s", r.Title, after.Problem)
	}

	if err := Smoke(ctx, r, after.Path, w); err != nil {
		return after, err
	}

	return after, nil
}

// HowLongASmokeTestTakes bounds the harmless run.
const HowLongASmokeTestTakes = 60 * time.Second

// Smoke runs the recipe's harmless check against what was found.
func Smoke(ctx context.Context, r Recipe, path string, w io.Writer) error {
	if len(r.Smoke) == 0 || path == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, HowLongASmokeTestTakes)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, r.Smoke...)
	out, err := cmd.CombinedOutput()

	fmt.Fprintf(w, "$ %s %s\n%s\n", filepath.Base(path), strings.Join(r.Smoke, " "), lastLines(string(out), 8))

	if err != nil {
		return fmt.Errorf("%s is installed but did not run: %v\n%s", r.Title, err, lastLines(string(out), 8))
	}

	return nil
}

// Take removes what this program installed, and proves it went.
func Take(r Recipe, w io.Writer) error {
	if r.Remove == nil {
		return fmt.Errorf("%s was not installed by this program, so it is not mine to remove", r.Title)
	}

	if err := r.Remove(w); err != nil {
		return err
	}

	if _, _, still := r.find(); still {
		return fmt.Errorf("%s is still here after removing it", r.Title)
	}

	return nil
}

func orSaid(manual string) string {
	if manual == "" {
		return "nobody has written down how"
	}

	return manual
}

func lastLines(out string, n int) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}

// Book is every recipe this program knows, by id.
type Book struct {
	byID map[string]Recipe
}

// NewBook holds recipes. A later one with the same id replaces an earlier.
func NewBook(recipes ...Recipe) *Book {
	b := &Book{byID: map[string]Recipe{}}

	for _, r := range recipes {
		if r.ID != "" {
			b.byID[r.ID] = r
		}
	}

	return b
}

// Get is one recipe.
func (b *Book) Get(id string) (Recipe, bool) {
	if b == nil {
		return Recipe{}, false
	}

	r, ok := b.byID[id]

	return r, ok
}

// Known answers whether an id is a recipe, for checking what a package names.
func (b *Book) Known(id string) bool {
	_, ok := b.Get(id)

	return ok
}

// All is every recipe, by kind and then title.
func (b *Book) All() []Recipe {
	if b == nil {
		return nil
	}

	out := make([]Recipe, 0, len(b.byID))

	for _, r := range b.byID {
		out = append(out, r)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}

		return out[i].Title < out[j].Title
	})

	return out
}

// Check is one recipe's status by id, or a status saying it is not known.
func (b *Book) Check(id, want string) Status {
	r, ok := b.Get(id)
	if !ok {
		return Status{ID: id, Title: id, Wanted: want, Problem: "no recipe of that name"}
	}

	return r.Check(want)
}
