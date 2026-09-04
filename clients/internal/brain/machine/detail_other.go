//go:build !linux

package machine

import "time"

// detail reports nothing rather than guessing.
//
// Every figure in the operations view comes from the Linux kernel's /proc,
// which no other system publishes in the same form. Inventing a plausible
// number for a wall of them is how the whole wall stops being trustworthy.
func detail() Detail {
	return Detail{At: time.Now(), Available: false, Overall: -1}
}
