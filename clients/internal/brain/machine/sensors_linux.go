//go:build linux

package machine

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

/*
 * Whatever this machine measures about itself.
 *
 * Nothing here is a list of things to look for. It walks what the kernel has
 * published and reports what it finds, so the same code says "package 66°C,
 * four cores" on a desktop, adds a battery and two fans on a laptop, and lists
 * a dozen probes on a server — without knowing in advance which machine it is
 * running on.
 *
 * That is the whole requirement: the brain gets carried between machines, and
 * a panel that only works on the machine it was written on is a panel that
 * works once.
 */

// hwmonRoot is where the kernel publishes its sensors, replaceable so a test
// can describe a machine other than the one it is running on.
var hwmonRoot = "/sys/class/hwmon"

func sensors() ([]Sensor, []Fan) {
	chips, err := filepath.Glob(filepath.Join(hwmonRoot, "hwmon*"))
	if err != nil {
		return nil, nil
	}

	var (
		temps []Sensor
		fans  []Fan
	)

	for _, chip := range chips {
		name := firstLineOf(filepath.Join(chip, "name"))

		if name == "" {
			name = filepath.Base(chip)
		}

		temps = append(temps, temperaturesIn(chip, name)...)
		fans = append(fans, fansIn(chip, name)...)
	}

	/*
	 * Ordered so the same machine reads the same way twice.
	 *
	 * The glob comes back in whatever order the directory happens to be in,
	 * which is stable in practice and not guaranteed — and a list of
	 * temperatures that reorders itself while somebody is watching it is
	 * unreadable in a way that is hard to even describe as a bug.
	 */
	sort.Slice(temps, func(i, j int) bool {
		if temps[i].Chip != temps[j].Chip {
			return temps[i].Chip < temps[j].Chip
		}

		return temps[i].Label < temps[j].Label
	})

	sort.Slice(fans, func(i, j int) bool { return fans[i].Label < fans[j].Label })

	return temps, fans
}

func temperaturesIn(chip, name string) []Sensor {
	inputs, _ := filepath.Glob(filepath.Join(chip, "temp*_input"))

	var out []Sensor

	for _, input := range inputs {
		milli, ok := readNumber(input)
		if !ok {
			continue
		}

		// A probe that is not connected reads zero, and zero degrees in a
		// list of real temperatures is indistinguishable from a cold room.
		if milli <= 0 {
			continue
		}

		base := strings.TrimSuffix(input, "_input")

		s := Sensor{
			Chip:      name,
			Label:     labelFor(base, name),
			Celsius:   milli / 1000,
			HighC:     -1,
			CriticalC: -1,
		}

		if high, ok := readNumber(base + "_max"); ok && high > 0 {
			s.HighC = high / 1000
		}

		if crit, ok := readNumber(base + "_crit"); ok && crit > 0 {
			s.CriticalC = crit / 1000
		}

		out = append(out, s)
	}

	return out
}

func fansIn(chip, name string) []Fan {
	inputs, _ := filepath.Glob(filepath.Join(chip, "fan*_input"))

	var out []Fan

	for _, input := range inputs {
		rpm, ok := readNumber(input)
		if !ok {
			continue
		}

		out = append(out, Fan{
			Chip:  name,
			Label: labelFor(strings.TrimSuffix(input, "_input"), name),
			RPM:   rpm,
		})
	}

	return out
}

/*
 * labelFor is what the chip calls this reading, or something a person can read.
 *
 * Most chips publish a label — "Package id 0", "Core 3", "Composite" — and the
 * ones that do not leave "temp1", which says nothing. Falling back to the chip
 * name plus the number at least says which thing is being measured.
 */
func labelFor(base, chip string) string {
	if label := firstLineOf(base + "_label"); label != "" {
		return label
	}

	name := filepath.Base(base)

	if n := strings.TrimLeft(name, "abcdefghijklmnopqrstuvwxyz"); n != "" && n != "1" {
		return chip + " " + n
	}

	return chip
}

func firstLineOf(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
}

/*
 * battery is the machine's own power, on a machine that runs on any.
 *
 * A desktop has none, which is not a fault and is reported as nothing at all
 * rather than as a battery at zero percent.
 */
var powerRoot = "/sys/class/power_supply"

func battery() *Battery {
	supplies, err := filepath.Glob(filepath.Join(powerRoot, "*"))
	if err != nil {
		return nil
	}

	for _, supply := range supplies {
		if firstLineOf(filepath.Join(supply, "type")) != "Battery" {
			continue
		}

		percent, ok := readNumber(filepath.Join(supply, "capacity"))
		if !ok {
			continue
		}

		b := Battery{
			Percent: percent,
			State:   strings.ToLower(firstLineOf(filepath.Join(supply, "status"))),
			Watts:   -1,
		}

		// Some report power directly in microwatts; others report current and
		// voltage and leave the multiplication to the reader.
		if micro, ok := readNumber(filepath.Join(supply, "power_now")); ok && micro > 0 {
			b.Watts = micro / 1e6
		} else {
			amps, hasAmps := readNumber(filepath.Join(supply, "current_now"))
			volts, hasVolts := readNumber(filepath.Join(supply, "voltage_now"))

			if hasAmps && hasVolts {
				b.Watts = (amps / 1e6) * (volts / 1e6)
			}
		}

		return &b
	}

	return nil
}

func readInt(path string) (int64, bool) {
	raw := firstLineOf(path)

	if raw == "" {
		return 0, false
	}

	v, err := strconv.ParseInt(raw, 10, 64)

	return v, err == nil
}
