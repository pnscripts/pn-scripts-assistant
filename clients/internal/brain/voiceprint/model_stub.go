//go:build !cgo

package voiceprint

import (
	"fmt"
	"os"
	"path/filepath"
)

/*
 * Telling voices apart needs a model, and running a model needs cgo.
 *
 * The desktop builds for macOS and Windows are compiled without it — they have
 * no native window layer to link against there yet, so the whole binary is
 * built the portable way. Rather than making those builds impossible, this
 * says plainly that the machine cannot do it.
 *
 * Which is the same shape as everything else here: a capability the machine
 * lacks is reported as absent, not faked.
 */

// Where the parts would live, so the interface can name the folder even where
// nothing can use it.
func Parts() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "pn-brain", FolderName)
	}

	return filepath.Join(home, ".local", "share", "pn-brain", FolderName)
}

// Folder is where this brain would keep the voice it knows.
func Folder(root string) string { return filepath.Join(root, FolderName) }

// Installed is always false: this build cannot run the model whatever is on
// the disk.
func Installed() bool { return false }

// Of cannot measure a voice in this build, and says so rather than returning
// numbers that mean nothing.
func Of(samples []float64) ([]float32, error) {
	return nil, fmt.Errorf(
		"this build cannot tell voices apart: it was compiled without the part that runs the model")
}
