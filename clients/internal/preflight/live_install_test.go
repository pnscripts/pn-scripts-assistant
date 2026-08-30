package preflight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Actually install the voice, into a home directory that is thrown away.
 *
 * Skipped unless PN_BRAIN_LIVE_INSTALL is set, because it downloads eighty
 * megabytes from the internet and no unit test should do that on its own. It
 * exists because the alternative was shipping an installer nobody had ever
 * run — and the first URL written into this file was a 404, which no amount of
 * reading it would have revealed.
 */
func TestInstallingTheVoiceForReal(t *testing.T) {
	if os.Getenv("PN_BRAIN_LIVE_INSTALL") == "" {
		t.Skip("set PN_BRAIN_LIVE_INSTALL=1 to download and install for real")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	var log strings.Builder

	if err := installPiper(&log); err != nil {
		t.Fatalf("installing the voice failed: %v\n%s", err, log.String())
	}

	for _, want := range []string{
		filepath.Join(home, ".local", "share", "piper", "piper"),
		filepath.Join(home, ".local", "share", "piper", "voices", "en_GB-alba-medium.onnx"),
		filepath.Join(home, ".local", "bin", "piper"),
	} {
		info, err := os.Stat(want)
		if err != nil {
			t.Errorf("%s is missing after installing: %v", want, err)

			continue
		}

		if info.Size() == 0 {
			t.Errorf("%s is empty", want)
		}
	}

	// And the check that decides whether the button disappears now agrees.
	for _, r := range Requirements() {
		if r.Name != "Voice (speaking)" {
			continue
		}

		if state, detail := r.Check(); state != OK {
			t.Errorf("installed the voice and the check still says %v (%s)", state, detail)
		} else if !strings.Contains(detail, "piper") {
			t.Errorf("the check passed but not because of piper: %q", detail)
		}
	}
}
