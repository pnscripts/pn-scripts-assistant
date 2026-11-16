package programs

import (
	"os"
	"syscall"
)

// changedAt is when a file last changed in any way (its inode's change
// time): unlike the time it was written, a program cannot set it back.
func changedAt(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ctim.Sec*1e9 + st.Ctim.Nsec
	}

	return 0
}
