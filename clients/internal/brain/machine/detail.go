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

	/*
	 * GPUs is every graphics device, not only the ones with their own tool.
	 *
	 * This listed NVIDIA cards and nothing else, so a machine with integrated
	 * graphics — which is most machines, including this one — was shown a
	 * panel whose entire content was "No NVIDIA card on this machine." A
	 * screenful of what somebody does not have is worse than nothing: it takes
	 * the space the things they do have should be in.
	 */
	GPUs []GPU `json:"gpus"`

	// GPUNote explains a gap rather than filling one — a machine where the
	// card's own tool is installed but will not answer.
	GPUNote string `json:"gpu_note,omitempty"`

	// Temperatures and Fans are whatever this machine's sensors publish.
	Temperatures []Sensor `json:"temperatures,omitempty"`
	Fans         []Fan    `json:"fans,omitempty"`

	// Links are the network interfaces and what is moving over each.
	Links []Link `json:"links,omitempty"`

	// Power is the battery, on a machine that has one.
	Power *Battery `json:"power,omitempty"`

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

// Sensor is one temperature the machine publishes.
type Sensor struct {
	// Chip is the thing doing the measuring — coretemp, acpitz, nvme — and
	// Label is what it says the reading is of.
	Chip  string `json:"chip"`
	Label string `json:"label"`

	Celsius float64 `json:"celsius"`

	// HighC and CriticalC are the thresholds the chip itself publishes, where
	// it publishes them. Its own numbers rather than a guess: what counts as
	// hot for a processor package is not what counts as hot for a disk, and
	// picking one figure for both is how a dashboard cries wolf.
	HighC     float64 `json:"high_c"`
	CriticalC float64 `json:"critical_c"`
}

// Fan is one fan and how fast it is turning.
type Fan struct {
	Chip  string  `json:"chip"`
	Label string  `json:"label"`
	RPM   float64 `json:"rpm"`
}

// Link is one network interface.
type Link struct {
	Name  string `json:"name"`
	State string `json:"state"`

	// SpeedMbps is what the link negotiated, or -1 where the driver does not
	// say — which is the ordinary case for wireless.
	SpeedMbps float64 `json:"speed_mbps"`

	InPerSecond  float64 `json:"in_per_second"`
	OutPerSecond float64 `json:"out_per_second"`

	// Wireless marks a link whose speed is a moving target rather than a
	// property of the cable.
	Wireless bool `json:"wireless"`
}

// Battery is the machine's own power, where it has any.
type Battery struct {
	Percent float64 `json:"percent"`
	State   string  `json:"state"`

	// Watts is what it is drawing or being charged at, or -1 when unpublished.
	Watts float64 `json:"watts"`
}

// GPU is one graphics device, as its own driver describes it.
type GPU struct {
	Index int    `json:"index"`
	Name  string `json:"name"`

	// Vendor and Driver say what it is and what is running it, which for an
	// integrated part is most of what can be known about it.
	Vendor string `json:"vendor,omitempty"`
	Driver string `json:"driver,omitempty"`

	// ClockMaxMHz is what it is allowed to run at, so the current clock means
	// something. Integrated graphics publish a clock and no busy figure, and
	// how close it is running to its maximum is the honest answer to "is it
	// doing anything".
	ClockMaxMHz float64 `json:"clock_max_mhz"`

	// UtilPercent and MemoryPercent are what the card reports, or -1 when the
	// driver does not publish that figure.
	UtilPercent   float64 `json:"util_percent"`
	MemoryPercent float64 `json:"memory_percent"`

	/*
	 * UtilFromClock marks a figure that is not a busy percentage at all.
	 *
	 * Intel publishes a clock speed and no busy figure, so what can honestly
	 * be derived is how close the part is running to its maximum. That answers
	 * "is it doing anything", which is the question — but it is not the same
	 * measurement NVIDIA and AMD report, and labelling both "busy" would put
	 * an integrated chip at 100% next to a card at 100% and mean two different
	 * things by it.
	 */
	UtilFromClock bool `json:"util_from_clock,omitempty"`

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

/*
 * Vitals is the cheap reading, taken every second for the graphs.
 *
 * Five files. What separates it from Now is everything that has to walk the
 * machine — the processes, the sensors, the graphics driver — which is worth
 * taking while somebody is looking at it and worth nothing otherwise.
 */
func Vitals() Detail { return vitals() }
