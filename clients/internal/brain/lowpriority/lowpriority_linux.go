package lowpriority

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func prepare(*exec.Cmd) {}

// lower renices the child and moves it to the idle I/O class. Failure leaves
// it at normal priority, which is how it ran before; nothing depends on it.
func lower(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, Nice)

	const whoProcess, idleClass = 1, 3

	_, _, _ = unix.Syscall(unix.SYS_IOPRIO_SET, whoProcess, uintptr(pid), idleClass<<13)
}
