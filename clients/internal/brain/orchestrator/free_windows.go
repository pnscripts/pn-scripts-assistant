//go:build windows

package orchestrator

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

/*
 * How much room is left where something lives, asked the way Windows asks.
 *
 * GetDiskFreeSpaceEx rather than statfs, and the number wanted is the one
 * available to this user rather than the total free — a disk with a quota on
 * it has two different answers and only one of them is room this program can
 * use.
 *
 * Walking up the same way the Unix one does: the folder may not exist yet,
 * and the disk it would be on is a better answer than none.
 */
func freeAt(path string) (int64, bool) {
	for p := path; p != ""; {
		var free, total, totalFree uint64

		name, err := windows.UTF16PtrFromString(p)
		if err == nil && windows.GetDiskFreeSpaceEx(name, &free, &total, &totalFree) == nil {
			return int64(free), true
		}

		parent := filepath.Dir(p)
		if parent == p {
			break
		}

		p = parent
	}

	return 0, false
}
