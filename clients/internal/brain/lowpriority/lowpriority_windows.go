package lowpriority

import (
	"os/exec"
	"syscall"
)

const belowNormalPriorityClass = 0x00004000

// prepare starts the child in the below-normal priority class.
func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.CreationFlags |= belowNormalPriorityClass
}

func lower(int) {}
