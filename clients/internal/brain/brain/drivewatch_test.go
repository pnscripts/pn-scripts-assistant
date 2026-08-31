package brain

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * The marker, not the folder.
 *
 * Unmounting a drive usually leaves its mount point behind as an empty
 * directory, so a brain that checked for the directory would go on believing
 * everything was fine while every write failed.
 */
func TestAnEmptyMountPointIsNotADrive(t *testing.T) {
	root := t.TempDir()
	b := &Brain{Root: root}

	if b.rootIsThere() {
		t.Error("an empty folder was accepted as the brain's drive")
	}

	marker := filepath.Join(root, ".brain-root.json")

	if err := os.WriteFile(marker, []byte(`{"id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if !b.rootIsThere() {
		t.Error("a real root was not recognised")
	}

	// And going away is noticed.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	if b.rootIsThere() {
		t.Error("the drive going away was not noticed")
	}
}

// A brain with no root set is not a brain on a missing drive — the check must
// not report a problem that does not exist.
func TestNoRootIsNotAMissingDrive(t *testing.T) {
	b := &Brain{}

	if !b.rootIsThere() {
		t.Error("a brain with no root was reported as having lost its drive")
	}
}
