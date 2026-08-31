package paths

import (
	"path/filepath"
	"runtime"
	"testing"
)

// The drive has to be found on whatever system it is plugged into, because
// carrying it to another computer is the only reason to keep the brain on an
// external drive at all. Searching only the Linux mount points made it
// invisible on macOS and Windows — where nothing failed, it just quietly
// started a new empty brain beside the full one.
func TestEverySystemLooksWhereItsDrivesAppear(t *testing.T) {
	if len(removableMounts()) == 0 && runtime.GOOS == "linux" {
		// No drive plugged in is a fine answer; an empty list on a machine
		// with one mounted would not be.
		t.Log("no removable drives mounted here")
	}

	// The promise that matters: a root is located by its marker, wherever the
	// system happened to mount it and under whichever user's name.
	drive := t.TempDir()
	root := filepath.Join(drive, "PN-BRAIN-DATA")

	if _, err := Create(root); err != nil {
		t.Fatalf("creating a root on a pretend drive: %v", err)
	}

	t.Setenv("PN_BRAIN_SEARCH_PATHS", drive)
	t.Setenv("PN_BRAIN_DATA_ROOT", "")

	found, err := Find()
	if err != nil {
		t.Fatalf("a root on a drive was not found: %v", err)
	}

	if found.Path != root {
		t.Errorf("found %q, want %q", found.Path, root)
	}
}
