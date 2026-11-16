//go:build !linux

package programs

import "os"

// changedAt: this system's file information carries no change time the
// same way; the size and the time written are what is followed.
func changedAt(os.FileInfo) int64 { return 0 }
