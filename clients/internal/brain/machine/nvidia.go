package machine

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

/*
 * Asking the graphics cards what they are doing.
 *
 * NVIDIA publishes nothing usable through the kernel's sysfs — no busy
 * percentage, no memory figure, no temperature — so the only honest source is
 * its own tool. That tool is not always installed, and a machine without it is
 * a machine this cannot answer for: said as such rather than shown as zeroes,
 * because a card sitting at 0% and a card nobody can ask look identical on a
 * dial and mean completely different things.
 *
 * Intel and AMD do publish through sysfs, and gpuUsage covers them for the
 * single dial in the corner of the screen. This is the detailed reading, which
 * only NVIDIA's tool can give.
 */

// nvidiaTimeout is how long the tool gets. It normally answers in tens of
// milliseconds; a card in a bad state can hang it, and a hung operations panel
// is worse than one that says it could not read.
const nvidiaTimeout = 3 * time.Second

// whatToAsk is the query, in the order the fields come back.
const whatToAsk = "index,name,utilization.gpu,utilization.memory,memory.used," +
	"memory.total,temperature.gpu,fan.speed,power.draw,power.limit,clocks.sm,clocks.mem"

// askNvidia runs the tool, so tests can supply its output instead.
var askNvidia = func(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu="+whatToAsk, "--format=csv,noheader,nounits").Output()

	return string(out), err
}

func graphicsCards() ([]GPU, string) {
	ctx, cancel := context.WithTimeout(context.Background(), nvidiaTimeout)
	defer cancel()

	raw, err := askNvidia(ctx)
	if err != nil {
		if _, missing := exec.LookPath("nvidia-smi"); missing != nil {
			// Not a fault. Most machines have no NVIDIA card, and the panel
			// says that rather than showing an empty table.
			return nil, "No NVIDIA card on this machine."
		}

		return nil, "nvidia-smi is installed but would not answer: " + err.Error()
	}

	cards := parseNvidia(raw)

	if len(cards) == 0 {
		return nil, "nvidia-smi answered, but listed no cards."
	}

	return cards, ""
}

/*
 * parseNvidia reads the tool's csv.
 *
 * Every field is optional in practice. A card that does not report fan speed
 * answers "[N/A]", and a driver that does not publish power limits leaves them
 * blank — so each is parsed on its own and left negative when it is not a
 * number, rather than throwing away the whole row because one column was
 * missing.
 */
func parseNvidia(raw string) []GPU {
	var out []GPU

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		fields := strings.Split(line, ",")

		if len(fields) < 6 {
			continue
		}

		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		index, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		g := GPU{
			Index:         index,
			Name:          fields[1],
			UtilPercent:   number(at(fields, 2)),
			MemoryPercent: number(at(fields, 3)),
			TemperatureC:  number(at(fields, 6)),
			FanPercent:    number(at(fields, 7)),
			PowerWatts:    number(at(fields, 8)),
			PowerCapWatts: number(at(fields, 9)),
			ClockMHz:      number(at(fields, 10)),
			MemClockMHz:   number(at(fields, 11)),
		}

		// Memory comes back in mebibytes, and everything else in this program
		// counts bytes.
		if used := number(at(fields, 4)); used >= 0 {
			g.MemoryUsedBytes = uint64(used) * 1024 * 1024
		}

		if total := number(at(fields, 5)); total >= 0 {
			g.MemoryTotalBytes = uint64(total) * 1024 * 1024
		}

		/*
		 * And the memory percentage the card actually has, not the one it
		 * reports.
		 *
		 * utilization.memory is the fraction of time the memory bus was busy,
		 * which is a different thing entirely from how full the memory is —
		 * and it is the second one anybody reading a dashboard means. Three
		 * cards holding a model each show 70% full and 0% bus.
		 */
		if g.MemoryTotalBytes > 0 {
			g.MemoryPercent = float64(g.MemoryUsedBytes) / float64(g.MemoryTotalBytes) * 100
		}

		out = append(out, g)
	}

	return out
}

func at(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}

	return ""
}

// number reads a figure the tool may have declined to give.
func number(raw string) float64 {
	raw = strings.TrimSpace(raw)

	if raw == "" || strings.Contains(raw, "N/A") || strings.Contains(raw, "Unknown") {
		return -1
	}

	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}

	return v
}
