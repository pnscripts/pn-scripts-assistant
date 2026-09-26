//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"
)

/*
 * A process group of its own, the Unix way.
 *
 * Setpgid makes the child the leader of a new group, and a signal to the
 * negative of its id reaches the whole group — the compiler it started, the
 * renderer, whatever else. Killing only the process that was launched leaves
 * those behind holding the project open, which is how a build that was
 * stopped goes on writing files.
 */
func OwnGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setpgid = true
}

// KillGroup stops a process and everything it started.
func KillGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGKILL) }

// StopGroup asks a process and everything it started to stop, which is the
// polite half of KillGroup: a build tool given a moment writes out what it
// has rather than leaving half a file.
func StopGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGTERM) }
