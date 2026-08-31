package paths

import (
	"errors"
	"os"
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

	// Isolated from whatever this machine has chosen for itself: a remembered
	// root now outranks the search, so the real one would answer instead.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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

/*
 * A drive that is not plugged in must not look like a machine that has never
 * seen this program.
 *
 * They produced the same answer from Find — nothing — and the same response:
 * make a new brain. So starting once without the drive replaced everything
 * somebody had with an empty brain that introduced itself, and the only
 * evidence of what happened was that it had forgotten them.
 */
func TestAMissingDriveIsNotAFirstRun(t *testing.T) {
	drive := t.TempDir()
	root := filepath.Join(drive, "PN-BRAIN-DATA")

	// This machine's own drives are not part of the story being told.
	real := mounts
	mounts = func() []string { return nil }

	t.Cleanup(func() { mounts = real })

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("PN_BRAIN_DATA_ROOT", "")
	t.Setenv("PN_BRAIN_SEARCH_PATHS", drive)

	if _, err := Create(root); err != nil {
		t.Fatalf("creating the brain on the drive: %v", err)
	}

	// Used once while the drive is there, which is what makes it remembered.
	found, err := FindOrCreate()
	if err != nil {
		t.Fatalf("with the drive plugged in: %v", err)
	}

	if found.Path != root {
		t.Fatalf("found %q, want %q", found.Path, root)
	}

	// Unplugged.
	if err := os.RemoveAll(drive); err != nil {
		t.Fatalf("removing the pretend drive: %v", err)
	}

	_, err = FindOrCreate()

	var away *AwayError
	if !errors.As(err, &away) {
		t.Fatalf("a missing drive gave %v, want an AwayError naming it", err)
	}

	if away.Path != root {
		t.Errorf("it points at %q, want %q", away.Path, root)
	}

	// And somebody who decides it is gone can move on.
	Forget()

	fresh, err := FindOrCreate()
	if err != nil {
		t.Fatalf("after forgetting: %v", err)
	}

	if fresh.Path == root {
		t.Errorf("it went back to the drive that is gone")
	}
}

/*
 * Choosing a drive has to beat the search order.
 *
 * The search looks in the home folder before any drive, so on a machine that
 * had run this before, picking a drive created the folder there and then never
 * opened it — two brains, with the explicitly chosen one silently losing to
 * the one that happened to be looked at first.
 */
func TestTheChosenPlaceWinsOverTheNearestOne(t *testing.T) {
	real := mounts
	mounts = func() []string { return nil }

	t.Cleanup(func() { mounts = real })

	home := t.TempDir()
	drive := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("PN_BRAIN_DATA_ROOT", "")

	// A brain already in the home folder, as on any machine that has run this.
	nearest := filepath.Join(home, ".local", "share", "pn-brain")
	if _, err := Create(nearest); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PN_BRAIN_SEARCH_PATHS", nearest+string(os.PathListSeparator)+drive)

	// Without a choice, the nearest one is correct.
	found, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if found.Path != nearest {
		t.Fatalf("found %q, want the home one at %q", found.Path, nearest)
	}

	// Now somebody chooses the drive.
	chosen := filepath.Join(drive, "PN-BRAIN-DATA")
	if _, err := Choose(chosen); err != nil {
		t.Fatal(err)
	}

	found, err = FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if found.Path != chosen {
		t.Errorf("after choosing %q it still opened %q", chosen, found.Path)
	}
}
