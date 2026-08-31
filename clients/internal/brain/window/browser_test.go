package window

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

/*
 * Nothing here starts a browser.
 *
 * The first version of this test called OpenInBrowser for real, which opened
 * two tabs on the screen of whoever ran the suite. What is worth checking is
 * the decision — which command, and whether the answer is honest — and none of
 * that needs a browser to actually appear.
 */
func TestItAsksTheRightProgramForThisSystem(t *testing.T) {
	var asked *exec.Cmd

	real := launch
	launch = func(cmd *exec.Cmd) error { asked = cmd; return nil }

	t.Cleanup(func() { launch = real })

	if !OpenInBrowser("http://example.invalid/") {
		if runtime.GOOS == "linux" {
			if _, err := exec.LookPath("xdg-open"); err != nil {
				t.Skip("no xdg-open on this machine, which is its own answer")
			}
		}

		t.Fatal("it reported failure despite the launch succeeding")
	}

	want := map[string]string{"darwin": "open", "windows": "cmd"}[runtime.GOOS]
	if want == "" {
		want = "xdg-open"
	}

	if asked == nil {
		t.Fatal("nothing was launched at all")
	}

	if got := asked.Args[0]; got != want {
		t.Errorf("asked %q to open the page, want %q", got, want)
	}

	// The address has to survive into the command, or a browser opens on
	// nothing and the person is no better off.
	found := false

	for _, arg := range asked.Args {
		if arg == "http://example.invalid/" {
			found = true
		}
	}

	if !found {
		t.Errorf("the address never reached the command: %v", asked.Args)
	}
}

/*
 * Claiming success after failing to launch anything is the failure that
 * matters: the caller then does not print the address, and setup becomes
 * unreachable with no clue why.
 */
func TestItNeverClaimsSuccessItDidNotHave(t *testing.T) {
	real := launch
	launch = func(*exec.Cmd) error { return errors.New("no browser here") }

	t.Cleanup(func() { launch = real })

	if OpenInBrowser("http://example.invalid/") {
		t.Error("it claimed to open a browser that failed to start")
	}
}
