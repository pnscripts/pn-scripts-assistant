package window

import (
	"os/exec"
	"runtime"
	"strings"
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
/*
 * launch is what actually starts the browser, replaceable so a test can check
 * this logic without a browser appearing on somebody's screen.
 *
 * Learned the hard way: a test that called OpenInBrowser for real opened two
 * tabs on the author's desktop every time the suite ran. A test with a visible
 * side effect on the machine running it is a broken test however green it goes.
 */
var launch = func(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	// Released rather than waited on: these hand off to a browser that outlives
	// this call, and on some systems the launcher does not exit at all.
	go func() { _ = cmd.Wait() }()

	return nil
}

func OpenInBrowser(url string) bool {
	args := opener(runtime.GOOS, url)
	if args == nil {
		return false
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		if _, err := exec.LookPath(args[0]); err != nil {
			return false
		}
	}

	return launch(exec.Command(args[0], args[1:]...)) == nil
}

/*
 * opener is the command that opens url on goos, or nil for an address that
 * must not be handed to the system at all.
 *
 * Only http and https: what is opened here is a page of this program's own,
 * and an address of any other kind — a file:, a javascript:, a path — is not
 * one of those whoever produced it.
 *
 * On Windows through rundll32's URL handler rather than "cmd /c start". The
 * shell reads & | ^ < > in an address as its own syntax, so an address with a
 * query string in it was cut at the first & and the rest run as a command.
 * The handler takes the address as one argument and interprets nothing.
 */
func opener(goos, url string) []string {
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil
	}

	if strings.ContainsAny(url, "\x00\r\n") {
		return nil
	}

	switch goos {
	case "darwin":
		return []string{"open", url}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	default:
		return []string{"xdg-open", url}
	}
}
