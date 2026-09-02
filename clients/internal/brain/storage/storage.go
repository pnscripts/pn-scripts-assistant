// Package storage reports how much room the brain has left.
//
// This matters more here than in most programs. The brain grows on its own —
// every document it reads becomes facts and vectors — and it lives on a
// removable disk that may be shared with everything else the owner keeps. A
// brain that fills its disk does not fail politely: writes start failing
// mid-learning, which is how a build once died and took a day's work with it.
//
// So the interface shows the number before it becomes a problem, and says what
// to do about it while there is still room to do it.
package storage

import "os"

// Report is what the interface displays.
type Report struct {
	Path          string `json:"path"`
	TotalBytes    uint64 `json:"total_bytes"`
	UsedBytes     uint64 `json:"used_bytes"`
	FreeBytes     uint64 `json:"free_bytes"`
	DatabaseBytes int64  `json:"database_bytes"`

	// Level is ok, warn or critical.
	Level string `json:"level"`

	// Advice is empty when there is nothing to say. The interface hides the
	// warning entirely rather than showing a reassuring message nobody reads.
	Advice string `json:"advice"`
}

// Thresholds. Percentage alone is wrong on a large disk — 5% of 2TB is 100GB,
// which is not an emergency — and an absolute floor alone is wrong on a small
// one. Both are checked, and the worse answer wins.
const (
	warnFraction  = 0.10
	criticalBytes = 2 << 30 // 2GB
)

// Check inspects the filesystem holding root.
func Check(root, databasePath string) Report {
	r := Report{Path: root, Level: "ok"}

	total, free, err := spaceOn(root)
	if err != nil {
		// Not being able to measure is not the same as being full. Report
		// unknown rather than inventing a number that would drive the warning.
		r.Level = "unknown"
		r.Advice = "Could not read free space for " + root + "."

		return r
	}

	r.TotalBytes = total
	r.FreeBytes = free
	r.UsedBytes = r.TotalBytes - r.FreeBytes

	if info, err := os.Stat(databasePath); err == nil {
		r.DatabaseBytes = info.Size()
	}

	switch {
	case r.FreeBytes < criticalBytes:
		r.Level = "critical"
		r.Advice = "Under 2GB left on this drive. The brain will stop being able to " +
			"learn, and writes may fail part-way. Move it to a larger drive or free space now."
	case r.TotalBytes > 0 && float64(r.FreeBytes)/float64(r.TotalBytes) < warnFraction:
		r.Level = "warn"
		r.Advice = "Less than a tenth of this drive is free. Consider moving the brain " +
			"to a larger drive before it fills."
	}

	return r
}

// FreeOn is the usable space on the filesystem holding path.
//
// Exported for the places that want the one number rather than the whole
// report — chiefly deciding whether a copy of the brain will fit on a drive
// before starting to write one.
func FreeOn(path string) (uint64, error) {
	_, free, err := spaceOn(path)

	return free, err
}
