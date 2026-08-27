package machine

import (
	"runtime"
	"testing"
	"time"
)

// The first reading cannot be a percentage, and the second must be.
//
// Processor use only exists between two samples of the kernel's counters. The
// tempting shortcut is to report busy-since-boot on the first call, which is a
// real number that answers a question nobody asked — on a machine up for a week
// it would show a calm average no matter what is happening now.
func TestCPUNeedsTwoReadings(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}

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
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}

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

// Memory must be reported as used against total, and be plausible.
func TestMemoryIsMeasured(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}

	load := Current()

	if load.MemoryTotalBytes == 0 {
		t.Fatal("total memory reads as zero")
	}

	if load.MemoryUsedBytes == 0 || load.MemoryUsedBytes > load.MemoryTotalBytes {
		t.Errorf("used %d of %d bytes, which cannot be right",
			load.MemoryUsedBytes, load.MemoryTotalBytes)
	}

	// The cache trap: used must not be nearly everything on a machine that is
	// merely running a test suite. This is what reading MemFree instead of
	// MemAvailable would produce.
	if pct := load.MemoryPercent(); pct > 99 {
		t.Errorf("memory reads as %.0f%% used, which suggests cache is being counted", pct)
	}
}

// An unmeasurable figure must say so rather than read as zero.
func TestUnknownMemoryIsNotZeroPercent(t *testing.T) {
	if got := (Load{}).MemoryPercent(); got != -1 {
		t.Errorf("memory with no total gave %v, want -1 for unknown", got)
	}
}
