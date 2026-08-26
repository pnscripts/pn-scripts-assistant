package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seed(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		".brain-root.json":  `{"id":"x","schema":1}`,
		"brain.sqlite":      strings.Repeat("data", 5000),
		"brain.conf":        "BRAIN_PRIVACY=private\n",
		"backups/dump.json": strings.Repeat("old", 1000),
	}

	for rel, content := range files {
		full := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(full), 0o755)

		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestMoveCopiesEverythingAndVerifies(t *testing.T) {
	from := seed(t)
	to := filepath.Join(t.TempDir(), "moved")

	rep, err := Move(from, to, false, nil)
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}

	if rep.Files != 4 {
		t.Errorf("moved %d files, want 4", rep.Files)
	}

	if rep.Verified != rep.Files {
		t.Errorf("verified %d of %d files", rep.Verified, rep.Files)
	}

	for _, rel := range []string{".brain-root.json", "brain.sqlite", "brain.conf", "backups/dump.json"} {
		if _, err := os.Stat(filepath.Join(to, rel)); err != nil {
			t.Errorf("%s did not arrive: %v", rel, err)
		}
	}

	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Error("the source was not removed after a verified move")
	}
}

func TestMoveCanKeepTheSource(t *testing.T) {
	from := seed(t)
	to := filepath.Join(t.TempDir(), "copy")

	if _, err := Move(from, to, true, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(from, "brain.sqlite")); err != nil {
		t.Error("the source was removed despite keepSource")
	}
}

// Merging two brains would silently mix two histories.
func TestMoveRefusesADestinationThatAlreadyHoldsABrain(t *testing.T) {
	from := seed(t)
	to := seed(t)

	if _, err := Move(from, to, false, nil); err == nil {
		t.Fatal("moved a brain on top of another one")
	}

	// And the source must be untouched after a refusal.
	if _, err := os.Stat(filepath.Join(from, "brain.sqlite")); err != nil {
		t.Error("the source was damaged by a refused move")
	}
}

// Copying a directory into itself recurses until the disk fills.
func TestMoveRefusesToNestInsideItself(t *testing.T) {
	from := seed(t)

	if _, err := Move(from, filepath.Join(from, "deeper"), false, nil); err == nil {
		t.Fatal("allowed a move into a subdirectory of the source")
	}

	if _, err := Move(from, from, false, nil); err == nil {
		t.Fatal("allowed a move onto itself")
	}
}

// The whole point of copy-verify-delete: a destination that cannot be written
// must leave the original alone.
func TestFailedMoveNeverDeletesTheSource(t *testing.T) {
	from := seed(t)

	parent := t.TempDir()
	blocked := filepath.Join(parent, "readonly")

	if err := os.Mkdir(blocked, 0o555); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { os.Chmod(blocked, 0o755) })

	if os.Geteuid() == 0 {
		t.Skip("running as root; permissions do not apply")
	}

	if _, err := Move(from, filepath.Join(blocked, "brain"), false, nil); err == nil {
		t.Fatal("a move into an unwritable directory reported success")
	}

	for _, rel := range []string{"brain.sqlite", "brain.conf", ".brain-root.json"} {
		if _, err := os.Stat(filepath.Join(from, rel)); err != nil {
			t.Errorf("%s was lost by a failed move: %v", rel, err)
		}
	}
}

func TestMoveReportsProgress(t *testing.T) {
	from := seed(t)
	to := filepath.Join(t.TempDir(), "moved")

	var seen int
	var lastTotal int

	if _, err := Move(from, to, false, func(_ string, done, total int) {
		seen++
		lastTotal = total

		if done > total {
			t.Errorf("progress reported %d of %d", done, total)
		}
	}); err != nil {
		t.Fatal(err)
	}

	if seen != 4 || lastTotal != 4 {
		t.Errorf("progress fired %d times with total %d, want 4 and 4", seen, lastTotal)
	}
}

func TestDrivesExcludePseudoFilesystems(t *testing.T) {
	drives, err := Drives("/")
	if err != nil {
		t.Fatal(err)
	}

	if len(drives) == 0 {
		t.Fatal("no drives found at all")
	}

	for _, d := range drives {
		if pseudoFilesystems[d.Filesystem] {
			t.Errorf("offered a pseudo filesystem: %+v", d)
		}

		// Moving a growing brain onto a 512MB EFI partition would succeed and
		// then immediately fail.
		if d.TotalBytes < MinimumUsableBytes {
			t.Errorf("offered a drive too small to be useful: %+v", d)
		}
	}

	// The drive holding the given root must be marked, or the interface cannot
	// show which one is in use.
	var current int
	for _, d := range drives {
		if d.Current {
			current++
		}
	}

	if current == 0 {
		t.Error("no drive was marked as the current one")
	}
}

// Bind mounts and snap packages put the same filesystem under several paths.
// Offering it twice invites somebody to "move to another drive" that is the
// drive they are already on.
func TestDrivesAreListedOncePerFilesystem(t *testing.T) {
	drives, err := Drives("/")
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}

	for _, d := range drives {
		device := deviceOf(d.MountPoint)

		if previous, clash := seen[device]; clash {
			t.Errorf("filesystem %s listed twice: %s and %s", device, previous, d.MountPoint)
		}

		seen[device] = d.MountPoint
	}
}
