//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// detach starts a program in a session of its own, so it outlives this one.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
