//go:build !linux

package speech

import "os/exec"

// dieWithParent has no equivalent on these platforms.
//
// Linux can be asked to signal a child when its parent dies; the others cannot,
// at least not this simply. The explicit stop at the end of each turn still
// runs, so this only affects what happens after a crash.
func dieWithParent(cmd *exec.Cmd) {}
