//go:build linux

package machine

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * The view describes the machine it is running on, whichever machine that is.
 *
 * This is the whole requirement, and it is not something the machine it was
 * written on can demonstrate: that one has an Intel chip, seven temperature
 * probes, no fans it publishes and no battery. So the machine is described to
 * the reader instead — a laptop here, a server there — and what comes back is
 * checked against it.
 */
func pretendMachine(t *testing.T, chips map[string]map[string]string) {
	t.Helper()

	root := t.TempDir()

	for chip, files := range chips {
		dir := filepath.Join(root, chip)

		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		for name, value := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	was := hwmonRoot
	hwmonRoot = root

	t.Cleanup(func() { hwmonRoot = was })
}

func TestItReadsWhateverSensorsAMachineHas(t *testing.T) {
	pretendMachine(t, map[string]map[string]string{
		"hwmon0": {
			"name":        "coretemp",
			"temp1_input": "66000",
			"temp1_label": "Package id 0",
			"temp1_max":   "80000",
			"temp1_crit":  "100000",
		},
		"hwmon1": {
			"name":        "nvme",
			"temp1_input": "41850",
			"temp1_label": "Composite",
			"temp1_crit":  "84850",
			"fan1_input":  "2400",
			"fan1_label":  "Chassis",
		},
		// A probe that is not connected. Zero degrees among real readings is
		// indistinguishable from a very cold room.
		"hwmon2": {"name": "nct6798", "temp1_input": "0"},
	})

	temps, fans := sensors()

	if len(temps) != 2 {
		t.Fatalf("read %d temperatures, want 2: %+v", len(temps), temps)
	}

	// Ordered, so the same machine reads the same way twice.
	if temps[0].Chip != "coretemp" || temps[1].Chip != "nvme" {
		t.Errorf("the order is not stable: %+v", temps)
	}

	if temps[0].Celsius != 66 || temps[0].HighC != 80 || temps[0].CriticalC != 100 {
		t.Errorf("the processor package read as %+v", temps[0])
	}

	/*
	 * A disk's limits are not a processor's, and the chip publishes its own.
	 * 41.85°C is normal for an NVMe drive and would be alarming for nothing.
	 */
	if temps[1].CriticalC < 84 || temps[1].CriticalC > 85 {
		t.Errorf("the disk's own critical point was not used: %+v", temps[1])
	}

	if temps[1].HighC != -1 {
		t.Errorf("a limit the chip does not publish was invented: %v", temps[1].HighC)
	}

	if len(fans) != 1 || fans[0].RPM != 2400 || fans[0].Label != "Chassis" {
		t.Errorf("the fan read as %+v", fans)
	}
}

// A machine that publishes none of it reports none of it, rather than a row of
// zeroes that reads as a very cold computer with stopped fans.
func TestAMachineWithNoSensorsReportsNone(t *testing.T) {
	pretendMachine(t, map[string]map[string]string{})

	temps, fans := sensors()

	if len(temps) != 0 || len(fans) != 0 {
		t.Errorf("sensors were invented: %+v %+v", temps, fans)
	}
}

/*
 * And a laptop's battery, which most machines do not have.
 *
 * Reported as nothing at all on a desktop rather than as a battery at zero
 * percent, which is what a dead one looks like.
 */
func TestTheBatteryIsReadWhereThereIsOne(t *testing.T) {
	root := t.TempDir()

	bat := filepath.Join(root, "BAT0")
	os.MkdirAll(bat, 0o755)
	os.WriteFile(filepath.Join(bat, "type"), []byte("Battery\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "capacity"), []byte("72\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "status"), []byte("Discharging\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "power_now"), []byte("7500000\n"), 0o644)

	// And a wall socket, which is a power supply and not a battery.
	ac := filepath.Join(root, "AC")
	os.MkdirAll(ac, 0o755)
	os.WriteFile(filepath.Join(ac, "type"), []byte("Mains\n"), 0o644)

	was := powerRoot
	powerRoot = root

	t.Cleanup(func() { powerRoot = was })

	b := battery()

	if b == nil {
		t.Fatal("a machine with a battery reported none")
	}

	if b.Percent != 72 || b.State != "discharging" || b.Watts != 7.5 {
		t.Errorf("the battery read as %+v", b)
	}

	powerRoot = t.TempDir()

	if desktop := battery(); desktop != nil {
		t.Errorf("a desktop was given a battery: %+v", desktop)
	}
}
