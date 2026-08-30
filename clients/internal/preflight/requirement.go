package preflight

import (
	"fmt"
	"io"
	"os/exec"
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

// Requirement is one thing PN Brain needs in order to run properly.
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

	// Check reports the current state and a human-readable detail
	// (usually the installed version).
	Check func() (State, string)

	// InstallCmd returns the command to satisfy this requirement on the
	// current platform, or nil when it must be done by hand — some things
	// (a Docker Desktop download, a kernel feature) genuinely cannot be
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

// ollamaHasModel asks the local Ollama daemon rather than shelling out, so it
// also proves the daemon is actually reachable and not merely installed.
func ollamaHasModel(model string) bool {
	out, err := exec.Command("ollama", "list").CombinedOutput()
	if err != nil {
		return false
	}

	// `ollama list` prints "name:tag" in the first column; a bare name in the
	// requirement should match the ":latest" form too.
	base := strings.SplitN(model, ":", 2)[0]

	return strings.Contains(string(out), base)
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
