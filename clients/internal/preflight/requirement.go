package preflight

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// State is what a check found. "Outdated" matters as much as "missing":
// requirements are re-checked on every launch, not once at install time, so
// something that drifts behind has to be reportable without being fatal.
type State int

const (
	OK State = iota
	Missing
	Outdated
	Unknown
)

func (s State) Label() string {
	switch s {
	case OK:
		return "ok"
	case Missing:
		return "missing"
	case Outdated:
		return "outdated"
	default:
		return "unknown"
	}
}

// Requirement is one thing PN Scripts Assistant needs in order to run properly.
//
// Every requirement knows three things: how to see whether it is satisfied, how
// to satisfy it, and what breaks if it isn't. That last part is why Consequence
// exists — a list of red crosses tells someone what is wrong but not whether
// they should care, and several of these are genuinely optional.
type Requirement struct {
	Name        string
	Why         string // what it is for
	Consequence string // what stops working without it

	// Optional requirements are reported but never block startup.
	Optional bool

	/*
	 * OnlyForLocalBrain marks a piece that exists to run a model on this
	 * machine, and is therefore not required of somebody using a paid service.
	 *
	 * Ollama, the chat model and the embedding model are all of them. Without
	 * this the program refused to start for anybody on the paid path — the
	 * wizard let them through, having asked how it should think and been told,
	 * and then the launch checked a rule that had never heard the answer.
	 *
	 * A field rather than a list of names at each caller, because the two
	 * callers that ask "what is still missing" are in different packages and a
	 * rule written twice is a rule that drifts.
	 */
	OnlyForLocalBrain bool

	// Check reports the current state and a human-readable detail
	// (usually the installed version).
	Check func() (State, string)

	/*
	 * Where it is, or where it is going to go.
	 *
	 * Setup asks permission to install things and used to name none of the
	 * places it would put them, which is a strange thing to agree to — and on
	 * a machine where something is already present, "where is it?" had no
	 * answer inside the program at all. Answered before installing as the
	 * destination, and afterwards as the fact.
	 */
	Where func() string

	// InstallCmd returns the command to satisfy this requirement on the
	// current platform, or nil when it must be done by hand — some things
	// (a signed installer to download, a kernel feature) genuinely cannot be
	// scripted, and pretending otherwise wastes the user's time.
	InstallCmd func() []string

	/*
	 * InstallFunc is for the things that are more than one command.
	 *
	 * Ollama, the voice and the recogniser are each a download from their own
	 * project, unpacked into the home directory, and in one case a build. None
	 * of that fits in an argv, and the alternative — telling somebody to open a
	 * terminal — is the wall this whole page exists to remove.
	 */
	InstallFunc func(io.Writer) error

	/*
	 * Size is how big the download is, in words a person uses.
	 *
	 * The single most useful thing this screen was not saying. On a machine
	 * with nothing installed, pressing Apply fetches about five gigabytes —
	 * most of it one model — and somebody on a phone connection or a metered
	 * line found that out by watching it happen. "What will happen" is not
	 * answered by a list of names.
	 *
	 * Empty where there is nothing to download: an apt package pulls what the
	 * system decides it needs, and inventing a number for that would be worse
	 * than saying nothing.
	 */
	Size string

	/*
	 * RemoveFunc undoes what InstallFunc did, where that is this program's to
	 * undo.
	 *
	 * Only the pieces installed into the user's own folders have one: ollama,
	 * the voice, the recogniser and Godot were downloaded here, put in
	 * ~/.local, and nothing else on the machine depends on them. The system
	 * packages deliberately have none. ffmpeg and poppler arrived through apt
	 * and are shared — poppler is what the printing system uses to render a
	 * page — so a Remove button beside them would offer, in one click, to
	 * break something the person never connected to this program. They are
	 * reported as belonging to the system instead, which is true and is the
	 * more useful answer.
	 *
	 * Nil means "not ours to remove", which the interface says in those words
	 * rather than showing a button that fails.
	 */
	RemoveFunc func(io.Writer) error

	/*
	 * Occupies is every path this piece takes up, for measuring what removing
	 * it would give back.
	 *
	 * The same list the remover deletes, not a second one beside it. Size on
	 * disk and download size are different numbers and it is the first that
	 * somebody clearing space needs: the recogniser is a 30MB download that
	 * becomes half a gigabyte of built objects.
	 */
	Occupies func() []string

	// NeedsRoot marks installs that will prompt for a password.
	NeedsRoot bool

	// ManualHint is shown when InstallCmd is nil.
	ManualHint string
}

func (r Requirement) Installable() bool {
	if r.InstallFunc != nil {
		return true
	}

	return r.InstallCmd != nil
}

// commandExists is the cheapest possible check and covers most requirements.
/*
 * whereIs answers with the path a command was found at.
 *
 * Empty when it is not installed, so the caller can say where it would go
 * instead. The same lookup the shell does, which is the point: this is the
 * copy that would actually run, not the one somebody remembers installing.
 */
/*
 * Location answers Where, or says nothing rather than guessing.
 *
 * A requirement without one is a gap in what setup can tell somebody, not a
 * reason to invent a path — a wrong location is worse than none, because it
 * sends them to look somewhere the thing is not.
 */
func (r Requirement) Location() string {
	if r.Where == nil {
		return ""
	}

	return r.Where()
}

func whereIs(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}

	return path
}

/*
 * ollamaModelDir is where pulled models actually land.
 *
 * They are the largest thing setup downloads by a wide margin — several
 * gigabytes each — and they do not go anywhere near the brain's own folder, so
 * somebody who moved the brain to a big drive can still be surprised by where
 * the models went. OLLAMA_MODELS overrides it; otherwise it is the fixed place
 * ollama uses.
 */
// ModelDir is ollamaModelDir for callers outside this package — setup shows it
// on the last step, because it is where the gigabytes actually go.
func ModelDir() string { return ollamaModelDir() }

func ollamaModelDir() string {
	if set := os.Getenv("OLLAMA_MODELS"); set != "" {
		return set
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.ollama/models"
	}

	return filepath.Join(home, ".ollama", "models")
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)

	return err == nil
}

// versionOf runs a command and returns its first line, used as the detail shown
// beside a requirement. Failures are not errors here: the caller has already
// established the command exists, and a version string is a nicety.
func versionOf(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return ""
	}

	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if len(line) > 60 {
		line = line[:60]
	}

	return line
}

func pkgConfigExists(pkg string) bool {
	if !commandExists("pkg-config") {
		return false
	}

	return exec.Command("pkg-config", "--exists", pkg).Run() == nil
}

/*
 * ollamaHasModel asks the daemon, which is what the assistant talks to.
 *
 * The comment here used to say it asked the daemon "rather than shelling
 * out", and it shelled out — so it answered about a machine's installed files
 * rather than about a running service, and said no whenever the command was
 * somewhere this program could not see.
 */
func ollamaHasModel(model string) bool {
	// A bare name in the requirement should match the ":latest" form too.
	base := strings.SplitN(model, ":", 2)[0]

	for _, name := range installedModels() {
		if strings.Contains(name, base) {
			return true
		}
	}

	return false
}

func aptInstall(packages ...string) []string {
	return append([]string{"sudo", "apt-get", "install", "-y"}, packages...)
}

func Describe(state State, detail string) string {
	if detail == "" {
		return state.Label()
	}

	return fmt.Sprintf("%s (%s)", state.Label(), detail)
}
