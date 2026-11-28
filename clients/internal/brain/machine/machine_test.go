package machine

import (
	"runtime"
	"testing"
)

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
