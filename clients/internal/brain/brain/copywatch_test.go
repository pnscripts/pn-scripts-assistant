package brain

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pn-brain/internal/brain/copies"
	"pn-brain/internal/brain/store"
)

func withACopy(t *testing.T) (*Brain, string) {
	t.Helper()

	root := t.TempDir()

	os.WriteFile(filepath.Join(root, ".brain-root.json"), []byte(`{"id":"one"}`), 0o644)

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	if _, err := db.AddFact("test", "something worth keeping", nil); err != nil {
		t.Fatal(err)
	}

	b := &Brain{
		Root: root,
		DB:   db,
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	backup := filepath.Join(t.TempDir(), "backup")

	if err := copies.Keep(root, backup); err != nil {
		t.Fatal(err)
	}

	return b, backup
}

/*
 * The copies are made without being asked, and only when there is a reason.
 *
 * Both halves matter. A backup somebody has to remember to make is a backup
 * from whenever they last remembered — and a backup that rewrites gigabytes
 * every ten minutes on a brain that has been sitting idle is a good way to
 * wear out the drive it is protecting.
 */
func TestCopiesAreRefreshedOnlyWhenSomethingHasChanged(t *testing.T) {
	b, backup := withACopy(t)

	b.CopyNowIfDue(context.Background())

	first, err := os.Stat(filepath.Join(backup, copies.Marker))
	if err != nil {
		t.Fatalf("nothing was copied: %v", err)
	}

	// Nothing has happened since, so nothing should be written again.
	b.CopyNowIfDue(context.Background())

	again, err := os.Stat(filepath.Join(backup, copies.Marker))
	if err != nil {
		t.Fatal(err)
	}

	if !again.ModTime().Equal(first.ModTime()) {
		t.Error("an idle brain rewrote its backup for nothing")
	}

	// And now it learns something.
	if _, err := b.DB.AddFact("test", "learned after the copy", nil); err != nil {
		t.Fatal(err)
	}

	// The comparison is between file times, which on some filesystems have a
	// resolution of a second.
	os.Chtimes(filepath.Join(backup, copies.Marker),
		time.Now().Add(-time.Minute), time.Now().Add(-time.Minute))

	b.CopyNowIfDue(context.Background())

	after, err := os.Stat(filepath.Join(backup, copies.Marker))
	if err != nil {
		t.Fatal(err)
	}

	if !after.ModTime().After(first.ModTime().Add(-time.Second)) {
		t.Error("the brain learned something and the copy was left behind")
	}

	held, err := store.Open(filepath.Join(backup, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if n, _ := held.CountFacts(); n != 2 {
		t.Errorf("the refreshed copy holds %d facts; the brain knows 2", n)
	}
}

/*
 * And nothing is copied out of a brain whose own drive has gone.
 *
 * The database is still open at that point and still answering — that is the
 * whole trap of an unplugged drive — so a copy taken then would be a copy of
 * whatever state the file was in when the disk left, written over a good
 * backup from before.
 */
func TestNothingIsCopiedWhileTheDriveIsGone(t *testing.T) {
	b, backup := withACopy(t)

	b.drive.mu.Lock()
	b.drive.gone = true
	b.drive.mu.Unlock()

	b.CopyNowIfDue(context.Background())

	if _, err := os.Stat(filepath.Join(backup, copies.Marker)); err == nil {
		t.Error("a copy was taken from a brain whose drive had gone")
	}
}
