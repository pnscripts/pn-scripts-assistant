package machine

import "time"

/*
 * The whole picture, for the operations view.
 *
 * Load answers "is it working" for a dial in the corner of the screen. This
 * answers the questions somebody asks when they are watching a machine do a
 * long job: which core is pegged, how much of the graphics card's memory the
 * model took, what is actually running, how long it has been up.
 *
 * Everything is measured or marked unknown. That rule matters more here than
 * anywhere else in the program, because a wall of numbers reads as authority —
 * and a single invented figure among fifty real ones is indistinguishable from
 * the real ones.
 */

// Detail is everything the operations view shows about this machine.
type Detail struct {
	At time.Time `json:"at"`

	// Processors. Each is 0-100; Overall is the average.
	Overall float64   `json:"overall"`
	Cores   []float64 `json:"cores"`

	// LoadAverage over one, five and fifteen minutes, or nil when the system
	// does not publish one.
	LoadAverage []float64 `json:"load_average,omitempty"`

	UptimeSeconds float64 `json:"uptime_seconds"`

	// Tasks, Threads and Running are the process counts, the way top says them.
	Tasks   int `json:"tasks"`
	Threads int `json:"threads"`
	Running int `json:"running"`

	MemoryUsedBytes  uint64 `json:"memory_used_bytes"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`
	SwapUsedBytes    uint64 `json:"swap_used_bytes"`
	SwapTotalBytes   uint64 `json:"swap_total_bytes"`

	// GPUs is empty on a machine with none, which is a fact rather than a
	// failure and is said as one.
	GPUs []GPU `json:"gpus"`

	// GPUTool is what was asked, so a machine with a card this cannot read
	// says why rather than showing nothing.
	GPUNote string `json:"gpu_note,omitempty"`

	// Processes are the heaviest few, by processor time.
	Processes []Process `json:"processes"`

	// Network and Disk are rates, in bytes per second, since the previous
	// sample. Negative until there has been a previous sample to compare with.
	NetworkInPerSecond  float64 `json:"network_in_per_second"`
	NetworkOutPerSecond float64 `json:"network_out_per_second"`
	DiskReadPerSecond   float64 `json:"disk_read_per_second"`
	DiskWritePerSecond  float64 `json:"disk_write_per_second"`

	// Available is false where the platform publishes none of this.
	Available bool `json:"available"`
}

// GPU is one graphics card, as its own driver describes it.
type GPU struct {
	Index int    `json:"index"`
	Name  string `json:"name"`

	// UtilPercent and MemoryPercent are what the card reports, or -1 when the
	// driver does not publish that figure.
	UtilPercent   float64 `json:"util_percent"`
	MemoryPercent float64 `json:"memory_percent"`

	MemoryUsedBytes  uint64 `json:"memory_used_bytes"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`

	TemperatureC float64 `json:"temperature_c"`
	FanPercent   float64 `json:"fan_percent"`

	PowerWatts    float64 `json:"power_watts"`
	PowerCapWatts float64 `json:"power_cap_watts"`

	ClockMHz    float64 `json:"clock_mhz"`
	MemClockMHz float64 `json:"mem_clock_mhz"`
}

// Process is one running program, the few facts a person scanning a list uses.
type Process struct {
	PID        int     `json:"pid"`
	User       string  `json:"user"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	RSSBytes   uint64  `json:"rss_bytes"`

	// CPUTime is total processor time used, as a person says it: 2h 19m.
	CPUTime string `json:"cpu_time"`

	Command string `json:"command"`

	// Ours marks the brain's own processes and the model server it talks to,
	// which is the row somebody is looking for in a list of two hundred.
	Ours bool `json:"ours"`
}

// Now reads the whole picture.
func Now() Detail { return detail() }
