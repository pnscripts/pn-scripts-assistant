package server

import (
	"errors"
	"testing"
	"time"
)

/*
 * One brain at a time, on the data root rather than the port.
 *
 * The database is what cannot be shared: two copies mean two learning workers
 * writing lessons and two microphone loops both recording the room and both
 * answering it. The port would refuse the second one too, but only after it had
 * printed to a terminal nobody launched it from.
 */
func TestOnlyOneCopyHoldsADataRoot(t *testing.T) {
	root := t.TempDir()

	first, err := Claim(root)
	if err != nil {
		t.Fatalf("the first copy could not claim an empty root: %v", err)
	}

	if _, err := Claim(root); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("a second copy claimed a root that was already held: %v", err)
	}

	// A different root is a different brain and is none of its business.
	other, err := Claim(t.TempDir())
	if err != nil {
		t.Fatalf("a second brain on its own data was refused: %v", err)
	}

	other.Release()

	// And once the first lets go, the next copy may start.
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}

	again, err := Claim(root)
	if err != nil {
		t.Fatalf("the root stayed locked after the copy holding it released: %v", err)
	}

	again.Release()
}

// The lock goes away with the process that held it.
//
// Held by an open file rather than written as a pid, so a brain that is killed
// leaves nothing behind to explain to anybody — which is the failure mode of
// every lock file that has to be interpreted by whatever starts next.
func TestAKilledCopyLeavesNothingToClean(t *testing.T) {
	root := t.TempDir()

	held, err := Claim(root)
	if err != nil {
		t.Fatal(err)
	}

	// Closing the file is what a dying process does to its descriptors.
	held.file.Close()

	next, err := Claim(root)
	if err != nil {
		t.Fatalf("the root was still locked by a copy that is gone: %v", err)
	}

	next.Release()
}

/*
 * A copy on its way out still holds the root for a moment.
 *
 * Stopping means letting the learning worker finish, closing the database and
 * letting go of the microphone, and the lock is held throughout. Anything
 * started inside that window — a restart, or somebody who closed the window and
 * pressed the icon again straight away — found the root held and refused to
 * start, which reads as the program being broken rather than half a second
 * early. It happened while testing the change that introduced the lock.
 */
func TestItWaitsForACopyThatIsQuitting(t *testing.T) {
	root := t.TempDir()

	quitting, err := Claim(root)
	if err != nil {
		t.Fatal(err)
	}

	// Let go shortly, the way a brain finishing its shutdown does.
	go func() {
		time.Sleep(250 * time.Millisecond)
		quitting.Release()
	}()

	start := time.Now()

	lock, err := ClaimWaiting(root, 5*time.Second)
	if err != nil {
		t.Fatalf("it gave up on a root that was let go after a moment: %v", err)
	}

	defer lock.Release()

	if time.Since(start) < 200*time.Millisecond {
		t.Error("it claimed a root that was still held")
	}
}

// But it does not wait forever for one that is staying.
func TestItGivesUpOnACopyThatIsStaying(t *testing.T) {
	root := t.TempDir()

	staying, err := Claim(root)
	if err != nil {
		t.Fatal(err)
	}

	defer staying.Release()

	if _, err := ClaimWaiting(root, 300*time.Millisecond); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("it took a root that another copy is still holding: %v", err)
	}
}
