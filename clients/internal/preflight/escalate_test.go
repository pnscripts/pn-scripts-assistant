package preflight

import "testing"

/*
 * The check that decides whether sudo can prompt has to be right for the case
 * it exists to serve: a program started from a desktop icon, whose stdin is
 * /dev/null. That is a character device, so the old test called it a terminal
 * and kept sudo — which then failed with "a terminal is required", the exact
 * error this was written to prevent. It only ever behaved correctly when run
 * from a terminal, where it made no difference.
 */
func TestDevNullIsNotATerminal(t *testing.T) {
	if stdinIsTTY() {
		t.Skip("this test run has a real terminal attached")
	}

	got := escalate([]string{"sudo", "apt-get", "install", "-y", "x"}, true)

	if got[0] == "sudo" {
		t.Errorf("kept sudo with no terminal to prompt on: %v", got)
	}
}
