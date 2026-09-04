//go:build linux

package machine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

/*
 * Every graphics device, not only the ones with their own tool.
 *
 * The detailed reading was NVIDIA-only, because NVIDIA is the only vendor
 * whose card cannot be read through the kernel at all. The effect was that a
 * machine with integrated graphics — most machines, this one included — got a
 * panel whose whole content was a sentence about a card it does not have.
 *
 * Intel and AMD publish through the kernel's DRM directory: a clock, sometimes
 * a busy percentage, sometimes memory. Less than NVIDIA gives, and enough to
 * say what the machine has and whether it is doing anything.
 */

var drmRoot = "/sys/class/drm"

// vendors are the three PCI identifiers worth naming. Anything else is
// reported by its number rather than guessed at.
var vendors = map[string]string{
	"0x8086": "Intel",
	"0x1002": "AMD",
	"0x10de": "NVIDIA",
}

func integratedCards(alreadyNamed int) []GPU {
	cards, err := filepath.Glob(filepath.Join(drmRoot, "card*"))
	if err != nil {
		return nil
	}

	var out []GPU

	for _, card := range cards {
		name := filepath.Base(card)

		// card1-HDMI-A-1 is a connector on card1, not another card.
		if strings.Contains(name, "-") {
			continue
		}

		device := filepath.Join(card, "device")

		vendor := firstLineOf(filepath.Join(device, "vendor"))

		if vendor == "" {
			continue
		}

		/*
		 * NVIDIA cards are skipped here even though they appear in this
		 * directory, because nvidia-smi has already described them properly —
		 * with memory, power and temperature. Listing them twice, once in
		 * detail and once as a bare clock, would read as six cards on a
		 * machine with three.
		 */
		if vendors[vendor] == "NVIDIA" && alreadyNamed > 0 {
			continue
		}

		g := GPU{
			Index:         len(out) + alreadyNamed,
			Vendor:        vendors[vendor],
			Driver:        driverOf(device),
			UtilPercent:   -1,
			MemoryPercent: -1,
			TemperatureC:  -1,
			FanPercent:    -1,
			PowerWatts:    -1,
			PowerCapWatts: -1,
			ClockMHz:      -1,
			ClockMaxMHz:   -1,
			MemClockMHz:   -1,
		}

		if g.Vendor == "" {
			g.Vendor = vendor
		}

		g.Name = describeCard(g.Vendor, g.Driver, firstLineOf(filepath.Join(device, "device")))

		// Intel publishes a clock and no busy figure; how close it is running
		// to its maximum is what can honestly be derived from that.
		if now, ok := readNumber(filepath.Join(card, "gt_cur_freq_mhz")); ok {
			g.ClockMHz = now
		}

		if most, ok := readNumber(filepath.Join(card, "gt_max_freq_mhz")); ok && most > 0 {
			g.ClockMaxMHz = most

			if g.ClockMHz >= 0 {
				g.UtilPercent = g.ClockMHz / most * 100
				g.UtilFromClock = true
			}
		}

		// AMD publishes a real busy percentage, and its memory. A real one
		// replaces the figure derived from the clock, and says so.
		if busy, ok := readNumber(filepath.Join(device, "gpu_busy_percent")); ok {
			g.UtilPercent = busy
			g.UtilFromClock = false
		}

		used, hasUsed := readNumber(filepath.Join(device, "mem_info_vram_used"))
		total, hasTotal := readNumber(filepath.Join(device, "mem_info_vram_total"))

		if hasUsed && hasTotal && total > 0 {
			g.MemoryUsedBytes = uint64(used)
			g.MemoryTotalBytes = uint64(total)
			g.MemoryPercent = used / total * 100
		}

		g.TemperatureC = cardTemperature(device)

		out = append(out, g)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

func driverOf(device string) string {
	link, err := os.Readlink(filepath.Join(device, "driver"))
	if err != nil {
		return ""
	}

	return filepath.Base(link)
}

/*
 * describeCard names the part as well as it honestly can.
 *
 * The kernel publishes a PCI device number, not a marketing name, and the file
 * that maps one to the other is a package that may not be installed. So it is
 * named by vendor and driver — "Intel graphics · i915" — with the device
 * number kept, because that is the thing somebody can actually look up.
 */
func describeCard(vendor, driver, device string) string {
	name := strings.TrimSpace(vendor + " graphics")

	if driver != "" {
		name += " · " + driver
	}

	if device != "" {
		name += " · " + strings.TrimPrefix(device, "0x")
	}

	return name
}

// cardTemperature finds a temperature the graphics device publishes about
// itself, which AMD does and Intel does not.
func cardTemperature(device string) float64 {
	chips, _ := filepath.Glob(filepath.Join(device, "hwmon", "hwmon*"))

	for _, chip := range chips {
		if milli, ok := readNumber(filepath.Join(chip, "temp1_input")); ok && milli > 0 {
			return milli / 1000
		}
	}

	return -1
}

// linksRoot is where the kernel publishes network interfaces.
var linksRoot = "/sys/class/net"

/*
 * networkLinks is every interface and what is moving over it.
 *
 * Per interface rather than one total, because "the network is busy" is not
 * useful on a machine with wifi, ethernet and half a dozen virtual bridges —
 * which one is the whole question.
 */
func networkLinks() []Link {
	names, err := os.ReadDir(linksRoot)
	if err != nil {
		return nil
	}

	var out []Link

	for _, entry := range names {
		name := entry.Name()

		// This machine talking to itself. Every request the interface makes to
		// its own server goes over it, so including it shows a busy network on
		// a machine that is not on one.
		if name == "lo" {
			continue
		}

		l := Link{
			Name:      name,
			State:     firstLineOf(filepath.Join(linksRoot, name, "operstate")),
			SpeedMbps: -1,
			Wireless:  isWireless(name),
		}

		if speed, ok := readInt(filepath.Join(linksRoot, name, "speed")); ok && speed > 0 {
			l.SpeedMbps = float64(speed)
		}

		rx, hasRx := readNumber(filepath.Join(linksRoot, name, "statistics", "rx_bytes"))
		tx, hasTx := readNumber(filepath.Join(linksRoot, name, "statistics", "tx_bytes"))

		if hasRx && hasTx {
			l.InPerSecond, l.OutPerSecond = perLink(name).rate(rx, tx)
		} else {
			l.InPerSecond, l.OutPerSecond = -1, -1
		}

		out = append(out, l)
	}

	/*
	 * The ones that are up first, then by name.
	 *
	 * A machine with containers or virtual machines on it has a dozen bridges
	 * that are down, and burying the one interface that is carrying traffic
	 * underneath them is the opposite of what this panel is for.
	 */
	sort.Slice(out, func(i, j int) bool {
		if (out[i].State == "up") != (out[j].State == "up") {
			return out[i].State == "up"
		}

		return out[i].Name < out[j].Name
	})

	return out
}

func isWireless(name string) bool {
	_, err := os.Stat(filepath.Join(linksRoot, name, "wireless"))

	return err == nil
}

/*
 * perLink keeps a previous reading for each interface, since a rate is the
 * difference between two of them and each interface needs its own.
 *
 * Guarded because the reading is normally taken by the watcher's goroutine and
 * there is nothing stopping a second caller asking at the same moment — and a
 * map written from two goroutines does not race quietly, it ends the process.
 */
var perLinkCounters struct {
	mu   sync.Mutex
	seen map[string]*counterPair
}

func perLink(name string) *counterPair {
	perLinkCounters.mu.Lock()
	defer perLinkCounters.mu.Unlock()

	if perLinkCounters.seen == nil {
		perLinkCounters.seen = map[string]*counterPair{}
	}

	if p, had := perLinkCounters.seen[name]; had {
		return p
	}

	p := &counterPair{}
	perLinkCounters.seen[name] = p

	return p
}
