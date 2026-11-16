//go:build unix

package askclaude

import "syscall"

// noFollow makes opening a file fail when its last part is a link.
const noFollow = syscall.O_NOFOLLOW
