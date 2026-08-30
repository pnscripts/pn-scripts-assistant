package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

/*
 * One brain at a time.
 *
 * Two copies of this program on one machine are not two windows onto the same
 * thing — they are two processes with the same SQLite file open, two learning
 * workers writing lessons, and two microphone loops both recording the room and
 * both answering it. Nothing about that is what somebody double-clicking an
 * icon a second time is asking for.
 *
 * The port would already refuse the second one, but only after it has printed
 * to a terminal nobody launched it from and left the person looking at a
 * window that never appeared. Held properly, the second copy finds the first,
 * brings its window forward, and exits without saying anything.
 */

// LockName is the file whose lock says a brain is running on this data root.
const LockName = ".brain-running"

// Lock is a held claim on a data root.
type Lock struct {
	file *os.File
}

// ErrAlreadyRunning means another copy holds the data root.
var ErrAlreadyRunning = errors.New("another copy is already running")

/*
 * Claim takes the lock for a data root.
 *
 * The lock is on the data root rather than the port, because the data is what
 * cannot be shared: two brains on one database corrupt each other's learning
 * whatever addresses they listen on. It is an advisory lock held by an open
 * file, so it goes away by itself if the process is killed — there is no stale
 * lock file to explain to anybody, which is the failure mode of the version
 * that writes a pid and asks the next process to interpret it.
 */
func Claim(root string) (*Lock, error) {
	path := filepath.Join(root, LockName)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening the lock: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()

		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}

		return nil, fmt.Errorf("locking %s: %w", path, err)
	}

	return &Lock{file: f}, nil
}

/*
 * ClaimWaiting takes the lock, waiting a little for a copy that is on its way
 * out.
 *
 * A brain that has just been asked to quit still holds its data root for the
 * moment it takes to stop the learning worker, close the database and let go of
 * the microphone. Anything started inside that window — a restart, or somebody
 * who closed the window and immediately pressed the icon again — found the lock
 * held and refused to start, which reads as the program being broken rather
 * than as it being half a second early.
 *
 * Only used once the copy holding the lock has been found not to answer.
 * A healthy one is raised instead, and that path never waits.
 */
func ClaimWaiting(root string, patience time.Duration) (*Lock, error) {
	deadline := time.Now().Add(patience)

	for {
		lock, err := Claim(root)
		if err == nil || !errors.Is(err, ErrAlreadyRunning) {
			return lock, err
		}

		if time.Now().After(deadline) {
			return nil, err
		}

		time.Sleep(150 * time.Millisecond)
	}
}

// Release gives up the claim.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	// Unlocked by closing, which also happens if this process dies.
	return l.file.Close()
}

/*
 * Raise asks the copy already running to come to the front.
 *
 * Returns whether it managed to. A copy that holds the lock but does not answer
 * is starting up, shutting down, or wedged — in which case saying so is more
 * use than silently doing nothing, since the person is looking at an icon they
 * have now clicked twice.
 */
func Raise(addr string) bool {
	client := &http.Client{Timeout: 2 * time.Second}

	res, err := client.Post("http://"+addr+"/api/present", "application/json", nil)
	if err != nil {
		return false
	}

	defer res.Body.Close()

	return res.StatusCode == http.StatusOK
}

// InUse reports whether something is already listening on the address.
//
// Used only to describe what happened, never to decide it: the lock decides.
func InUse(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
