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

/*
 * Opening a brain once must not change which brain this machine uses.
 *
 * PN_BRAIN_DATA_ROOT names a root for one run — that is what the tests use,
 * and what anybody wanting to look at a second brain would use. But
 * FindOrCreate writes down whatever it found, so a single run with the
 * variable set replaced the machine's answer with a temporary folder.
 *
 * The symptom is the worst kind. Nothing failed and nothing was damaged: the
 * program simply opened afterwards as an empty brain, asked for a name as
 * though it had never been run before, and introduced itself — while
 * everything the real one knew sat unread on the drive it was no longer
 * looking at.
 */
func TestABorrowedRootDoesNotBecomeTheMachinesOwn(t *testing.T) {
	real := mounts
	mounts = func() []string { return nil }

	t.Cleanup(func() { mounts = real })

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("PN_BRAIN_SEARCH_PATHS", "")

	drive := t.TempDir()
	mine := filepath.Join(drive, "PN-BRAIN-DATA")

	if _, err := Create(mine); err != nil {
		t.Fatal(err)
	}

	// The brain this machine uses, chosen the way somebody chooses one.
	if _, err := Choose(mine); err != nil {
		t.Fatal(err)
	}

	// And now one run against somewhere else entirely.
	scratch := filepath.Join(t.TempDir(), "scratch")

	if _, err := Create(scratch); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PN_BRAIN_DATA_ROOT", scratch)

	borrowed, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if borrowed.Path != scratch {
		t.Fatalf("the run did not use the root it was given: %s", borrowed.Path)
	}

	// The run is over. The machine must be where it was.
	t.Setenv("PN_BRAIN_DATA_ROOT", "")

	after, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if after.Path != mine {
		t.Errorf("this machine now opens %s; it should still open %s", after.Path, mine)
	}

	if remembered, _ := LastKnown(); remembered != mine {
		t.Errorf("the remembered root is %s, not %s", remembered, mine)
	}
}

/*
 * A drive full of copies is not a drive full of brains.
 *
 * The search walks every mounted drive looking for a marker, so a backup
 * carrying the brain's own marker would be started as the brain by any machine
 * that had the backup attached and its own drive missing — and once a day of
 * conversation has gone into it there are two brains, both real, growing
 * apart, with no honest way back to one.
 *
 * Copies are marked with a different file for exactly this reason, and this is
 * the test that says so from the searching end.
 */
func TestACopyOnADriveIsNotFoundAsTheBrain(t *testing.T) {
	real := mounts
	mounts = func() []string { return nil }

	t.Cleanup(func() { mounts = real })

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("PN_BRAIN_DATA_ROOT", "")

	drive := t.TempDir()
	backup := filepath.Join(drive, "PN-BRAIN-COPY")

	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}

	// Everything a copy has: the memory, the settings, and its own marker.
	os.WriteFile(filepath.Join(backup, "brain.sqlite"), []byte("not really a database"), 0o644)
	os.WriteFile(filepath.Join(backup, "brain.conf"), []byte("BRAIN_NAME=Ariel\n"), 0o600)
	os.WriteFile(filepath.Join(backup, ".brain-copy.json"), []byte(`{"brain_id":"one"}`), 0o644)

	t.Setenv("PN_BRAIN_SEARCH_PATHS", drive)

	if found, err := Find(); err == nil {
		t.Errorf("a copy at %s was found and would have been opened as the brain", found.Path)
	} else if !errors.Is(err, ErrNotFound) {
		t.Errorf("looking at a drive holding only a copy: %v", err)
	}
}

/*
 * The same brain, found somewhere else, is a journey rather than a new brain.
 *
 * A drive carried to another machine mounts at a different path — and every
 * memory that names a file names it by the old one. Noticing that is the only
 * chance anybody gets: after the pointer is rewritten there is nothing left to
 * compare against, and the brain looks entirely normal while a large part of
 * what it knows points nowhere.
 */
func TestTheSameBrainFoundElsewhereIsNoticed(t *testing.T) {
	real := mounts
	mounts = func() []string { return nil }

	t.Cleanup(func() { mounts = real })

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("PN_BRAIN_DATA_ROOT", "")

	// Where it was, on the machine it came from.
	before := filepath.Join(t.TempDir(), "OLDMOUNT", "PN-BRAIN-DATA")

	was, err := Create(before)
	if err != nil {
		t.Fatal(err)
	}

	Remember(was)

	// The same brain — same marker, same id — mounted somewhere else.
	after := filepath.Join(t.TempDir(), "NEWMOUNT", "PN-BRAIN-DATA")

	if err := os.MkdirAll(after, 0o755); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(before, Marker))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(after, Marker), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	// And the old place is gone, as it is when the drive is on another desk.
	os.RemoveAll(filepath.Dir(before))

	t.Setenv("PN_BRAIN_SEARCH_PATHS", filepath.Dir(after))

	found, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if found.Path != after {
		t.Fatalf("it opened %s rather than the drive that is here", found.Path)
	}

	if found.MovedFrom != before {
		t.Errorf("the journey was not noticed: MovedFrom is %q, want %q", found.MovedFrom, before)
	}

	// And opening it again from the same place is not a journey.
	next, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if next.MovedFrom != "" {
		t.Errorf("opening it where it already was reported a journey from %q", next.MovedFrom)
	}
}

/*
 * Renaming the product must not orphan anybody's brain.
 *
 * The folder used to be called PN-BRAIN-DATA because the program was called PN
 * Brain. It is PN-SCRIPTS-ASSISTANT-DATA now, and a machine with the old one
 * has to keep working — the alternative is somebody opening their assistant
 * after an update and being told it has never met them.
 */
func TestAnOlderDataFolderIsStillFound(t *testing.T) {
	drive := t.TempDir()

	old := filepath.Join(drive, LegacyData)
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, Marker), []byte(`{"id":"older","schema":1}`), 0o644)

	found := DataFolderIn(drive)

	if len(found) != 2 {
		t.Fatalf("looked in %d places, want the new name and the old", len(found))
	}

	// Newest first, so a machine carrying both prefers the current one.
	if filepath.Base(found[0]) != DataFolder {
		t.Errorf("looks for %q first, want %q", filepath.Base(found[0]), DataFolder)
	}

	if filepath.Base(found[1]) != LegacyData {
		t.Errorf("does not look for the old name at all: %v", found)
	}
}
