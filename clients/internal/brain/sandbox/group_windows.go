//go:build windows

package sandbox

import (
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

/*
 * A process group of its own, the Windows way.
 *
 * CREATE_NEW_PROCESS_GROUP is the nearest thing to setpgid: the child does
 * not share this program's console group, so stopping one does not stop the
 * other by accident.
 *
 * Killing the tree is a different matter. Windows has no signal that reaches
 * a group, and the reliable way to stop a program and the ones it started is
 * taskkill with /T — which is a command rather than a syscall, and is the
 * same command every build tool on Windows ends up using for this.
 */
func OwnGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

// KillGroup stops a process and everything it started.
func KillGroup(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

/*
 * StopGroup asks a process and everything it started to stop.
 *
 * Windows has no SIGTERM. taskkill without /F asks a program to close, which
 * a console program with no window will often ignore — so this is the polite
 * attempt and KillGroup is what actually ends it, which is the same shape the
 * Unix side has for a different reason.
 */
func StopGroup(pid int) error {
	return exec.Command("taskkill", "/T", "/PID", strconv.Itoa(pid)).Run()
}
