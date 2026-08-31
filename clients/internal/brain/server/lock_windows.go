//go:build windows

package server

import (
	"os"

	"golang.org/x/sys/windows"
)

/*
 * The same guarantee, by the name Windows gives it.
 *
 * LockFileEx with EXCLUSIVE and FAIL_IMMEDIATELY is flock's non-blocking
 * exclusive lock: held against the open handle, and released by the operating
 * system when the process ends however it ends. Without this the whole program
 * did not build for Windows, so one brain at a time was enforced there by the
 * program being unavailable.
 */
func lockFile(f *os.File) error {
	var overlapped windows.Overlapped

	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped,
	)

	// The lock being held by another copy is the expected answer, not a fault.
	if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
		return ErrAlreadyRunning
	}

	return err
}
