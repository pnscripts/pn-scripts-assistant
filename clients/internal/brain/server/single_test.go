package server

import (
	"errors"
	"testing"
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
