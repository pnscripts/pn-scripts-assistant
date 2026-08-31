//go:build !linux && !darwin

package preflight

// Nothing here escalates with sudo, so there is nothing to decide.
func stdinIsTTY() bool { return false }
