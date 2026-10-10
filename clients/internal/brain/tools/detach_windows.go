//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

// detach starts a program in a process group of its own, so it outlives this
// one.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
