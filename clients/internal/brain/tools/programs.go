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

// ProgramChoosing is a keeper that also knows what each program may be
// used for (its card's choices, the general setting, the stop switch).
type ProgramChoosing interface {
	ProgramChoices() (programs.Choices, error)
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
	return "The programs installed on this computer, kept current by itself: menu programs (the Start " +
		"menu on Windows), system packages, snaps and Flatpaks on Linux, installed programs and app " +
		"packages on Windows, with versions, and what was installed, updated or removed in the last two " +
		"weeks. name finds a program (also one that was removed, with when) and says what the owner chose " +
		"for it: start it, type and click in its windows, run it as a command (always, never, or not " +
		"decided); the overview names the programs with a choice of their own, and says when the owner " +
		"stopped all use of other programs. kind narrows to app, deb, snap, flatpak, win (Windows' " +
		"installed programs) or msix (app packages). Use it before saying a program is or is not there, " +
		"or what you may do with it."
}

func (ListPrograms) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Part of a program's name or package, e.g. gimp."},
			"kind": {"type": "string", "enum": ["app", "deb", "snap", "flatpak", "win", "msix"], "description": "Only this source."},
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
	if kind != "" && !programs.Known(kind) {
		return "", fmt.Errorf("kind is app, deb, snap, flatpak, win or msix, not %q", a.Kind)
	}

	records, state, err := t.Programs.ProgramInventory()
	if err != nil {
		return "", err
	}

	var choices *programs.Choices

	if c, ok := t.Programs.(ProgramChoosing); ok {
		got, err := c.ProgramChoices()
		if err != nil {
			return "", err
		}

		choices = &got
	}

	// Stopped is said first, whatever was asked: it decides everything else.
	stopped := ""
	if choices != nil && choices.Stopped {
		stopped = "Stopped: the owner stopped all use of other programs on " + choices.Since.Local().Format("2 Jan 2006 at 15:04") +
			"; nothing is opened, typed, clicked, scrolled or run in another program until they turn it back on " +
			"(\"Use other programs again\" on the Programs page). "
	}

	if len(records) == 0 {
		return stopped + "The programs on this computer have not been read yet; ask again in a minute.", nil
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
		return stopped + findPrograms(mine, name) + matchedChoices(mine, name, choices), nil
	case a.Removed:
		return stopped + removedPrograms(mine), nil
	}

	return stopped + programOverview(mine, state, kind, time.Now()) + overviewChoices(mine, choices), nil
}

// mostChoices is how many programs one answer says the choices of.
const mostChoices = 10

/*
 * choiceWords is one program's choices, verb by verb, as decided on its
 * card or, undecided, as the general setting answers: "GIMP (menu): start
 * it: always; type and click in its windows: not decided, asked each time;
 * run it as a command: never". onlyDecided leaves out what is not decided.
 * A removed program says only its decisions, kept for when it is back.
 * Empty when there is nothing to say.
 */
func choiceWords(r programs.Record, c programs.Choices, onlyDecided bool) string {
	byVerb := map[programs.Verb]programs.Permit{}
	for _, p := range c.Permits[r.RowID] {
		byVerb[p.Verb] = p
	}

	var parts []string

	for _, v := range programs.InUse {
		p, decided := byVerb[v]

		if !decided && (onlyDecided || !r.Present) {
			continue
		}

		var said string

		switch {
		case !decided && v != programs.VerbInput && c.Unasked:
			said = "not decided, runs without asking"
		case !decided && v == programs.VerbInput:
			said = "not decided, asked each time"
		case !decided:
			said = "not decided, the general permissions decide"
		case p.Answer == programs.Never:
			said = "never"
		case !r.Present:
			said = "always"
		case p.Asleep(r.Version):
			said = "always, given to version " + p.Version + ", asked again since the update"
		case !c.Honoured:
			said = "always, but asked each time while every action is asked"
		default:
			said = "always"
		}

		parts = append(parts, programs.VerbWords(v)+": "+said)
	}

	if len(parts) == 0 {
		return ""
	}

	if !r.Present {
		return programLabel(r) + ": kept for when it comes back: " + strings.Join(parts, "; ")
	}

	return programLabel(r) + ": " + strings.Join(parts, "; ")
}

// matchedChoices is the choices of the programs a name found.
func matchedChoices(rs []programs.Record, name string, c *programs.Choices) string {
	if c == nil {
		return ""
	}

	var lines []string

	for _, r := range rs {
		if !strings.Contains(strings.ToLower(r.Name), name) && !strings.Contains(strings.ToLower(r.ID), name) &&
			!strings.Contains(strings.ToLower(r.Exec), name) {
			continue
		}

		if w := choiceWords(r, *c, false); w != "" {
			lines = append(lines, w+".")
		}
	}

	if len(lines) == 0 {
		return ""
	}

	more := ""
	if len(lines) > mostChoices {
		more = fmt.Sprintf(" And %d more (ask with a longer name to see their choices).", len(lines)-mostChoices)
		lines = lines[:mostChoices]
	}

	return " What the owner chose for each: " + strings.Join(lines, " ") + more
}

// overviewChoices names the programs with a choice of their own, and says
// how the general setting answers for every other one.
func overviewChoices(rs []programs.Record, c *programs.Choices) string {
	if c == nil {
		return ""
	}

	var lines []string

	for _, r := range rs {
		if w := choiceWords(r, *c, true); w != "" {
			lines = append(lines, w+".")
		}
	}

	undecided := "starting a program and running it as a command follow the general permissions"
	if c.Unasked {
		undecided = "starting a program and running it as a command run without asking"
	}

	undecided += ", and typing and clicking in one is asked each time."

	if len(lines) == 0 {
		return " No program has a choice of its own: " + undecided
	}

	more := ""
	if len(lines) > mostListed {
		more = fmt.Sprintf(" And %d more (ask with a name to see them).", len(lines)-mostListed)
		lines = lines[:mostListed]
	}

	return " Choices of their own: " + strings.Join(lines, " ") + more + " Every other program: " + undecided
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

// sourceNames are the sources as the overview counts them.
var sourceNames = map[programs.Kind]string{
	programs.Deb:     "Packages",
	programs.Snap:    "Snaps",
	programs.Flatpak: "Flatpaks",
	programs.Win:     "Installed programs (Settings, Apps)",
	programs.Msix:    "App packages",
}

func programOverview(rs []programs.Record, state []programs.SourceState, only programs.Kind, now time.Time) string {
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
		case !r.Initial && r.ChangedAt.Equal(r.FirstSeen):
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

	// Each source this computer has, and any other a remembered program
	// came from; asked for one kind, only that one.
	for _, k := range programs.Shown(rs) {
		if k == programs.App || (only != "" && k != only) {
			continue
		}

		fmt.Fprintf(&b, " %s: %d.", sourceNames[k], counts[k])
	}

	if len(changes) > 0 {
		b.WriteString(" In the last two weeks: " + cappedPrograms(changes) + ".")
	} else {
		b.WriteString(" Nothing was installed, updated or removed in the last two weeks.")
	}

	// A source that cannot be watched is said, unless it is simply not on
	// this computer (no Flatpak installed) and has nothing in it.
	for _, s := range state {
		if s.Watched == 0 && len(s.Polled) > 0 && (counts[s.Kind] > 0 || s.Polled[0].Why != programs.WhyMissing) {
			fmt.Fprintf(&b, " %s: looked at every %d minutes (%s), not noticed at once.",
				s.Kind, int(programs.DefaultOptions().PollEvery/time.Minute), s.Polled[0].Why)
		}
	}

	return strings.TrimSpace(b.String())
}
