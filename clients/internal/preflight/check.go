package preflight

import (
	"fmt"
	"io"
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

// Blocking returns the requirements that stop PN Brain running, so a caller
// can name them rather than only count them.
func Blocking(results []Result) []Result {
	blocking := make([]Result, 0, len(results))

	for _, r := range results {
		if r.Blocking() {
			blocking = append(blocking, r)
		}
	}

	return blocking
}

func BlockingCount(results []Result) int {
	return len(Blocking(results))
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
	// The things that are a download and an unpacking rather than a command.
	if r.InstallFunc != nil {
		return r.InstallFunc(w)
	}

	if r.InstallCmd == nil {
		return fmt.Errorf("%s must be installed manually: %s", r.Name, r.ManualHint)
	}

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

/*
 * isInteractiveTerminal asks whether there is somewhere for sudo to prompt.
 *
 * It used to ask whether stdin was a character device, which /dev/null is —
 * and /dev/null is exactly what a desktop launcher hands a program it starts.
 * So the check said "there is a terminal" in precisely the case there was
 * none, kept sudo, and sudo then failed with "a terminal is required to read
 * the password". The code existed to spare GUI users that error and was
 * switched off for GUI users alone; from a real terminal, where it did
 * nothing, it looked correct.
 *
 * Asking the kernel for the terminal settings is the actual question. A pipe,
 * a file and /dev/null all refuse it; only a tty answers.
 */
func isInteractiveTerminal() bool {
	return stdinIsTTY()
}
