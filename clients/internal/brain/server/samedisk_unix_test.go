//go:build !windows

package server

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// sameDisk reports whether the brain and the home folder are on one
// filesystem, which is the ordinary shape of a laptop and of a build runner.
// Anything it cannot read counts as one filesystem: the caller then skips
// rather than failing on a question it could not ask.
func sameDisk(t *testing.T, root string) bool {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}

	var a, b unix.Stat_t

	if unix.Stat(root, &a) != nil || unix.Stat(home, &b) != nil {
		return true
	}

	return a.Dev == b.Dev
}
