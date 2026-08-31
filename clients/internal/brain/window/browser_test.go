package window

import (
	"os/exec"
	"runtime"
	"testing"
)

/*
 * The command this system would use has to exist, or the fallback must.
 *
 * Not that a browser opens — that needs a desktop and would make the test
 * depend on the machine's mood — but that OpenInBrowser reports honestly.
 * Returning true after failing to launch anything is the failure that matters,
 * because the caller then does not print the address and setup becomes
 * unreachable.
 */
func TestItOnlyClaimsSuccessWhenSomethingRan(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fallback path being checked is the Linux one")
	}

	_, err := exec.LookPath("xdg-open")
	have := err == nil

	// A URL that opens nothing harmful if a browser really does appear.
	got := OpenInBrowser("http://127.0.0.1:1/")

	if !have && got {
		t.Error("claimed to open a browser on a machine with no xdg-open")
	}

	if have && !got {
		t.Error("xdg-open is installed but it reported failure")
	}
}
