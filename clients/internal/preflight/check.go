package preflight

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Result pairs a requirement with what was found for it.
type Result struct {
	Requirement Requirement
	State       State
	Detail      string
}

func (r Result) Satisfied() bool {
	return r.State == OK
}

// Blocking reports whether this alone stops PN Brain from running.
func (r Result) Blocking() bool {
	return r.State != OK && !r.Requirement.Optional
}

// Check runs every requirement's check. Deliberately cheap and side-effect
// free, so it can run on every launch rather than once at install time.
func Check() []Result {
	reqs := Requirements()
	results := make([]Result, 0, len(reqs))

	for _, req := range reqs {
		state, detail := req.Check()
		results = append(results, Result{Requirement: req, State: state, Detail: detail})
	}

	return results
}

func BlockingCount(results []Result) int {
	count := 0

	for _, r := range results {
		if r.Blocking() {
			count++
		}
	}

	return count
}

func fileGlobExists(pattern string) bool {
	matches, err := filepath.Glob(pattern)

	return err == nil && len(matches) > 0
}

// Install satisfies one requirement, streaming output to w.
//
// Root commands are escalated with pkexec when there is a desktop session and
// no terminal to type a password into — a graphical prompt is the only way this
// can work from inside the app rather than from a shell.
func Install(r Requirement, w io.Writer) error {
	argv := r.InstallCmd()
	if argv == nil {
		return fmt.Errorf("%s must be installed manually: %s", r.Name, r.ManualHint)
	}

	argv = escalate(argv, r.NeedsRoot)

	fmt.Fprintf(w, "$ %v\n", argv)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w

	return cmd.Run()
}

// escalate swaps sudo for pkexec when running without a terminal, because sudo
// has nowhere to prompt and would simply hang.
func escalate(argv []string, needsRoot bool) []string {
	if !needsRoot || len(argv) == 0 || argv[0] != "sudo" {
		return argv
	}

	if isInteractiveTerminal() {
		return argv
	}

	if _, err := exec.LookPath("pkexec"); err != nil {
		return argv
	}

	return append([]string{"pkexec"}, argv[1:]...)
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
