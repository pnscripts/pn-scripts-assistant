//go:build !windows

package orchestrator

import (
	"path/filepath"
	"syscall"
)

/*
 * How much room is left where something lives.
 *
 * Asked of the filesystem holding the path, walking up until a folder that
 * exists answers: a brain on a drive that is not plugged in has a path whose
 * parent is still there, and "no idea" is a worse answer than the disk the
 * folder would be on.
 *
 * Its own file because statfs is a Unix call. Windows has its own and it is
 * in free_windows.go; they were written inline once, and the program could
 * not be compiled for Windows at all.
 */
func freeAt(path string) (int64, bool) {
	for p := path; p != "" && p != "/"; p = filepath.Dir(p) {
		var st syscall.Statfs_t

		if syscall.Statfs(p, &st) == nil {
			return int64(st.Bavail) * int64(st.Bsize), true
		}
	}

	var st syscall.Statfs_t
	if syscall.Statfs("/", &st) == nil {
		return int64(st.Bavail) * int64(st.Bsize), true
	}

	return 0, false
}
