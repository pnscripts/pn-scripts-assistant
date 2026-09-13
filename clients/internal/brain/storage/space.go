package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

/*
 * How much room there is, and how much a thing takes up.
 *
 * Both questions are asked at the same moment and were answerable by neither
 * the setup screen nor the panel that installs things: the page offered to
 * download nine gigabytes onto a disk whose free space it knew and did not
 * say. "Install" is a different decision on a disk with 200GB free and one
 * with four.
 */

// SpaceOn reports the size and usable free space of the filesystem holding
// path, walking up to the nearest directory that exists — the answer wanted is
// usually about a folder that has not been created yet.
func SpaceOn(path string) (total, free uint64, err error) {
	at := path

	for {
		if _, err := os.Stat(at); err == nil {
			break
		}

		parent := filepath.Dir(at)
		if parent == at {
			return 0, 0, fmt.Errorf("no part of %s exists", path)
		}

		at = parent
	}

	return spaceOn(at)
}

/*
 * SizeOf is how much disk a path actually occupies.
 *
 * Measured rather than declared, because the two differ enough to matter: the
 * recogniser is a 30MB download that becomes half a gigabyte of built objects,
 * and somebody deciding what to delete to reclaim space needs the second
 * number. Symbolic links are counted as themselves rather than followed, so a
 * link into a directory being measured cannot count it twice.
 */
func SizeOf(path string) int64 {
	var total int64

	filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			// Unreadable corners are skipped rather than failing the whole
			// measurement: a number that is slightly low is far more useful
			// than no number.
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if info, err := d.Info(); err == nil {
			total += info.Size()
		}

		return nil
	})

	return total
}

// InWords says a size the way a person would, at the precision they would.
func InWords(bytes int64) string {
	switch {
	case bytes <= 0:
		return ""
	case bytes < 1<<20:
		return fmt.Sprintf("%dKB", bytes/(1<<10))
	case bytes < 1<<30:
		return fmt.Sprintf("%dMB", bytes/(1<<20))
	default:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(1<<30))
	}
}
