//go:build !windows

package server

import (
	"errors"
	"os"
	"syscall"
)

/*
 * lockFile takes an exclusive lock that the kernel drops by itself.
 *
 * flock is tied to the open file, so a brain that is killed outright releases
 * it without anything having to notice — which is the whole reason for a lock
 * rather than a pid file somebody has to interpret after a crash.
 */
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)

	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrAlreadyRunning
	}

	return err
}
