package brain

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"pn-scripts-assistant/internal/brain/machinecare"
	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Looking after the machine (package machinecare): how the last session
 * ended, what was left running whose job is over, and whether a restart now
 * would cut anything off or come back without a driver.
 */

// CareReport is the whole look, for the System page and the tools.
type CareReport struct {
	LastStop  machinecare.LastStop   `json:"last_stop"`
	Restart   machinecare.Readiness  `json:"restart"`
	Leftovers []machinecare.Leftover `json:"leftovers"`

	// Watch is "tell me when I can restart": going now, or the last one,
	// or nil. See restartwatch.go.
	Watch *machinecare.Watch `json:"watch"`

	// Here is false on a system the look does not cover yet.
	Here bool `json:"here"`
}

// MostReposChecked is how many project folders are asked about unpushed
// work in one look.
const MostReposChecked = 30

// MachineCare looks now.
func (b *Brain) MachineCare(ctx context.Context) CareReport {
	r := CareReport{LastStop: b.LastStop, Leftovers: []machinecare.Leftover{}}

	procs, err := machinecare.Processes()
	r.Here = err == nil

	if r.Here {
		r.Leftovers = machinecare.Find(procs, os.Getuid(), os.Getpid())
		if r.Leftovers == nil {
			r.Leftovers = []machinecare.Leftover{}
		}
	}

	r.Restart = machinecare.Read(ctx, b.busyWithWhat(), b.workFolders())
	r.Watch = b.RestartWatch()

	return r
}

// busyWithWhat is the assistant's own work that a restart would cut off.
func (b *Brain) busyWithWhat() []string {
	var busy []string

	if tasks, err := b.DB.Tasks([]string{store.TaskPlanning, store.TaskWorking}, 20); err == nil {
		for _, t := range tasks {
			busy = append(busy, t.Name)
		}
	}

	if !agentRun.TryLock() {
		busy = append(busy, "a task given to a coding agent")
	} else {
		agentRun.Unlock()
	}

	return busy
}

// workFolders is where unpushed work could be: the places it learns from
// and the folders its tasks worked in.
func (b *Brain) workFolders() []string {
	seen := map[string]bool{}

	var out []string

	add := func(dir string) {
		if dir == "" || seen[dir] || len(out) >= MostReposChecked {
			return
		}

		if !filepath.IsAbs(dir) {
			dir = filepath.Join(b.Root, dir)
		}

		seen[dir] = true
		out = append(out, dir)
	}

	if list, err := places.List(b.Root); err == nil {
		for _, p := range list {
			add(p.Path)
		}
	}

	if tasks, err := b.DB.Tasks(nil, 50); err == nil {
		for _, t := range tasks {
			add(t.Project)
		}
	}

	return out
}

// StopLeftover stops one process on the leftover list, by its number and
// the command the list showed for it.
func (b *Brain) StopLeftover(pid int, command string) error {
	if pid <= 1 || pid == os.Getpid() {
		return errors.New("that process is not one to stop")
	}

	return machinecare.Stop(pid, command)
}

// CareLook is MachineCare for the tools.
func (b *Brain) CareLook(ctx context.Context) (machinecare.LastStop, machinecare.Readiness, []machinecare.Leftover, bool) {
	r := b.MachineCare(ctx)

	return r.LastStop, r.Restart, r.Leftovers, r.Here
}
