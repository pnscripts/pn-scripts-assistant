//go:build !windows

package programs

import (
	"os"
	"syscall"
)

// openToRead opens a file to read without waiting: a pipe put where a
// menu entry, a service file or a script was expected opens at once and
// is then refused as not a regular file, instead of holding whoever looks
// until something writes to it. On a regular file the flag changes nothing.
func openToRead(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
