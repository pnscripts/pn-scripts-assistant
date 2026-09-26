//go:build !linux

package orchestrator

import (
	"os/exec"
	"strings"
)

/*
 * Which kernel this is, on a system with no uname syscall of Linux's shape.
 *
 * The command exists on macOS and says the same thing; on Windows there is
 * neither, and an empty answer is the honest one — the machine report says
 * what it knows and leaves out what it does not.
 */
func kernelName() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
