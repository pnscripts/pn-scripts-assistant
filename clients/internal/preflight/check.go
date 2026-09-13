package preflight

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/storage"
	"strings"
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

// Blocking reports whether this alone stops PN Scripts Assistant from running.
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

// Blocking returns the requirements that stop PN Scripts Assistant running, so a caller
// can name them rather than only count them.
func Blocking(results []Result) []Result {
	return BlockingFor(results, false)
}

/*
 * BlockingFor is what is still missing, given how the assistant will think.
 *
 * paidBrain says a service has been configured that can answer without a model
 * on this machine. When it has, the pieces that exist only to run one locally
 * stop being required — see Requirement.OnlyForLocalBrain.
 *
 * Passed in rather than looked up, because this package has no idea where the
 * settings live and should not learn: the two callers that ask this question
 * both already hold the config.
 */
func BlockingFor(results []Result, paidBrain bool) []Result {
	blocking := make([]Result, 0, len(results))

	for _, r := range results {
		if paidBrain && r.Requirement.OnlyForLocalBrain {
			continue
		}

		if r.Blocking() {
			blocking = append(blocking, r)
		}
	}

	return blocking
}

func BlockingCount(results []Result) int {
	return len(Blocking(results))
}

func BlockingCountFor(results []Result, paidBrain bool) int {
	return len(BlockingFor(results, paidBrain))
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

/*
 * InstallAndVerify installs a requirement and proves it arrived.
 *
 * Both callers need the same three things — run the installer, check the
 * thing is actually there afterwards, and report the end of the output when
 * it is not — and they used to have them written out separately, in a package
 * that could not be reached from the other. Two copies of "did it work?" is
 * one copy too many for a question whose wrong answer sends somebody looking
 * for a feature that was never installed.
 */
func InstallAndVerify(r Requirement) (string, error) {
	var said strings.Builder

	if err := Install(r, &said); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", r.Name, err, LastFew(said.String()))
	}

	// Checked rather than trusted: an installer that exits zero and leaves
	// nothing behind is a thing that happens, and reporting success for it
	// means somebody looks for the feature and does not find it.
	if state, detail := r.Check(); state != OK {
		if detail != "" {
			return "", fmt.Errorf("%s finished but is still not there: %s", r.Name, detail)
		}

		return "", fmt.Errorf("%s finished but is still not there", r.Name)
	}

	return r.Name + " is installed.", nil
}

// LastFew is the end of an installer's output, which is where it says what
// went wrong. The rest is a progress bar drawn in text.
func LastFew(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}

	return strings.Join(lines, "\n")
}

// Removable reports whether this program installed the piece and can take it
// away again. See Requirement.RemoveFunc.
func (r Requirement) Removable() bool { return r.RemoveFunc != nil }

/*
 * RemoveAndVerify takes a piece off the machine and proves it went.
 *
 * The mirror of InstallAndVerify, and checked the same way and for the same
 * reason: a remove that quietly failed would leave the interface saying a
 * thing is gone while it is still there, and the next install would then be
 * asked to put back something that never left.
 */
func RemoveAndVerify(r Requirement) (string, error) {
	if r.RemoveFunc == nil {
		return "", fmt.Errorf("%s was not installed by this program, so it is not "+
			"mine to remove", r.Name)
	}

	var said strings.Builder

	if err := r.RemoveFunc(&said); err != nil {
		return "", fmt.Errorf("%s: %w\n%s", r.Name, err, LastFew(said.String()))
	}

	if state, _ := r.Check(); state == OK {
		return "", fmt.Errorf("%s is still here after removing it", r.Name)
	}

	return r.Name + " has been removed.", nil
}

// SizeOnDisk is how much a piece actually takes up, or zero when it is not
// this program's to measure. See Requirement.Occupies.
func (r Requirement) SizeOnDisk() int64 {
	if r.Occupies == nil {
		return 0
	}

	var total int64

	for _, path := range r.Occupies() {
		total += storage.SizeOf(path)
	}

	return total
}
