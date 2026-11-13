package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

// ProgramKeeping is what list_programs needs from the brain.
type ProgramKeeping interface {
	ProgramInventory() ([]programs.Record, []programs.SourceState, error)
}

/*
 * ListPrograms is "which programs do I have", "is GIMP installed", "what
 * did the update change", "why can't you use Krita": the inventory the
 * assistant keeps of this computer (package programs), present and
 * removed. It reads the assistant's own records, so it is Safe.
 */
type ListPrograms struct{ Programs ProgramKeeping }

func (ListPrograms) Name() string { return "list_programs" }

func (ListPrograms) Description() string {
	return "The programs installed on this computer, kept current by itself: menu programs, system " +
		"packages, snaps and Flatpaks, with versions, and what was installed, updated or removed in the " +
		"last two weeks. name finds a program (also one that was removed, with when); kind narrows to " +
		"app, deb, snap or flatpak. Use it before saying a program is or is not there."
}

func (ListPrograms) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Part of a program's name or package, e.g. gimp."},
			"kind": {"type": "string", "enum": ["app", "deb", "snap", "flatpak"], "description": "Only this source."},
			"removed": {"type": "boolean", "description": "List the removed programs instead."}
		},
		"required": []
	}`)
}

func (ListPrograms) Risk() Risk { return Safe }

func (ListPrograms) Cues() []string {
	return []string{"program", "programs", "installed", "install", "uninstall", "package", "packages",
		"snap", "flatpak", "програм", "програма", "програми", "инсталир", "инсталиран", "приложени", "пакет"}
}

type listProgramsArgs struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Removed bool   `json:"removed"`
}

func (ListPrograms) Summarize(raw json.RawMessage) string {
	var a listProgramsArgs

	_ = json.Unmarshal(raw, &a)

	if strings.TrimSpace(a.Name) != "" {
		return fmt.Sprintf("Look for %q among the programs on this computer", strings.TrimSpace(a.Name))
	}

	return "List the programs on this computer"
}

// mostListed is how many programs one answer names; the rest are counted.
const mostListed = 40

// mostMenu is how many menu programs the overview names.
const mostMenu = 80

// recently is how far back "what changed" goes.
const recently = 14 * 24 * time.Hour

func (t ListPrograms) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Programs == nil {
		return "", errors.New("the program inventory is not kept here")
	}

	var a listProgramsArgs

	if len(raw) > 0 {
		if err := argsOf(raw, &a); err != nil {
			return "", err
		}
	}

	kind := programs.Kind(strings.ToLower(strings.TrimSpace(a.Kind)))
	if kind != "" && kind != programs.App && kind != programs.Deb && kind != programs.Snap && kind != programs.Flatpak {
		return "", fmt.Errorf("kind is app, deb, snap or flatpak, not %q", a.Kind)
	}

	records, state, err := t.Programs.ProgramInventory()
	if err != nil {
		return "", err
	}

	if len(records) == 0 {
		return "The programs on this computer have not been read yet; ask again in a minute.", nil
	}

	var mine []programs.Record

	for _, r := range records {
		if kind == "" || r.Kind == kind {
			mine = append(mine, r)
		}
	}

	name := strings.ToLower(strings.TrimSpace(a.Name))

	switch {
	case name != "":
		return findPrograms(mine, name), nil
	case a.Removed:
		return removedPrograms(mine), nil
	}

	return programOverview(records, mine, state, time.Now()), nil
}

// programLabel is a program as the answer names it: name (source version).
func programLabel(r programs.Record) string {
	if r.Kind == programs.App {
		return r.Name + " (menu)"
	}

	if r.Version == "" {
		return fmt.Sprintf("%s (%s)", r.Name, r.Kind)
	}

	return fmt.Sprintf("%s (%s %s)", r.Name, r.Kind, r.Version)
}

func programTime(t time.Time) string { return t.Local().Format("2 Jan 2006 15:04") }

func findPrograms(rs []programs.Record, name string) string {
	var here, gone []string

	for _, r := range rs {
		if !strings.Contains(strings.ToLower(r.Name), name) && !strings.Contains(strings.ToLower(r.ID), name) &&
			!strings.Contains(strings.ToLower(r.Exec), name) {
			continue
		}

		if r.Present {
			here = append(here, programLabel(r))
		} else if r.RemovedAt != nil {
			gone = append(gone, programLabel(r)+" was removed on "+programTime(*r.RemovedAt))
		}
	}

	if len(here) == 0 && len(gone) == 0 {
		return fmt.Sprintf("No program called %q is or was on this computer.", name)
	}

	var b strings.Builder

	if len(here) > 0 {
		b.WriteString("On this computer: " + cappedPrograms(here) + ".")
	}

	if len(gone) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}

		b.WriteString(cappedPrograms(gone) + "; not on this computer now.")
	}

	return b.String()
}

func removedPrograms(rs []programs.Record) string {
	var gone []programs.Record

	for _, r := range rs {
		if !r.Present && r.RemovedAt != nil {
			gone = append(gone, r)
		}
	}

	if len(gone) == 0 {
		return "No program has been removed since the assistant began keeping the list."
	}

	sort.SliceStable(gone, func(i, j int) bool { return gone[i].RemovedAt.After(*gone[j].RemovedAt) })

	lines := make([]string, 0, len(gone))
	for _, r := range gone {
		lines = append(lines, programLabel(r)+" on "+programTime(*r.RemovedAt))
	}

	return "Removed, newest first: " + cappedPrograms(lines) + "."
}

// cappedPrograms joins the first mostListed and counts the rest.
func cappedPrograms(items []string) string {
	if len(items) <= mostListed {
		return strings.Join(items, "; ")
	}

	return strings.Join(items[:mostListed], "; ") +
		fmt.Sprintf("; and %d more (ask with a longer name to see them)", len(items)-mostListed)
}

func programOverview(all, rs []programs.Record, state []programs.SourceState, now time.Time) string {
	// The first look found what was already there: that is not news.
	var start time.Time
	for _, r := range all {
		if start.IsZero() || r.FirstSeen.Before(start) {
			start = r.FirstSeen
		}
	}

	var (
		menu    []string
		counts  = map[programs.Kind]int{}
		changes []string
	)

	sorted := append([]programs.Record(nil), rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ChangedAt.After(sorted[j].ChangedAt) })

	for _, r := range sorted {
		if r.Present {
			counts[r.Kind]++

			if r.Kind == programs.App {
				menu = append(menu, r.Name)
			}
		}

		if now.Sub(r.ChangedAt) > recently {
			continue
		}

		switch {
		case !r.Present && r.RemovedAt != nil:
			changes = append(changes, "Removed: "+programLabel(r)+" on "+programTime(*r.RemovedAt))
		case r.FirstSeen.Sub(start) > time.Minute && r.ChangedAt.Equal(r.FirstSeen):
			changes = append(changes, "Installed: "+programLabel(r)+" on "+programTime(r.FirstSeen))
		case r.ChangedAt.After(r.FirstSeen) && r.PreviousVersion != "" && r.PreviousVersion != r.Version:
			changes = append(changes, fmt.Sprintf("Updated: %s (%s) from %s to %s on %s",
				r.Name, r.Kind, r.PreviousVersion, r.Version, programTime(r.ChangedAt)))
		case r.ChangedAt.After(r.FirstSeen):
			changes = append(changes, "Installed again: "+programLabel(r)+" on "+programTime(r.ChangedAt))
		}
	}

	sort.Strings(menu)

	var b strings.Builder

	if len(menu) > 0 {
		shown := menu
		if len(shown) > mostMenu {
			shown = shown[:mostMenu]
		}

		fmt.Fprintf(&b, "In the menu (%d): %s", len(menu), strings.Join(shown, ", "))

		if len(menu) > mostMenu {
			fmt.Fprintf(&b, " and %d more", len(menu)-mostMenu)
		}

		b.WriteString(".")
	}

	fmt.Fprintf(&b, " Packages: %d. Snaps: %d. Flatpaks: %d.", counts[programs.Deb], counts[programs.Snap], counts[programs.Flatpak])

	if len(changes) > 0 {
		b.WriteString(" In the last two weeks: " + cappedPrograms(changes) + ".")
	} else {
		b.WriteString(" Nothing was installed, updated or removed in the last two weeks.")
	}

	for _, s := range state {
		if s.Watched == 0 && len(s.Polled) > 0 {
			fmt.Fprintf(&b, " %s: looked at every %d minutes (%s), not noticed at once.",
				s.Kind, int(programs.DefaultOptions().PollEvery/time.Minute), s.Polled[0].Why)
		}
	}

	return strings.TrimSpace(b.String())
}
