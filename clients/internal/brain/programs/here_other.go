//go:build !windows

package programs

import "os/exec"

// windowsHere: nothing to add off Windows.
func windowsHere(*Scanner) {}

func noWindow(*exec.Cmd) {}

// keyStamp: no registry off Windows; a key is never there.
func keyStamp(string) (string, bool) { return "-", false }
