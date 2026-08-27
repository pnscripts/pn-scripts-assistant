//go:build linux

package speech

import (
	"os/exec"
	"syscall"
)

// dieWithParent asks the kernel to kill this child if the brain goes away.
//
// The recorder is normally stopped explicitly at the end of every turn, and on
// a clean exit that is enough. It is not enough when the brain dies without
// getting the chance: a crash, a SIGKILL, a pulled power cable during
// development. The recorder is then re-parented and goes on running — holding
// the microphone open and burning a little processor each — with nothing left
// that knows to stop it. Twenty-six of them were found running at once on this
// machine, the oldest more than four hours old, one for every time the brain
// had been killed outright that afternoon.
//
// A microphone that stays open after the program holding it has gone is worse
// than untidy, so the kernel is asked to close it.
func dieWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
