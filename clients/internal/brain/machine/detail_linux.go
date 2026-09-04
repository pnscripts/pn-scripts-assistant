//go:build linux

package machine

import (
	"bufio"
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

/*
 * vitals is the cheap reading: five files, taken every second.
 *
 * The separation is the whole of what makes this affordable. A full reading
 * walks every process on the machine and shells out to the graphics driver,
 * which was measured here at a fifth of a second — repeated every second,
 * forever, on four cores already running a language model. That is a tenth of
 * a core spent drawing a panel nobody may be looking at.
 *
 * What the graphs need is only this: the processors, the memory, and the two
 * throughputs. Each is a difference between two readings and so has to be
 * taken on a rhythm; none of them costs more than reading a file.
 */
func vitals() Detail {
	d := Detail{At: time.Now(), Available: true, Overall: -1}

	d.Overall, d.Cores = processors()
	d.MemoryUsedBytes, d.MemoryTotalBytes, d.SwapUsedBytes, d.SwapTotalBytes = memoryAndSwap()
	d.NetworkInPerSecond, d.NetworkOutPerSecond = networkRate()
	d.DiskReadPerSecond, d.DiskWritePerSecond = diskRate()

	return d
}

/*
 * detail is everything, and is only read while somebody is watching.
 *
 * The expensive half: the process list, the sensors, and the graphics driver.
 * None of it is needed to draw a graph of the last three minutes, and all of
 * it is needed the moment the operations view is open — so it is read then,
 * rather than every second against the chance that it might be.
 */
func detail() Detail {
	d := vitals()

	d.LoadAverage = loadAverage()
	d.UptimeSeconds = uptime()
	d.Tasks, d.Threads, d.Running = tasks()
	/*
	 * Every graphics device, whichever vendor made it.
	 *
	 * NVIDIA first, because its own tool gives a full account — memory, power,
	 * temperature — and then whatever else the kernel knows about, which on
	 * most machines is the integrated part doing the actual work.
	 */
	d.GPUs, d.GPUNote = graphicsCards()
	d.GPUs = append(d.GPUs, integratedCards(len(d.GPUs))...)

	// A note explaining an absence is only worth keeping when there is one.
	if len(d.GPUs) > 0 {
		d.GPUNote = ""
	}

	d.Temperatures, d.Fans = sensors()
	d.Links = networkLinks()
	d.Power = battery()
	d.Processes = heaviest(d.MemoryTotalBytes)

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

/*
 * tasks counts processes, threads, and how many are on a processor right now.
 *
 * From one file. This used to open /proc/<pid>/status for every process on the
 * machine and add the numbers up — three hundred file reads a second to
 * produce three integers the kernel already publishes in a single line of
 * /proc/loadavg, which is where top gets them.
 *
 * The line ends "0.42 0.51 0.60 2/2033 918273": the load averages, then
 * running out of total threads, then the last process id issued.
 */
func tasks() (processes, threads, running int) {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}

	fields := strings.Fields(string(raw))

	if len(fields) < 4 {
		return 0, 0, 0
	}

	share := strings.SplitN(fields[3], "/", 2)

	if len(share) != 2 {
		return 0, 0, 0
	}

	running, _ = strconv.Atoi(share[0])
	threads, _ = strconv.Atoi(share[1])

	// Processes, as distinct from threads, still means counting the
	// directories — but that is one readdir rather than three hundred opens.
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name()[0] >= '0' && e.Name()[0] <= '9' {
				processes++
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
