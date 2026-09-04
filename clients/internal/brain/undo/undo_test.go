package undo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
 * "Put it back" used to be the one word in the vocabulary that meant nothing.
 *
 * The brain could be stopped mid-answer, the last exchange forgotten and a
 * whole conversation thrown away — and a file it had written over was simply
 * gone, because nothing took a copy first.
 */
func TestWhatWasThereBeforeIsPutBack(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(t.TempDir(), "notes.md")

	if err := os.WriteFile(work, []byte("the original"), 0o644); err != nil {
		t.Fatal(err)
	}

	Keep(root, work, "written")

	if err := os.WriteFile(work, []byte("the replacement"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := PutBack(root, ""); err != nil {
		t.Fatal(err)
	}

	back, _ := os.ReadFile(work)

	if string(back) != "the original" {
		t.Errorf("the file reads %q", back)
	}

	/*
	 * And putting it back is itself undoable.
	 *
	 * Somebody who restores the wrong version has not lost the work they
	 * restored over — which is the difference between an undo and a second
	 * mistake.
	 */
	if _, err := PutBack(root, ""); err != nil {
		t.Fatal(err)
	}

	again, _ := os.ReadFile(work)

	if string(again) != "the replacement" {
		t.Errorf("undoing the undo left %q", again)
	}
}

/*
 * A file the brain created is put back by removing it.
 *
 * The likelier of the two cases — "no, I did not want that file" — and one
 * that a store of previous versions has no previous version for.
 */
func TestAFileItCreatedIsPutBackByRemovingIt(t *testing.T) {
	root := t.TempDir()
	fresh := filepath.Join(t.TempDir(), "invented.txt")

	Keep(root, fresh, "written")

	if err := os.WriteFile(fresh, []byte("something new"), 0o644); err != nil {
		t.Fatal(err)
	}

	done, err := PutBack(root, "")
	if err != nil {
		t.Fatal(err)
	}

	if !done.New {
		t.Error("it was not recorded as a file that did not exist before")
	}

	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Errorf("the invented file is still there: %v", err)
	}
}

// The most recent change to one particular file, when several have been
// changed since — "put the config back" means that file, not the last one.
func TestPuttingOneNamedFileBack(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()

	first := filepath.Join(dir, "one.txt")
	second := filepath.Join(dir, "two.txt")

	os.WriteFile(first, []byte("first original"), 0o644)
	os.WriteFile(second, []byte("second original"), 0o644)

	Keep(root, first, "edited")
	os.WriteFile(first, []byte("first changed"), 0o644)

	Keep(root, second, "edited")
	os.WriteFile(second, []byte("second changed"), 0o644)

	change, found := Latest(root, first)

	if !found {
		t.Fatal("the change to the first file was not found")
	}

	if _, err := PutBack(root, change.ID); err != nil {
		t.Fatal(err)
	}

	back, _ := os.ReadFile(first)

	if string(back) != "first original" {
		t.Errorf("the named file reads %q", back)
	}

	untouched, _ := os.ReadFile(second)

	if string(untouched) != "second changed" {
		t.Errorf("putting one file back changed another: %q", untouched)
	}
}

/*
 * Bounded, so the safety net does not become the thing filling the disk.
 *
 * This is a way back from the last hour of a conversation, not a version
 * control system — and the copies are deleted with the record, or the folder
 * would keep growing with the index cleared.
 */
func TestOldChangesAreForgottenAndTheirCopiesDeleted(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(t.TempDir(), "busy.txt")

	os.WriteFile(work, []byte("x"), 0o644)

	for i := 0; i < HowMany+5; i++ {
		Keep(root, work, "written")
	}

	if kept := List(root); len(kept) != HowMany {
		t.Errorf("kept %d changes, the limit is %d", len(kept), HowMany)
	}

	copies, _ := os.ReadDir(filepath.Join(root, FolderName))

	if len(copies) > HowMany {
		t.Errorf("%d copies on disk for %d changes", len(copies), HowMany)
	}

	// And age, not only count.
	old := List(root)[0]
	old.At = time.Now().Add(-HowLong - time.Hour)

	save(root, tidy(root, []Change{old}))

	if left := List(root); len(left) != 0 {
		t.Errorf("a change from beyond the limit was kept: %+v", left)
	}
}
