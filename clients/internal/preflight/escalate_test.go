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

	got, err := forRoot([]string{"sudo", "apt-get", "install", "-y", "x"}, true)

	/*
	 * Two honest answers with no terminal, and which one depends on whether
	 * this machine has a way to ask on screen.
	 *
	 * On a desktop, pkexec asks. On a machine with neither — a container, a
	 * build runner — there is nowhere to ask at all, and saying so is the
	 * answer. What must never happen is the third thing: keeping sudo, which
	 * can only end in "a terminal is required to read the password" after a
	 * download, inside an installer somebody is watching.
	 */
	if err != nil {
		if got != nil {
			t.Errorf("it refused and still handed back %v", got)
		}

		return
	}

	if len(got) == 0 || got[0] == "sudo" {
		t.Errorf("kept sudo with no terminal to prompt on: %v", got)
	}
}
