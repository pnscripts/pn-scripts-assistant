/*
 * Package autonomy is what goal mode does on its own (owner 2026-10-23:
 * "моделите и всичко останало трябва да се изтеглят, инсталират и използват
 * автоматично"): it chooses and downloads the model a step needs, sized to the
 * machine; installs the user-level tools a goal needs from the recipes, which
 * fetch from official sources and check them against their published
 * checksums; gives each goal a folder of its own; and keeps a ledger of what
 * it put on the machine, so that only those are ever taken away again and the
 * final report can say what was prepared.
 *
 * What it never does on its own is the four things of tools.GoalModeAsks:
 * anything needing administrator rights is refused here with ErrNeedsOwner.
 */
package autonomy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrOff is goal mode not agreed to: nothing is provided alone.
var ErrOff = errors.New("goal mode is off")

// ErrNeedsOwner is work goal mode may not do alone: it changes the whole
// system, so its owner is asked.
var ErrNeedsOwner = errors.New("this needs administrator rights, so it is put to you rather than done alone")

// Entry is one thing goal mode put on the machine.
type Entry struct {
	Kind string `json:"kind"` // "model", "tool" or "integration"
	Name string `json:"name"`
	// ID is the recipe's or integration's own id, for taking it away again.
	ID      string    `json:"id,omitempty"`
	Version string    `json:"version,omitempty"`
	Digest  string    `json:"digest,omitempty"`
	Bytes   int64     `json:"bytes,omitempty"`
	Source  string    `json:"source,omitempty"`
	For     string    `json:"for,omitempty"` // the specialist or goal it was for
	When    time.Time `json:"when"`
	Used    time.Time `json:"used"`
}

// Ledger is the record of what goal mode put here, in <brain>/tools.
type Ledger struct {
	mu   sync.Mutex
	path string
}

// LedgerAt is the ledger of the brain at root.
func LedgerAt(root string) *Ledger {
	return &Ledger{path: filepath.Join(root, "tools", "ledger.json")}
}

func (l *Ledger) read() []Entry {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil
	}

	var out []Entry

	_ = json.Unmarshal(data, &out)

	return out
}

func (l *Ledger) write(list []Entry) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	temp := l.path + ".new"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(temp, l.path)
}

// Entries is everything recorded.
func (l *Ledger) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.read()
}

// Add records something put here, replacing an earlier entry of the same
// kind and name.
func (l *Ledger) Add(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if e.When.IsZero() {
		e.When = time.Now()
	}

	if e.Used.IsZero() {
		e.Used = e.When
	}

	list := l.read()
	out := list[:0]

	for _, x := range list {
		if !(x.Kind == e.Kind && x.Name == e.Name) {
			out = append(out, x)
		}
	}

	return l.write(append(out, e))
}

// Touch records that something of ours was used now.
func (l *Ledger) Touch(kind, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	list := l.read()

	for i := range list {
		if list[i].Kind == kind && list[i].Name == name {
			list[i].Used = time.Now()
			_ = l.write(list)

			return
		}
	}
}

// Mine is whether goal mode put this here — the only things it may remove.
func (l *Ledger) Mine(kind, name string) bool {
	for _, e := range l.Entries() {
		if e.Kind == kind && e.Name == name {
			return true
		}
	}

	return false
}

// Forget drops an entry once what it records has been removed.
func (l *Ledger) Forget(kind, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	list := l.read()
	out := list[:0]

	for _, x := range list {
		if !(x.Kind == kind && x.Name == name) {
			out = append(out, x)
		}
	}

	return l.write(out)
}

// Unused is what goal mode put here and nobody has used for the given time,
// leaving out anything still in use. Never anything it did not put here.
func (l *Ledger) Unused(idle time.Duration, inUse func(Entry) bool, now time.Time) []Entry {
	var out []Entry

	for _, e := range l.Entries() {
		if now.Sub(e.Used) >= idle && (inUse == nil || !inUse(e)) {
			out = append(out, e)
		}
	}

	return out
}

// Since is what was put here from a moment on, oldest first: what a job
// prepared, for its report.
func (l *Ledger) Since(t time.Time) []Entry {
	var out []Entry

	for _, e := range l.Entries() {
		if !e.When.Before(t) {
			out = append(out, e)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].When.Before(out[j].When) })

	return out
}

// Lines are entries as a report reads them.
func Lines(entries []Entry) []string {
	var out []string

	for _, e := range entries {
		line := "• " + e.Name

		if e.Version != "" {
			line += " " + e.Version
		}

		var detail []string

		if e.Kind == "model" || e.Kind == "integration" {
			detail = append(detail, e.Kind)
		}

		if e.Bytes > 0 {
			detail = append(detail, readable(e.Bytes))
		}

		if e.Source != "" {
			detail = append(detail, "from "+e.Source)
		}

		if e.Digest != "" {
			d := strings.TrimPrefix(e.Digest, "sha256:")
			if len(d) > 12 {
				d = d[:12]
			}

			detail = append(detail, "checked "+d)
		}

		if e.For != "" {
			detail = append(detail, "for "+e.For)
		}

		if len(detail) > 0 {
			line += " (" + strings.Join(detail, ", ") + ")"
		}

		out = append(out, line)
	}

	return out
}

func readable(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	}

	return fmt.Sprintf("%d KB", b>>10)
}
