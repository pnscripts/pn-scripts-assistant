package programs

import "os"

// openToRead opens a file to read. Opening never waits here: a named pipe
// with no server refuses at once.
func openToRead(path string) (*os.File, error) {
	return os.Open(path)
}
