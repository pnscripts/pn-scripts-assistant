package machine

import (
	"testing"
	"time"
)

// The processor counters are read from /proc, so these tests live beside the
// Linux code they test: on the other systems the names they use do not exist,
// and a skip inside the test still left the file unable to compile there.

// The first reading cannot be a percentage, and the second must be.
//
// Processor use only exists between two samples of the kernel's counters. The
// tempting shortcut is to report busy-since-boot on the first call, which is a
// real number that answers a question nobody asked — on a machine up for a week
// it would show a calm average no matter what is happening now.
func TestCPUNeedsTwoReadings(t *testing.T) {
	// Whatever earlier tests left behind.
	cpu.mu.Lock()
	cpu.last = cpuSample{}
	cpu.mu.Unlock()
	lastCPUPercent = -1

	if got := cpuPercent(); got != -1 {
		t.Errorf("first reading gave %v, want -1 for not yet known", got)
	}

	time.Sleep(MinCPUInterval + 50*time.Millisecond)

	got := cpuPercent()

	if got < 0 || got > 100 {
		t.Errorf("second reading gave %v, want a percentage", got)
	}
}

// Readings taken too close together must not be believed.
func TestCPUIgnoresReadingsTakenTooCloseTogether(t *testing.T) {
	cpu.mu.Lock()
	cpu.last = cpuSample{}
	cpu.mu.Unlock()
	lastCPUPercent = -1

	cpuPercent()
	time.Sleep(MinCPUInterval + 50*time.Millisecond)

	settled := cpuPercent()

	// Immediately again: too soon to mean anything, so the previous answer
	// stands rather than a fresh division by an interval of almost nothing.
	if got := cpuPercent(); got != settled {
		t.Errorf("a reading taken immediately gave %v, want the previous %v", got, settled)
	}
}
