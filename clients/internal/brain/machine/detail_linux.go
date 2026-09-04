//go:build linux

package machine

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
 * Reading the machine, from the machine.
 *
 * All of it comes out of /proc, which is the same place top, htop and every
 * other tool reads — so the numbers here are the numbers those show, rather
 * than an approximation of them. Where a file is missing or unreadable the
 * figure is left unknown; there is no fallback that invents one.
 */

func detail() Detail {
	d := Detail{At: time.Now(), Available: true, Overall: -1}

	d.Overall, d.Cores = processors()
	d.LoadAverage = loadAverage()
	d.UptimeSeconds = uptime()
	d.Tasks, d.Threads, d.Running = tasks()
	d.MemoryUsedBytes, d.MemoryTotalBytes, d.SwapUsedBytes, d.SwapTotalBytes = memoryAndSwap()
	d.GPUs, d.GPUNote = graphicsCards()
	d.Processes = heaviest(d.MemoryTotalBytes)
	d.NetworkInPerSecond, d.NetworkOutPerSecond = networkRate()
	d.DiskReadPerSecond, d.DiskWritePerSecond = diskRate()

	return d
}

/*
 * Processors, overall and one by one.
 *
 * A percentage of processor time only means anything between two readings, so
 * the previous one is kept. The first call after start has nothing to compare
 * against and says so with -1 rather than reporting a busy machine as idle.
 */
type coreSample struct {
	busy, total float64
}

var lastCores struct {
	mu   sync.Mutex
	seen map[string]coreSample
}

func processors() (overall float64, cores []float64) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return -1, nil
	}
	defer file.Close()

	lastCores.mu.Lock()
	defer lastCores.mu.Unlock()

	if lastCores.seen == nil {
		lastCores.seen = map[string]coreSample{}
	}

	overall = -1

	scan := bufio.NewScanner(file)

	for scan.Scan() {
		fields := strings.Fields(scan.Text())

		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}

		var total, idle float64

		for i, raw := range fields[1:] {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}

			total += v

			// Fields four and five are idle and iowait: time the processor
			// was not working, which is the whole of what "busy" is measured
			// against.
			if i == 3 || i == 4 {
				idle += v
			}
		}

		now := coreSample{busy: total - idle, total: total}
		was, had := lastCores.seen[fields[0]]
		lastCores.seen[fields[0]] = now

		percent := -1.0

		if had {
			if spent := now.total - was.total; spent > 0 {
				percent = (now.busy - was.busy) / spent * 100
			}
		}

		if fields[0] == "cpu" {
			overall = percent

			continue
		}

		cores = append(cores, percent)
	}

	return overall, cores
}

func loadAverage() []float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}

	fields := strings.Fields(string(raw))

	if len(fields) < 3 {
		return nil
	}

	out := make([]float64, 0, 3)

	for _, f := range fields[:3] {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}

		out = append(out, v)
	}

	return out
}

func uptime() float64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(raw))

	if len(fields) == 0 {
		return 0
	}

	v, _ := strconv.ParseFloat(fields[0], 64)

	return v
}

// tasks counts processes, their threads, and how many are on a processor right
// now — the three numbers top puts at the top of the screen.
func tasks() (processes, threads, running int) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, 0, 0
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue
		}

		processes++

		for _, line := range strings.Split(string(status), "\n") {
			switch {
			case strings.HasPrefix(line, "Threads:"):
				n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Threads:")))
				threads += n

			case strings.HasPrefix(line, "State:"):
				if strings.Contains(line, "R (running)") {
					running++
				}
			}
		}
	}

	return processes, threads, running
}

/*
 * Memory as a person means it, and swap.
 *
 * Used is total minus available rather than total minus free: Linux lends
 * unused memory to the page cache, so "free" on a healthy machine is nearly
 * zero and reporting that as usage says the machine is full when it is fine.
 */
func memoryAndSwap() (used, total, swapUsed, swapTotal uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0
	}
	defer file.Close()

	var available, swapFree uint64

	scan := bufio.NewScanner(file)

	for scan.Scan() {
		fields := strings.Fields(scan.Text())

		if len(fields) < 2 {
			continue
		}

		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			available = kb * 1024
		case "SwapTotal:":
			swapTotal = kb * 1024
		case "SwapFree:":
			swapFree = kb * 1024
		}
	}

	if total > available {
		used = total - available
	}

	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}

	return used, total, swapUsed, swapTotal
}
