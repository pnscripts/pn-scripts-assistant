package server

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// sameDisk reports whether the brain and the home folder are on one volume.
// Windows has no device number in a stat: the volume a path is on is the
// mount point GetVolumePathName names (C:\, or a folder a volume is mounted
// at), compared without case as Windows compares paths. Anything it cannot
// read counts as one volume, as on the other systems.
func sameDisk(t *testing.T, root string) bool {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}

	a, okA := volumeOf(root)
	b, okB := volumeOf(home)

	if !okA || !okB {
		return true
	}

	return strings.EqualFold(a, b)
}

func volumeOf(path string) (string, bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}

	buf := make([]uint16, windows.MAX_PATH+1)

	if windows.GetVolumePathName(p, &buf[0], uint32(len(buf))) != nil {
		return "", false
	}

	return windows.UTF16ToString(buf), true
}
