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
 * How much is moving, rather than how much has moved.
 *
 * The kernel publishes totals since boot, which on a machine that has been up
 * for a week is a number with no meaning to anybody. What somebody watching a
 * machine wants is the rate right now, and a rate needs two readings — so the
 * previous one is kept and the first call reports unknown rather than zero.
 */
type counters struct {
	a, b float64
	at   time.Time
	had  bool
}

var (
	lastNet  = &counterPair{}
	lastDisk = &counterPair{}
)

type counterPair struct {
	mu   sync.Mutex
	last counters
}

func (p *counterPair) rate(a, b float64) (float64, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	was := p.last

	p.last = counters{a: a, b: b, at: now, had: true}

	if !was.had {
		return -1, -1
	}

	gap := now.Sub(was.at).Seconds()

	if gap <= 0 {
		return -1, -1
	}

	// A counter that went backwards means it wrapped or the interface was
	// reset. Reported as unknown rather than as a vast negative rate.
	if a < was.a || b < was.b {
		return -1, -1
	}

	return (a - was.a) / gap, (b - was.b) / gap
}

func networkRate() (in, out float64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return -1, -1
	}
	defer file.Close()

	var rx, tx float64

	scan := bufio.NewScanner(file)

	for scan.Scan() {
		line := scan.Text()
		at := strings.Index(line, ":")

		if at < 0 {
			continue
		}

		name := strings.TrimSpace(line[:at])

		// The loopback interface is this machine talking to itself, which
		// includes every request the interface makes to its own server — so
		// counting it would show a busy network on a machine that is not on
		// one.
		if name == "lo" {
			continue
		}

		fields := strings.Fields(line[at+1:])

		if len(fields) < 9 {
			continue
		}

		r, _ := strconv.ParseFloat(fields[0], 64)
		t, _ := strconv.ParseFloat(fields[8], 64)

		rx += r
		tx += t
	}

	return lastNet.rate(rx, tx)
}

func diskRate() (read, write float64) {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return -1, -1
	}
	defer file.Close()

	const sector = 512

	var readSectors, writeSectors float64

	scan := bufio.NewScanner(file)

	for scan.Scan() {
		fields := strings.Fields(scan.Text())

		if len(fields) < 10 {
			continue
		}

		name := fields[2]

		/*
		 * Whole disks only.
		 *
		 * /proc/diskstats lists every partition as well as the disk holding
		 * them, so counting all of it counts the same bytes two or three
		 * times. Partitions end in a digit on the names this cares about; loop
		 * and ram devices are not disks anybody is watching.
		 */
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}

		if last := name[len(name)-1]; last >= '0' && last <= '9' &&
			(strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "hd") ||
				strings.HasPrefix(name, "vd")) {
			continue
		}

		r, _ := strconv.ParseFloat(fields[5], 64)
		w, _ := strconv.ParseFloat(fields[9], 64)

		readSectors += r
		writeSectors += w
	}

	read, write = lastDisk.rate(readSectors*sector, writeSectors*sector)

	return read, write
}
