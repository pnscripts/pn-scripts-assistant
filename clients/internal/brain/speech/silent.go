package speech

import (
	"errors"
	"os"
	"sync"
	"testing"
)

/*
 * Silent: a process that must make no sound and touch no audio device.
 *
 * Learned the hard way (2026-10-25): a UI preview served the real interface
 * from a temporary brain and loaded it in a headless browser. The page greets
 * on load through /api/speak, the server handed the greeting to espeak-ng,
 * and a Bulgarian sentence in a robot voice came out of the owner's speakers
 * while nobody was looking at the program — from time to time, whenever a
 * check ran. Nothing about a test or a preview should be able to do that.
 *
 * Silent is true in every test binary, and in any process started with
 * PN_SCRIPTS_ASSISTANT_SILENT=1 (previews, harnesses, CI, `make` targets).
 * Every path here that makes a sound, records from a microphone, restarts the
 * sound server or changes a volume asks it first. What would have been said is
 * kept in Silenced, so a test can still check what the program meant to say.
 *
 * A test that genuinely needs the real path — and none in the suite does —
 * sets PN_TEST_REAL_AUDIO=1 in its environment by hand.
 */
func Silent() bool {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_SILENT") == "1" {
		return true
	}

	return testing.Testing() && os.Getenv("PN_TEST_REAL_AUDIO") != "1"
}

var hushed struct {
	sync.Mutex
	said []string
}

// hush reports whether to stay silent, and keeps what would have been said.
func hush(text string) bool {
	if !Silent() {
		return false
	}

	hushed.Lock()
	defer hushed.Unlock()

	if len(hushed.said) < 200 {
		hushed.said = append(hushed.said, text)
	}

	return true
}

// Silenced is what would have been said aloud while Silent, oldest first.
func Silenced() []string {
	hushed.Lock()
	defer hushed.Unlock()

	return append([]string(nil), hushed.said...)
}

// errSilent is a recording asked for while Silent: there is no microphone.
var errSilent = errors.New("no microphone: this process is silent (a test or a preview)")
