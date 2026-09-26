//go:build linux

package orchestrator

import (
	"strings"
	"syscall"
)

/*
 * Which kernel this is, asked of the kernel.
 *
 * Its own file because uname is not portable: the call, and the array of
 * signed characters it fills in, are Linux's shape. macOS has its own and
 * Windows has none, and this used to be written inline — which is why the
 * program could not be compiled for either of them at all.
 */
func kernelName() string {
	var u syscall.Utsname

	if syscall.Uname(&u) != nil {
		return ""
	}

	return utsString(u.Release)
}

func utsString(chars [65]int8) string {
	var b strings.Builder

	for _, c := range chars {
		if c == 0 {
			break
		}

		b.WriteByte(byte(c))
	}

	return b.String()
}
