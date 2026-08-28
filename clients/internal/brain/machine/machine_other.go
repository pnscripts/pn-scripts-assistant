//go:build !linux

package machine

import "runtime"

// current reports what it can without a /proc filesystem.
//
// Processor and memory use are left unknown rather than approximated. There is
// no machine of these kinds in this project's development environment, so any
// implementation here would be written blind, and a figure written blind is
// exactly the sort of thing this interface refuses to display.
func current() Load {
	return Load{
		CPUPercent: -1,
		GPUPercent: -1,
		Cores:      runtime.NumCPU(),
		Available:  false,
	}
}
