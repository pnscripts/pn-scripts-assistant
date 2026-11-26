//go:build !windows

package programs

import "syscall"

func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }
