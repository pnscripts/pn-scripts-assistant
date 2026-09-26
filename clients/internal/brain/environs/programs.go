package environs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

/*
 * Looking for what is installed.
 *
 * A name on the PATH and, when it is there, what version it says it is. Both
 * matter and they cost very different amounts: finding a name is a few
 * microseconds, and asking a program its version means starting it, which for
 * a game engine is most of a second. So every name is looked for first and
 * only the ones that exist are asked anything.
 *
 * The list is written down rather than discovered by reading every directory
 * on the PATH. A machine has two thousand programs on its PATH and almost
 * none of them is something an assistant should reason about; a list of the
 * eighty that matter is the difference between a sentence the model can use
 * and a wall of text. Anything missing from it can be found by running it,
 * which the assistant can already do.
 */

// Program is something to look for, and how to ask its version.
type Program struct {
	ID    string
	Title string
	Kind  Kind

	// Names it may be installed under, in order of preference.
	Names []string

	// Places are folders to look in when it is not on the PATH, for the
	// things people download rather than install. ~ means this user's home.
	Places []string

	// Version is the arguments that make it say what it is. Empty means do
	// not ask — for anything slow enough that starting it is rude.
	Version []string

	/*
	 * Find is a finder that knows better than a name on the PATH.
	 *
	 * Some things are not installed, they are downloaded: a Godot is a single
	 * file somebody put on their desktop, named for its version, and looking
	 * for "godot" on the PATH finds nothing while the engine sits there. The
	 * packages that drive those things already know where they live, so this
	 * asks them rather than writing the knowledge down twice — which is what
	 * made four half-answers to "what is on this machine" in the first place.
	 *
	 * Nil means the ordinary search.
	 */
	Find func() (path, version string, ok bool)

	// Needs is what to do when it is not here, in words somebody can act on.
	Needs string

	// Local says using it keeps everything on this machine.
	Local bool
}

const (
	// HowLongToAsk bounds one version question.
	HowLongToAsk = 3 * time.Second

	// HowManyAtOnce bounds how many programs are asked together. Enough to
	// be quick, few enough that a laptop does not stall.
	HowManyAtOnce = 8
)

/*
 * Look finds which of these are installed, and what they say they are.
 *
 * Everything asked at once, bounded, because the slow ones are slow for
 * unrelated reasons and one after another they add up to a wait somebody
 * notices.
 */
func Look(ctx context.Context, programs []Program) []Thing {
	found := make([]Thing, len(programs))

	var (
		wg   sync.WaitGroup
		gate = make(chan struct{}, HowManyAtOnce)
		seen = time.Now()
		home = homeDir()
		_    = home
	)

	for i, program := range programs {
		wg.Add(1)

		go func(at int, p Program) {
			defer wg.Done()

			gate <- struct{}{}
			defer func() { <-gate }()

			found[at] = look(ctx, p, seen)
		}(i, program)
	}

	wg.Wait()

	return found
}

func look(ctx context.Context, p Program, seen time.Time) Thing {
	thing := Thing{
		ID: p.ID, Kind: p.Kind, Title: p.Title,
		Local: p.Local, Needs: p.Needs, Observed: seen,
	}

	// Asked first, because a package that drives this thing knows where it
	// keeps it and this does not.
	if p.Find != nil {
		if at, version, ok := p.Find(); ok {
			thing.State, thing.Path, thing.Version = Here, at, version

			return thing
		}
	}

	where, why := find(p)

	switch {
	case where == "" && why != "":
		// Here and unusable: no executable bit, or a folder nobody may read.
		// See provision.Recipe.blocked, which answers the same question about
		// the things this program can install.
		thing.State, thing.Why = Blocked, why

	case where == "":
		thing.State = WantsInstalling

	default:
		thing.State, thing.Path = Here, where
		thing.Version = versionOf(ctx, where, p.Version)
	}

	return thing
}

// find is where it is, or why it cannot be used although it is there.
func find(p Program) (where, why string) {
	for _, name := range p.Names {
		if at, err := exec.LookPath(name); err == nil {
			return at, ""
		}
	}

	home := homeDir()

	for _, dir := range p.Places {
		if strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}

			dir = filepath.Join(home, dir[2:])
		}

		for _, name := range p.Names {
			at := filepath.Join(dir, name)

			info, err := os.Stat(at)

			switch {
			case err == nil && !info.IsDir() && info.Mode()&0o111 != 0:
				return at, ""

			case err == nil && !info.IsDir():
				return "", "it is at " + at + " and is not executable — chmod +x " + at

			case os.IsPermission(err):
				return "", "it may be at " + at + ", which this user may not read"
			}
		}
	}

	return "", ""
}

// versionNumber is the first version-looking thing in a line of output.
var versionNumber = regexp.MustCompile(`\d+(\.\d+)+([A-Za-z0-9._\-]*)?`)

/*
 * versionOf asks a program what it is, briefly.
 *
 * Failure is not an error here: a program that will not say its version is
 * still installed, and reporting it as missing because it printed its help
 * instead would be worse than saying nothing. The number is pulled out of
 * whatever it printed, because "go version go1.26.6 linux/amd64" is not a
 * version and "1.26.6" is.
 */
func versionOf(ctx context.Context, path string, args []string) string {
	if len(args) == 0 {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, HowLongToAsk)
	defer cancel()

	out, _ := exec.CommandContext(ctx, path, args...).CombinedOutput()

	/*
	 * The number, from wherever in the output it appears.
	 *
	 * Not the first line: Codex prints a warning before its version, and
	 * taking the first line gave "WARNING: proceeding, even though we coul"
	 * as a version number — which is worse than saying nothing, because it
	 * reads like one.
	 */
	if found := versionNumber.FindString(string(out)); found != "" {
		return found
	}

	/*
	 * No number anywhere. The first line will do, unless it is a complaint:
	 * a program that warns instead of answering has not told us its version,
	 * and repeating the warning as one is a lie with a plausible shape.
	 */
	line := strings.TrimSpace(string(out))
	if line == "" {
		return ""
	}

	line = strings.TrimSpace(strings.SplitN(line, "\n", 2)[0])

	switch {
	case line == "",
		strings.Contains(strings.ToLower(line), "warning"),
		strings.Contains(strings.ToLower(line), "error"),
		strings.Contains(strings.ToLower(line), "usage"):
		return ""
	}

	if len(line) > 40 {
		line = line[:40]
	}

	return line
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return home
}
