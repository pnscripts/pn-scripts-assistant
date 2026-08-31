package window

import (
	"os/exec"
	"runtime"
)

/*
 * Opening the page in whatever browser this system has.
 *
 * The native window only exists on Linux, so on macOS and Windows setup
 * printed its address and waited — which asks somebody to read a URL out of a
 * terminal they may never have opened, and retype it. For a program whose
 * whole promise is that everything works from inside it, "the setup is at
 * 127.0.0.1:41533" is not setup working; it is setup being described.
 *
 * Best effort by design. Every one of these commands is absent on some machine
 * — a Linux box with no xdg-utils, a locked-down desktop — and failing to open
 * a browser must not stop the program. The address is printed either way, so
 * the worst case is exactly the behaviour that existed before.
 */
func OpenInBrowser(url string) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)

	case "windows":
		// Through the shell's own handler: "start" is a cmd builtin rather
		// than a program, and the empty string is the window title that start
		// otherwise steals from a quoted URL.
		cmd = exec.Command("cmd", "/c", "start", "", url)

	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return false
		}

		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		return false
	}

	// Released rather than waited on: these hand off to a browser that outlives
	// this call, and on some systems the launcher does not exit at all.
	go func() { _ = cmd.Wait() }()

	return true
}
