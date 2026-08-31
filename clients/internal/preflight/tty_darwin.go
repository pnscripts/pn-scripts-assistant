//go:build darwin

package preflight

import "golang.org/x/sys/unix"

const tcGetAttr = unix.TIOCGETA
