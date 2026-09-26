//go:build !linux

package sandbox

import "fmt"

/*
 * Confinement by the kernel is a Linux facility, and saying so is the whole
 * of what this file does.
 *
 * Not silence and not a pretence: a caller that asks to be confined on macOS
 * or Windows is told it cannot be, and decides what to do about it. The rest
 * of the sandbox — a scrubbed environment, a process group of its own, argv
 * rather than a shell — works on every system and is in sandbox.go.
 */

// ABI reports that this kernel has no Landlock, because it is not Linux.
func ABI() (int, error) {
	return 0, fmt.Errorf("work is confined by the kernel on Linux only")
}

// Restrict cannot confine anything here, and says so rather than returning
// nil — which would read as "confined" to every caller that checks.
func Restrict(dirs []string) error {
	return fmt.Errorf("work is confined by the kernel on Linux only; %d folders were not restricted",
		len(dirs))
}

/*
 * Main is how a confined command is started on Linux: a copy of this program
 * restricts itself and becomes the command. There is nothing to do here, and
 * false means "this run is the program itself, carry on".
 */
func Main() bool { return false }
