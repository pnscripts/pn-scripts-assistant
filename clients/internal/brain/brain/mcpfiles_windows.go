package brain

import "os"

// openWithoutWaiting opens a file to read. Opening never waits here: a
// named pipe with no server refuses at once, and the file is then
// measured and refused as not a regular one like on Linux.
func openWithoutWaiting(path string) (*os.File, error) {
	return os.Open(path)
}
