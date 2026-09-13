package speech

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
 * A recording belonging to a turn happening right now looks exactly like a
 * leftover: same name, same directory, still being written to.
 *
 * That is the whole risk in sweeping them, and it is why age decides rather
 * than the name. Deleting the file a live turn is recording into would produce
 * a fault nobody would connect to a cleanup — the microphone simply stopping,
 * once, for no reason.
 */
func TestSweepingLeavesALiveRecordingAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	fresh := filepath.Join(dir, "pn-scripts-assistant-turn-999.wav")
	stale := filepath.Join(dir, "pn-scripts-assistant-turn-111.wav")

	// Named the way a run from before the rename named it.
	listen := filepath.Join(dir, "pn-brain-listen-222.wav")
	other := filepath.Join(dir, "something-else.wav")

	for _, p := range []string{fresh, stale, listen, other} {
		if err := os.WriteFile(p, make([]byte, 2048), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	old := time.Now().Add(-3 * time.Hour)

	for _, p := range []string{stale, listen, other} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	removed, freed := SweepOldRecordings()

	if removed != 2 {
		t.Errorf("removed %d files, want the two stale recordings", removed)
	}

	if freed != 4096 {
		t.Errorf("freed %d bytes, want 4096", freed)
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Error("a recording from the last hour was deleted; a live turn writes into one of those")
	}

	if _, err := os.Stat(other); err != nil {
		t.Error("a file belonging to something else was deleted")
	}

	for _, gone := range []string{stale, listen} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s was left behind", filepath.Base(gone))
		}
	}
}
