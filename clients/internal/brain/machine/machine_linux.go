//go:build linux

package machine

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// current reads /proc.
func current() Load {
	load := Load{
		CPUPercent: cpuPercent(),
		Cores:      runtime.NumCPU(),
		Available:  true,
	}

	load.MemoryUsedBytes, load.MemoryTotalBytes = memory()
	load.GPUPercent, load.GPUName, load.GPUKnown = gpuUsage()

	return load
}

// cpuSample is one reading of the kernel's running totals.
//
// Processor use is not a quantity that can be read; it only exists between two
// readings. /proc/stat counts jiffies spent in each state since boot, so the
// busy fraction is the change in busy over the change in total across an
// interval.
type cpuSample struct {
	busy  uint64
	total uint64
	at    time.Time
}

var cpu struct {
	mu   sync.Mutex
	last cpuSample
}

// MinCPUInterval is how far apart two readings must be to mean anything.
//
// Closer together than this the jiffy counts have barely moved and the
// arithmetic turns into noise: a reading of 0% or 100% depending on where the
// tick fell. The previous answer is repeated instead, which is honest — it is
// the most recent measurement there is.
const MinCPUInterval = 300 * time.Millisecond

var lastCPUPercent = -1.0

func cpuPercent() float64 {
	sample, ok := readCPU()
	if !ok {
		return -1
	}

	cpu.mu.Lock()
	defer cpu.mu.Unlock()

	previous := cpu.last

	if previous.at.IsZero() {
		// The first reading has nothing to compare against. Rather than
		// reporting a made-up figure, the caller is told it is not known yet;
		// the next poll a moment later will have one.
		cpu.last = sample

		return -1
	}

	if sample.at.Sub(previous.at) < MinCPUInterval {
		return lastCPUPercent
	}

	cpu.last = sample

	total := sample.total - previous.total
	if total == 0 {
		return lastCPUPercent
	}

	busy := sample.busy - previous.busy

	percent := float64(busy) / float64(total) * 100

	if percent < 0 {
		percent = 0
	}

	if percent > 100 {
		percent = 100
	}

	lastCPUPercent = percent

	return percent
}

func readCPU() (cpuSample, bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		// The first line is the total across all processors.
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}

		var total, idle uint64

		for i, field := range fields[1:] {
			v, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				continue
			}

			total += v

			// Fields 4 and 5 are idle and iowait. Waiting on a disk is not
			// the processor working, so both count as not busy.
			if i == 3 || i == 4 {
				idle += v
			}
		}

		return cpuSample{busy: total - idle, total: total, at: time.Now()}, true
	}

	return cpuSample{}, false
}

// memory reads how much physical memory is in use.
//
// Used is total minus MemAvailable, not total minus MemFree. The difference
// matters: Linux fills memory it is not otherwise using with cache, so MemFree
// on a machine that has been up for a while is near zero and would make every
// system look full. MemAvailable is the kernel's own estimate of what a new
// program could actually get, which is the number a person means when they ask
// how much memory is left.
func memory() (used, total uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var available uint64
	var haveAvailable bool

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// Values are in kibibytes.
		v *= 1024

		switch fields[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			available = v
			haveAvailable = true
		}
	}

	if total == 0 || !haveAvailable || available > total {
		return 0, total
	}

	return total - available, total
}

// gpuUsage reads how busy the graphics card is.
//
// Intel and AMD expose this through the kernel's DRM sysfs; NVIDIA does not,
// and its own tool is a separate install. Where it cannot be read the caller is
// told so rather than shown a zero, because a graphics card that reports 0%
// and one that cannot be asked look identical on a dial and mean completely
// different things.
func gpuUsage() (float64, string, bool) {
	name := gpuName()

	// Intel's driver publishes a frequency, not a busy percentage. What can
	// honestly be derived is how close it is running to its maximum, which is
	// a fair answer to "is the card working".
	current, okCurrent := readNumber("/sys/class/drm/card1/gt_cur_freq_mhz")
	most, okMost := readNumber("/sys/class/drm/card1/gt_max_freq_mhz")

	if !okCurrent || !okMost {
		current, okCurrent = readNumber("/sys/class/drm/card0/gt_cur_freq_mhz")
		most, okMost = readNumber("/sys/class/drm/card0/gt_max_freq_mhz")
	}

	if okCurrent && okMost && most > 0 {
		return (current / most) * 100, name, true
	}

	// AMD publishes a real busy percentage.
	for _, path := range []string{
		"/sys/class/drm/card0/device/gpu_busy_percent",
		"/sys/class/drm/card1/device/gpu_busy_percent",
	} {
		if busy, ok := readNumber(path); ok {
			return busy, name, true
		}
	}

	return -1, name, false
}

func gpuName() string {
	for _, path := range []string{
		"/sys/class/drm/card0/device/label",
		"/sys/class/drm/card1/device/label",
	} {
		if raw, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(raw))
		}
	}

	return ""
}

func readNumber(path string) (float64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		return 0, false
	}

	return v, true
}
