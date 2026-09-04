package machine

import (
	"context"
	"sync"
	"time"
)

/*
 * Keeping the last few minutes, so the graphs have something to draw.
 *
 * A dial says what is happening now. The question somebody watching a machine
 * actually has is whether it is going up — a model loading, a scan starting, a
 * card heating — and that needs the shape of the last few minutes rather than
 * one reading.
 *
 * Held in memory and nowhere else. This is a picture of right now, not
 * knowledge: writing it to the database would put a row a second into the file
 * the brain keeps everything it has learned in, forever, to answer a question
 * nobody asks about last Tuesday.
 */

// HowOften is the gap between readings, and HowMany is how many are kept —
// together, how far back the graphs go.
const (
	HowOften = time.Second
	HowMany  = 180
)

// Moment is one reading, small on purpose: this is kept a hundred and eighty
// times over and sent to the interface on every refresh.
type Moment struct {
	At time.Time `json:"at"`

	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Swap   float64 `json:"swap"`

	// GPU and GPUMemory are per card, in the order the cards are listed.
	GPU       []float64 `json:"gpu,omitempty"`
	GPUMemory []float64 `json:"gpu_memory,omitempty"`

	NetworkIn  float64 `json:"network_in"`
	NetworkOut float64 `json:"network_out"`
	DiskRead   float64 `json:"disk_read"`
	DiskWrite  float64 `json:"disk_write"`
}

type recorder struct {
	mu      sync.Mutex
	moments []Moment
	latest  Detail
	started bool
}

var kept recorder

/*
 * Watch takes a reading every second until the context ends.
 *
 * Started once and shared: two callers watching the same machine would halve
 * the interval between readings for everything that reads a rate, and rates
 * are the numbers most easily made nonsense by that.
 */
func Watch(ctx context.Context) {
	kept.mu.Lock()

	if kept.started {
		kept.mu.Unlock()

		return
	}

	kept.started = true
	kept.mu.Unlock()

	tick := time.NewTicker(HowOften)
	defer tick.Stop()

	for {
		record(Now())

		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func record(d Detail) {
	m := Moment{
		At:         d.At,
		CPU:        d.Overall,
		NetworkIn:  d.NetworkInPerSecond,
		NetworkOut: d.NetworkOutPerSecond,
		DiskRead:   d.DiskReadPerSecond,
		DiskWrite:  d.DiskWritePerSecond,
		Memory:     -1,
		Swap:       -1,
	}

	if d.MemoryTotalBytes > 0 {
		m.Memory = float64(d.MemoryUsedBytes) / float64(d.MemoryTotalBytes) * 100
	}

	if d.SwapTotalBytes > 0 {
		m.Swap = float64(d.SwapUsedBytes) / float64(d.SwapTotalBytes) * 100
	}

	for _, g := range d.GPUs {
		m.GPU = append(m.GPU, g.UtilPercent)
		m.GPUMemory = append(m.GPUMemory, g.MemoryPercent)
	}

	kept.mu.Lock()
	defer kept.mu.Unlock()

	kept.latest = d
	kept.moments = append(kept.moments, m)

	if len(kept.moments) > HowMany {
		kept.moments = kept.moments[len(kept.moments)-HowMany:]
	}
}

// History is the readings kept so far, oldest first.
func History() []Moment {
	kept.mu.Lock()
	defer kept.mu.Unlock()

	out := make([]Moment, len(kept.moments))
	copy(out, kept.moments)

	return out
}

/*
 * Latest is the most recent full reading.
 *
 * Handed back from the watcher rather than taken fresh, because every rate here
 * — processor time, network, disk — is the difference between two readings, and
 * an extra reading taken out of turn consumes the comparison the next scheduled
 * one needed. The panel would show a machine busier or quieter than it is,
 * every other second.
 */
func Latest() (Detail, bool) {
	kept.mu.Lock()
	defer kept.mu.Unlock()

	if !kept.latest.Available && len(kept.moments) == 0 {
		return Detail{}, false
	}

	return kept.latest, true
}
