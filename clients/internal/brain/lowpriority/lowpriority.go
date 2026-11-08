/*
 * Package lowpriority runs the programs the assistant starts for work nobody
 * is waiting on — reading a PDF while it learns a folder — below everything
 * the person at the computer is doing.
 *
 * On Linux the child is reniced to 10 and put in the idle I/O class, so a
 * folder of documents being read never makes the desktop stutter; on Windows
 * it starts in the below-normal priority class. Either way it still gets the
 * whole machine when the machine is otherwise idle, which is when learning
 * happens anyway (owner 08.11.2026: the assistant should load the machine as
 * little as it possibly can).
 */
package lowpriority

import (
	"bytes"
	"os/exec"
)

// Nice is the niceness a background child gets on Linux.
const Nice = 10

// Output runs cmd at low priority and returns what it wrote to standard
// output, like exec.Cmd.Output.
func Output(cmd *exec.Cmd) ([]byte, error) {
	var out bytes.Buffer

	if cmd.Stdout == nil {
		cmd.Stdout = &out
	}

	prepare(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	lower(cmd.Process.Pid)

	err := cmd.Wait()

	return out.Bytes(), err
}
