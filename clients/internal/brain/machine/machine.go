// Package machine reports what the computer is doing.
//
// The brain runs a language model on this machine's own processor, which is by
// far the slowest part of talking to it. When an answer takes a minute, the
// difference between "it is working hard" and "it is stuck" is the single most
// useful thing the interface can say, and it can only say it by looking.
//
// Everything here is measured. Where a figure cannot be obtained on the
// platform it is reported as unknown rather than guessed, because a load
// reading that is sometimes invented is worse than no load reading: it is not
// possible to tell which kind you are looking at.
package machine

// Load is what the computer is doing right now.
type Load struct {
	// CPUPercent is how busy the processors are, 0 to 100. Negative when it
	// could not be measured.
	CPUPercent float64 `json:"cpu_percent"`
	// Cores is how many processors there are, or zero if unknown.
	Cores int `json:"cores"`

	// MemoryUsedBytes and MemoryTotalBytes describe physical memory. Zero
	// total means it could not be measured.
	MemoryUsedBytes  uint64 `json:"memory_used_bytes"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`

	// GPUPercent is how hard the graphics card is working, 0 to 100, or
	// negative when it cannot be read. What "working" means differs by vendor:
	// AMD reports a real busy figure, Intel publishes a clock speed and this is
	// how close it is running to its maximum. Both answer "is the card doing
	// anything", which is the question.
	GPUPercent float64 `json:"gpu_percent"`
	GPUName    string  `json:"gpu_name"`
	GPUKnown   bool    `json:"gpu_known"`

	// Available is false when this platform cannot report any of it.
	Available bool `json:"available"`
}

// MemoryPercent is how full memory is, or -1 when it is not known.
func (l Load) MemoryPercent() float64 {
	if l.MemoryTotalBytes == 0 {
		return -1
	}

	return float64(l.MemoryUsedBytes) / float64(l.MemoryTotalBytes) * 100
}

// Current reads the machine's load.
func Current() Load { return current() }
