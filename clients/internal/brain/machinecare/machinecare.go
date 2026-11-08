/*
 * Package machinecare looks after the computer the assistant runs on, as a
 * careful colleague would: what it left running that has finished its job,
 * whether the last session ended in a power cut and what it was in the
 * middle of, and whether this is a good moment to restart.
 *
 * It looks and says; it never stops, deletes or restarts anything by itself.
 * Stopping a leftover is a tool that asks first, and only a process this
 * package found and named can be stopped that way — never the user's own
 * programs, never another user's, never the system's.
 */
package machinecare

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// SessionName is the file that says a session is running on a data root.
const SessionName = ".session.json"

// session is what the file holds.
type session struct {
	Started time.Time `json:"started"`
	PID     int       `json:"pid"`
	Clean   bool      `json:"clean"`
	Stopped time.Time `json:"stopped,omitempty"`
}

/*
 * LastStop is how the previous session ended.
 *
 * Unclean means the program was not closed — a power cut, a crash, a kill —
 * and LastSeen is the last moment its data shows it working, which is about
 * when it stopped.
 */
type LastStop struct {
	Known    bool      `json:"known"`
	Unclean  bool      `json:"unclean"`
	Started  time.Time `json:"started,omitempty"`
	LastSeen time.Time `json:"last_seen,omitempty"`
}

/*
 * Begin starts a session on root and says how the last one ended.
 *
 * The file is written when a session starts and marked clean when it stops
 * properly (End). Found at the next start still unmarked, the stop was not a
 * proper one. When it was, the last time anything in the data root was
 * written — the database's journal, the log — is when it was last seen.
 */
func Begin(root string, now time.Time) (LastStop, error) {
	path := filepath.Join(root, SessionName)

	var last LastStop

	if raw, err := os.ReadFile(path); err == nil {
		var was session

		if json.Unmarshal(raw, &was) == nil && !was.Started.IsZero() {
			last = LastStop{Known: true, Unclean: !was.Clean, Started: was.Started}

			if last.Unclean {
				last.LastSeen = lastWritten(root, was.Started)
			} else {
				last.LastSeen = was.Stopped
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return LastStop{}, err
	}

	return last, write(path, session{Started: now.UTC(), PID: os.Getpid()})
}

// End marks the session stopped properly.
func End(root string, now time.Time) error {
	path := filepath.Join(root, SessionName)

	var s session

	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &s)
	}

	if s.PID != 0 && s.PID != os.Getpid() {
		// Another copy's session; never claim it was closed.
		return nil
	}

	s.Clean, s.Stopped = true, now.UTC()

	return write(path, s)
}

func write(path string, s session) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// lastWritten is the latest time any file directly in root was changed
// after since — the database and its journal, the log.
func lastWritten(root string, since time.Time) time.Time {
	latest := since

	entries, err := os.ReadDir(root)
	if err != nil {
		return latest
	}

	for _, e := range entries {
		if e.IsDir() || e.Name() == SessionName {
			continue
		}

		if info, err := e.Info(); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}

	return latest.UTC()
}
