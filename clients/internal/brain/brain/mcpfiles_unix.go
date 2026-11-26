//go:build !windows

package brain

import (
	"os"
	"syscall"
)

// openWithoutWaiting opens a file to read without waiting for anything:
// a pipe nobody writes to opens at once (and is then refused as not a
// file). On a regular file the flag changes nothing.
func openWithoutWaiting(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
