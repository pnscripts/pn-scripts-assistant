//go:build unix

package storage

import (
	"fmt"
	"syscall"
)

// spaceOn reports the size and usable free space of the filesystem holding path.
//
// Bavail rather than Bfree: the difference is space reserved for root, which
// this process cannot use and so must not be counted as available. Reporting it
// would tell somebody they have room they cannot actually write into.
func spaceOn(path string) (total, free uint64, err error) {
	var fs syscall.Statfs_t

	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0, err
	}

	block := uint64(fs.Bsize)

	return fs.Blocks * block, fs.Bavail * block, nil
}

// deviceOf identifies the filesystem a path sits on, so two mount points on the
// same device are not mistaken for two places to put things.
func deviceOf(path string) string {
	var stat syscall.Stat_t

	if err := syscall.Stat(path, &stat); err != nil {
		return ""
	}

	return fmt.Sprintf("%d", stat.Dev)
}
