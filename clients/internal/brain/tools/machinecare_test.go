package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

type careHere struct{ stopped *[]int }

func (careHere) CareLook(context.Context) (machinecare.LastStop, machinecare.Readiness, []machinecare.Leftover, bool) {
	return machinecare.LastStop{Known: true, Unclean: true, LastSeen: time.Date(2026, 11, 8, 13, 41, 0, 0, time.Local)},
		machinecare.Judge(machinecare.Inputs{Running: "6.8.0-147-generic", Installed: []string{"6.8.0-149-generic"},
			DKMS: "rtl8821au/5.12.5.2, 6.8.0-147-generic, x86_64: installed"}),
		[]machinecare.Leftover{{PID: 42, Command: "Xvfb :137", Age: "3 h", Kind: "virtual display", Why: "whatever started it has gone"}},
		true
}

func (c careHere) StopLeftover(pid int, command string) error {
	if pid != 42 || command != "Xvfb :137" {
		return errors.New("not on the list")
	}

	*c.stopped = append(*c.stopped, pid)

	return nil
}

func TestTheMachineCareToolsSayWhatTheMachineSays(t *testing.T) {
	var stopped []int

	care := careHere{stopped: &stopped}

	out, _ := CanIRestart{Care: care}.Execute(context.Background(), nil)
	for _, want := range []string{"Not yet.", "In the way: The driver rtl8821au is not built for 6.8.0-149-generic",
		"The last session was not closed", "13:41 on 8 November"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not in:\n%s", want, out)
		}
	}

	out, _ = LeftoverProcesses{Care: care}.Execute(context.Background(), nil)
	if !strings.Contains(out, "pid 42, virtual display, running 3 h") {
		t.Errorf("%s", out)
	}

	stop := StopLeftover{Care: care}
	if stop.Risk() != Mutating {
		t.Fatal("stopping a process is not asked about")
	}

	if _, err := stop.Execute(context.Background(), json.RawMessage(`{"pid":43,"command":"bash"}`)); err == nil {
		t.Error("a process not on the list was stopped")
	}

	if _, err := stop.Execute(context.Background(), json.RawMessage(`{"pid":42,"command":"Xvfb :137"}`)); err != nil || len(stopped) != 1 {
		t.Errorf("%v %v", err, stopped)
	}
}
