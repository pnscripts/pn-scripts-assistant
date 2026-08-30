package speech

import (
	"os"
	"path/filepath"
	"time"
)

/*
 * Clearing up recordings a previous run could not.
 *
 * Every turn writes a temporary file and removes it afterwards, and that works
 * for every turn that finishes. It does not work for the ones interrupted by
 * the program being killed, and over a few days of development that left
 * thirty-eight of them — once, before the system's own cleaner got to them,
 * several gigabytes.
 *
 * Swept at startup rather than left to the operating system, because the
 * operating system's rules are its own: some machines clear /tmp on boot, some
 * after ten days, and some not at all. A program that leaves rubbish behind
 * should not be relying on somebody else's housekeeping to hide it.
 */

// keepStraysFor is how old a leftover must be before it is removed.
//
// Generous, because a file that belongs to a turn happening right now looks
// exactly like a leftover: the same name, the same directory, still being
// written. An hour is far longer than the longest possible turn and far
// shorter than anybody would notice the disk.
const keepStraysFor = time.Hour

// SweepOldRecordings removes recordings left behind by earlier runs, and
// reports how many and how much.
func SweepOldRecordings() (int, int64) {
	var (
		removed int
		freed   int64
	)

	for _, pattern := range []string{"pn-brain-turn-*.wav", "pn-brain-listen-*.wav"} {
		matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
		if err != nil {
			continue
		}

		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			if time.Since(info.ModTime()) < keepStraysFor {
				continue
			}

			if err := os.Remove(path); err != nil {
				continue
			}

			removed++
			freed += info.Size()
		}
	}

	return removed, freed
}
