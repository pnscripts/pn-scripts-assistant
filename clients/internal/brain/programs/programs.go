/*
Package programs is the inventory of the programs on this computer: what is
installed, what came and went, and how the assistant keeps that current
without looking all the time.

Four sources, each read on its own (Scanner):

  - deb: the system's packages, asked of dpkg-query (dpkg's interface; its
    database files are dpkg's own business);
  - snap: /snap/<name>/current/meta/snap.yaml, readable by everybody (snapd's
    state file is root's);
  - flatpak: the system and the user installation folders;
  - app: the menu entries (freedesktop .desktop files), which are the
    programs a person sees and starts.

Kept current by file events (Watch: inotify on Linux, debounced), with a slow
poll for whatever cannot be watched, and judged by Reconcile, which tells a
new program from a removed one, a reinstall from an upgrade. Nothing here
runs a program it found or changes anything on the machine.
*/
package programs

import (
	"errors"
	"time"
)

// Kind is where a program came from, and which source reads it.
type Kind string

const (
	Deb     Kind = "deb"
	Snap    Kind = "snap"
	Flatpak Kind = "flatpak"
	App     Kind = "app"
)

// Kinds is every source, in the order they are first read.
var Kinds = []Kind{App, Deb, Snap, Flatpak}

// ErrNotHere is a source this system does not have (no dpkg on Fedora): its
// programs are unknown, never "all removed".
var ErrNotHere = errors.New("this system has no such source of programs")

// Program is one program as a source reports it.
type Program struct {
	Kind Kind `json:"kind"`

	// ID is the source's own name for it: the package, the snap, the
	// Flatpak application id, the menu entry's desktop id.
	ID string `json:"id"`

	// Name is what a person calls it.
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`

	// Path is where it lives: the menu entry, the snap's or Flatpak's folder.
	Path string `json:"path,omitempty"`

	// Exec is the program a menu entry starts (its first word), for app only.
	Exec string `json:"exec,omitempty"`
}

// Key is unique across sources.
func (p Program) Key() string { return string(p.Kind) + ":" + p.ID }

// Record is a program as the assistant remembers it, present or gone.
type Record struct {
	Program

	// RowID is the store's; zero for one never written.
	RowID int64 `json:"-"`

	Present   bool       `json:"present"`
	FirstSeen time.Time  `json:"first_seen"`
	ChangedAt time.Time  `json:"changed_at"`
	RemovedAt *time.Time `json:"removed_at,omitempty"`

	// PreviousVersion is the version before the last update.
	PreviousVersion string `json:"previous_version,omitempty"`
}

// ChangeKind is what happened to a program between two looks.
type ChangeKind string

const (
	Added    ChangeKind = "added"
	Removed  ChangeKind = "removed"
	Returned ChangeKind = "returned" // installed again after it had been removed
	Updated  ChangeKind = "updated"  // a new version of one that stayed
)

// Change is one thing that happened.
type Change struct {
	Kind    ChangeKind `json:"kind"`
	Program Program    `json:"program"`

	// From is the version before an update, or before a removal for a
	// program that came back with another one.
	From string    `json:"from,omitempty"`
	At   time.Time `json:"at"`

	// WasRemovedAt is when a returned program had gone.
	WasRemovedAt *time.Time `json:"was_removed_at,omitempty"`
}

/*
 * Reconcile compares what a source shows now (seen) with what is remembered
 * of that source (known; records of other kinds are left alone, so one
 * source's list never removes another's programs) and returns the records
 * to write and what happened.
 *
 * Only records that changed are returned. A program that disappeared stays
 * remembered, marked removed with the time and the version it had; one that
 * comes back is "returned" (with From when its version differs). A change
 * in the name, path or command alone is written down quietly: a menu entry
 * renamed by an update is not news.
 *
 * The first entry for an ID wins; entries without an ID or of another kind
 * are ignored.
 */
func Reconcile(kind Kind, known []Record, seen []Program, now time.Time) (save []Record, changes []Change) {
	byKey := map[string]Record{}

	for _, r := range known {
		if r.Kind == kind {
			byKey[r.Key()] = r
		}
	}

	shown := map[string]bool{}

	for _, p := range seen {
		if p.Kind != kind || p.ID == "" || shown[p.Key()] {
			continue
		}

		shown[p.Key()] = true

		r, ok := byKey[p.Key()]
		if !ok {
			save = append(save, Record{Program: p, Present: true, FirstSeen: now, ChangedAt: now})
			changes = append(changes, Change{Kind: Added, Program: p, At: now})

			continue
		}

		next := r
		next.Program = p

		switch {
		case !r.Present:
			next.Present, next.RemovedAt, next.ChangedAt = true, nil, now

			c := Change{Kind: Returned, Program: p, At: now, WasRemovedAt: r.RemovedAt}
			if r.Version != p.Version {
				c.From, next.PreviousVersion = r.Version, r.Version
			}

			save, changes = append(save, next), append(changes, c)
		case r.Version != p.Version:
			next.PreviousVersion, next.ChangedAt = r.Version, now
			save = append(save, next)
			changes = append(changes, Change{Kind: Updated, Program: p, From: r.Version, At: now})
		case r.Name != p.Name || r.Path != p.Path || r.Exec != p.Exec:
			save = append(save, next)
		}
	}

	// Gone: remembered and present, not shown now. In the order known gave
	// them, so the result does not depend on map order.
	for _, r := range known {
		if r.Kind != kind || !r.Present || shown[r.Key()] {
			continue
		}

		at := now
		r.Present, r.RemovedAt, r.ChangedAt = false, &at, now
		save = append(save, r)
		changes = append(changes, Change{Kind: Removed, Program: r.Program, At: now})
	}

	return save, changes
}
