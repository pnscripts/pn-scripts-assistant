package logs

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What happens is written where somebody can read it afterwards, and to the
// terminal as well for whoever is watching one.
func TestItWritesToAFileAndSaysWhere(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	logger, where := Open(slog.LevelInfo)

	if where == "" {
		t.Fatal("it did not say where the log is")
	}

	if !strings.HasPrefix(where, state) {
		t.Errorf("the log went to %s, outside %s", where, state)
	}

	logger.Info("the brain started", "port", 8790)

	written, err := os.ReadFile(where)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), "the brain started") {
		t.Errorf("the line is not in the file:\n%s", written)
	}

	// Nobody else's business on a shared machine.
	if info, err := os.Stat(where); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the log is %v, not 0600 (%v)", info.Mode().Perm(), err)
	}
}

/*
 * A log that is never rotated is a disk somebody loses.
 *
 * Written against the real rule rather than a smaller one: the file is moved
 * aside when it reaches its size, the newest keeps the plain name, and only
 * so many are kept.
 */
func TestItRotatesAndKeepsOnlySoMany(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	where := File()

	if err := os.MkdirAll(filepath.Dir(where), 0o700); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < Kept+2; i++ {
		if err := os.WriteFile(where, []byte(strings.Repeat("x", Biggest+1)), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := open(where); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := os.Stat(where + "." + string(rune('0'+Kept+1))); err == nil {
		t.Errorf("it kept more than %d older logs", Kept)
	}

	for i := 1; i <= Kept; i++ {
		if _, err := os.Stat(where + "." + string(rune('0'+i))); err != nil {
			t.Errorf("older log %d is missing: %v", i, err)
		}
	}

	// And the newest is the plain name, started again from empty.
	if info, err := os.Stat(where); err != nil || info.Size() > Biggest {
		t.Errorf("the current log is %v (%v)", info.Size(), err)
	}
}

// With nowhere to write, the program still runs and still says its lines.
func TestWithNowhereToWriteItStillSpeaks(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "state")

	if err := os.WriteFile(blocked, []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_STATE_HOME", blocked)

	logger, where := Open(slog.LevelInfo)

	if where != "" {
		t.Errorf("it claimed a log at %s", where)
	}

	if logger == nil {
		t.Fatal("it gave back no logger at all")
	}

	logger.Info("still working")
}
