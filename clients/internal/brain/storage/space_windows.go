//go:build windows

package storage

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// spaceOn reports the size and usable free space of the volume holding path.
//
// GetDiskFreeSpaceEx returns free-to-caller separately from total free, and the
// first is what matters: on a volume with quotas they differ, and promising
// space a quota forbids is the same mistake as counting root's reserve on Unix.
func spaceOn(path string) (total, free uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	var freeToCaller, totalBytes, totalFree uint64

	if err := windows.GetDiskFreeSpaceEx(p, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}

	return totalBytes, freeToCaller, nil
}

// deviceOf identifies the volume a path sits on.
//
// The drive letter, which is what "a different drive" means on Windows. There
// is no device number to compare, and the letter is the distinction a person
// actually has in mind.
func deviceOf(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	return strings.ToUpper(filepath.VolumeName(abs))
}
