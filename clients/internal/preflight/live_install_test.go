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
 * Skipped unless PN_SCRIPTS_ASSISTANT_LIVE_INSTALL is set, because it downloads eighty
 * megabytes from the internet and no unit test should do that on its own. It
 * exists because the alternative was shipping an installer nobody had ever
 * run — and the first URL written into this file was a 404, which no amount of
 * reading it would have revealed.
 */
func TestInstallingTheVoiceForReal(t *testing.T) {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_LIVE_INSTALL") == "" {
		t.Skip("set PN_SCRIPTS_ASSISTANT_LIVE_INSTALL=1 to download and install for real")
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

/*
 * And listening: whisper.cpp from its pinned commit, built, and the pinned
 * speech model beside it. Same switch; it compiles for a few minutes and
 * downloads 488MB, which is the point of running it before a release.
 */
func TestInstallingListeningForReal(t *testing.T) {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_LIVE_INSTALL") == "" {
		t.Skip("set PN_SCRIPTS_ASSISTANT_LIVE_INSTALL=1 to download, build and install for real")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	var log strings.Builder

	if err := installWhisper(&log); err != nil {
		t.Fatalf("installing listening failed: %v\n%s", err, log.String())
	}

	src := filepath.Join(home, ".local", "src", "whisper.cpp")

	if at, err := headOf(src); err != nil || at != WhisperCommit {
		t.Errorf("whisper.cpp is at %q (%v), want the pinned %s", at, err, WhisperCommit)
	}

	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "whisper-cli")); err != nil {
		t.Errorf("whisper-cli is not on the path: %v", err)
	}

	if err := Matches(filepath.Join(src, "models", SpeechModel), "sha256", SpeechModelSHA256); err != nil {
		t.Errorf("the speech model is not the pinned one: %v", err)
	}

	// Pressed again: nothing is downloaded or moved, everything is checked.
	log.Reset()

	if err := installWhisper(&log); err != nil {
		t.Fatalf("installing a second time failed: %v\n%s", err, log.String())
	}

	if strings.Contains(log.String(), "Downloading "+SpeechModel) {
		t.Errorf("the second install downloaded the model again:\n%s", log.String())
	}
}
