//go:build linux || darwin

package preflight

import (
	"os"

	"golang.org/x/sys/unix"
)

// stdinIsTTY reports whether standard input is a real terminal.
//
// By asking for the terminal attributes: every non-terminal refuses, which is
// the distinction sudo itself cares about.
func stdinIsTTY() bool {
	_, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), tcGetAttr)

	return err == nil
}
