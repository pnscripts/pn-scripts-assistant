package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

type inventory struct {
	records []programs.Record
	state   []programs.SourceState
	err     error
}

func (i inventory) ProgramInventory() ([]programs.Record, []programs.SourceState, error) {
	return i.records, i.state, i.err
}

func rec(kind programs.Kind, id, name, version string, first, changed time.Time, removed *time.Time) programs.Record {
	return programs.Record{
		Program:   programs.Program{Kind: kind, ID: id, Name: name, Version: version},
		Present:   removed == nil,
		FirstSeen: first, ChangedAt: changed, RemovedAt: removed,
		Initial: first.Before(time.Now().Add(-29 * 24 * time.Hour)),
	}
}

func sampleInventory() inventory {
	now := time.Now()
	start := now.Add(-30 * 24 * time.Hour)
	gone := now.Add(-2 * 24 * time.Hour)
	long := now.Add(-20 * 24 * time.Hour)

	krita := rec(programs.Deb, "krita", "krita", "5.2", start, gone, &gone)
	old := rec(programs.Deb, "old-game", "old-game", "1.0", start, long, &long)
	ff := rec(programs.Snap, "firefox", "Firefox", "146.0", start, now.Add(-time.Hour), nil)
	ff.PreviousVersion = "145.0"

	return inventory{records: []programs.Record{
		rec(programs.App, "gimp.desktop", "GIMP", "", start, start, nil),
		rec(programs.App, "org.inkscape.Inkscape.desktop", "Inkscape", "", start, start, nil),
		rec(programs.Deb, "gimp", "gimp", "2.10.36", start, start, nil),
		rec(programs.Deb, "libc6", "libc6", "2.39", start, start, nil),
		krita, old, ff,
		rec(programs.Deb, "ffmpeg", "ffmpeg", "7.0", start, now.Add(-3*time.Hour), nil),
		rec(programs.Flatpak, "org.blender.Blender", "Blender", "4.5", now.Add(-5*time.Hour), now.Add(-5*time.Hour), nil),
	}, state: []programs.SourceState{
		{Kind: programs.App, Watched: 3},
		{Kind: programs.Deb, Watched: 1},
		{Kind: programs.Flatpak, Watched: 0, Polled: []programs.Polled{{Dir: "/var/lib/flatpak", Why: programs.WhyMissing}}},
	}}
}

func TestListProgramsGivesTheMenuAndWhatChanged(t *testing.T) {
	tool := ListPrograms{Programs: sampleInventory()}

	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"In the menu (2): GIMP, Inkscape.",
		"Packages: 3", "Snaps: 1", "Flatpaks: 1",
		"Removed: krita (deb 5.2) on ",
		"Updated: Firefox (snap) from 145.0 to 146.0",
		"Installed: Blender (flatpak 4.5)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	// Older than two weeks is not "recent"; the first look is not news.
	for _, not := range []string{"old-game", "Installed: gimp", "Installed: GIMP", "libc6"} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in:\n%s", not, out)
		}
	}
}

func TestAskingForARemovedProgramSaysWhenItWent(t *testing.T) {
	tool := ListPrograms{Programs: sampleInventory()}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"KRITA"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "krita (deb 5.2) was removed on ") || !strings.Contains(out, "not on this computer now") {
		t.Errorf("%s", out)
	}

	out, _ = tool.Execute(context.Background(), json.RawMessage(`{"name":"gimp"}`))
	if !strings.Contains(out, "GIMP (menu)") || !strings.Contains(out, "gimp (deb 2.10.36)") || strings.Contains(out, "removed") {
		t.Errorf("%s", out)
	}

	out, _ = tool.Execute(context.Background(), json.RawMessage(`{"name":"photoshop"}`))
	if !strings.Contains(out, "No program called \"photoshop\" is or was on this computer") {
		t.Errorf("%s", out)
	}
}

func TestListProgramsNeverCutsSilently(t *testing.T) {
	inv := inventory{}
	start := time.Now().Add(-40 * 24 * time.Hour)

	for i := 0; i < 130; i++ {
		inv.records = append(inv.records, rec(programs.Deb, fmt.Sprintf("lib%03d", i), fmt.Sprintf("lib%03d", i), "1", start, start, nil))
	}

	out, err := ListPrograms{Programs: inv}.Execute(context.Background(), json.RawMessage(`{"name":"lib","kind":"deb"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "and 90 more") || strings.Contains(out, "lib041") {
		t.Errorf("forty shown, the rest counted: %s", out)
	}
}

func TestListProgramsSaysHowItKeepsUp(t *testing.T) {
	out, _ := ListPrograms{Programs: sampleInventory()}.Execute(context.Background(), nil)

	if !strings.Contains(out, "flatpak: looked at every 10 minutes") {
		t.Errorf("a polled source is said: %s", out)
	}
}

func TestListProgramsRefusesWhatItCannotDo(t *testing.T) {
	if _, err := (ListPrograms{}).Execute(context.Background(), nil); err == nil {
		t.Error("no inventory: an error")
	}

	if _, err := (ListPrograms{Programs: sampleInventory()}).Execute(context.Background(), json.RawMessage(`{"kind":"rpm"}`)); err == nil {
		t.Error("an unknown kind is refused")
	}

	if (ListPrograms{}).Risk() != Safe {
		t.Error("reading the inventory changes nothing")
	}
}

// Installed a minute after the assistant first looked is still news; a
// source this computer does not have is not worth a sentence.
func TestAnInstallJustAfterTheFirstLookIsSaid(t *testing.T) {
	now := time.Now()
	start := now.Add(-2 * time.Minute)

	inv := inventory{records: []programs.Record{
		{Program: programs.Program{Kind: programs.App, ID: "a.desktop", Name: "Old"}, Present: true, Initial: true, FirstSeen: start, ChangedAt: start},
		{Program: programs.Program{Kind: programs.App, ID: "b.desktop", Name: "Check Editor"}, Present: true, FirstSeen: start.Add(20 * time.Second), ChangedAt: start.Add(20 * time.Second)},
	}, state: []programs.SourceState{
		{Kind: programs.Flatpak, Polled: []programs.Polled{{Dir: "/var/lib/flatpak", Why: programs.WhyMissing}}},
		{Kind: programs.Snap, Polled: []programs.Polled{{Dir: "/snap", Why: programs.WhyLimit}}},
	}}

	out, _ := ListPrograms{Programs: inv}.Execute(context.Background(), nil)

	if !strings.Contains(out, "Installed: Check Editor (menu) on ") || strings.Contains(out, "Installed: Old") {
		t.Errorf("%s", out)
	}

	if strings.Contains(out, "flatpak:") {
		t.Errorf("no Flatpak on this computer is not worth saying: %s", out)
	}

	if !strings.Contains(out, "snap: looked at every 10 minutes (the system refused a file watch)") {
		t.Errorf("a refused watch is said: %s", out)
	}
}

func TestWindowsProgramsAreListedAndAskedForByTheirSource(t *testing.T) {
	now := time.Now()
	start := now.Add(-30 * 24 * time.Hour)

	tool := ListPrograms{Programs: inventory{records: []programs.Record{
		rec(programs.App, "7-Zip/7-Zip File Manager.lnk", "7-Zip File Manager", "", start, start, nil),
		rec(programs.Win, "7-Zip", "7-Zip 24.09 (x64)", "24.09", start, start, nil),
		rec(programs.Win, "Discord", "Discord", "1.0.9171", now.Add(-time.Hour), now.Add(-time.Hour), nil),
		rec(programs.Msix, "Microsoft.WindowsCalculator_8wekyb3d8bbwe", "Windows Calculator", "11.2405.2.0", start, start, nil),
	}}}

	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"In the menu (1): 7-Zip File Manager.",
		"Installed programs (Settings, Apps): 2.", "App packages: 1.",
		"Installed: Discord (win 1.0.9171)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	// A Windows computer has no snaps: not counted as none.
	if programs.Kinds[1] == programs.Win && (strings.Contains(out, "Snaps") || strings.Contains(out, "Flatpaks")) {
		t.Errorf("sources this computer has not:\n%s", out)
	}

	out, err = tool.Execute(context.Background(), json.RawMessage(`{"kind":"msix"}`))
	if err != nil || !strings.Contains(out, "App packages: 1") || strings.Contains(out, "Installed programs (Settings") {
		t.Errorf("kind msix: %v\n%s", err, out)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"rpm"}`)); err == nil {
		t.Error("an unknown source must be refused")
	}
}

// choosing is an inventory that also knows each program's choices.
type choosing struct {
	inventory
	choices programs.Choices
}

func (c choosing) ProgramChoices() (programs.Choices, error) { return c.choices, nil }

// withRows numbers the sample's records, as the store does.
func withRows(inv inventory) inventory {
	for i := range inv.records {
		inv.records[i].RowID = int64(i + 1)
	}

	return inv
}

func rowOf(t *testing.T, inv inventory, kind programs.Kind, id string) programs.Record {
	t.Helper()

	for _, r := range inv.records {
		if r.Kind == kind && r.ID == id {
			return r
		}
	}

	t.Fatalf("no %s %s", kind, id)

	return programs.Record{}
}

/*
 * list_programs says what each program may be used for: start it, type
 * and click in it, run it as a command, each as decided on its card or,
 * undecided, as the general setting answers.
 */
func TestAskingForAProgramSaysItsChoices(t *testing.T) {
	inv := withRows(sampleInventory())
	gimpMenu := rowOf(t, inv, programs.App, "gimp.desktop")
	gimpDeb := rowOf(t, inv, programs.Deb, "gimp")

	c := choosing{inventory: inv, choices: programs.Choices{Honoured: true, Permits: map[int64][]programs.Permit{
		gimpMenu.RowID: {{Verb: programs.VerbStart, Answer: programs.Always}},
		gimpDeb.RowID: {
			{Verb: programs.VerbInput, Answer: programs.Always, Version: "2.10.30"},
			{Verb: programs.VerbChange, Answer: programs.Never},
		},
	}}}

	out, err := ListPrograms{Programs: c}.Execute(context.Background(), json.RawMessage(`{"name":"gimp"}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"GIMP (menu): start it: always; type and click in its windows: not decided, asked each time; run it as a command: not decided, the general permissions decide",
		"gimp (deb 2.10.36): start it: not decided, the general permissions decide; type and click in its windows: always, given to version 2.10.30, asked again since the update; run it as a command: never",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUndecidedChoicesFollowTheGeneralSetting(t *testing.T) {
	inv := withRows(sampleInventory())
	ff := rowOf(t, inv, programs.Snap, "firefox")

	everything := choosing{inventory: inv, choices: programs.Choices{Honoured: true, Unasked: true}}
	out, _ := ListPrograms{Programs: everything}.Execute(context.Background(), json.RawMessage(`{"name":"firefox"}`))

	// Typing and clicking is asked whatever the setting.
	if !strings.Contains(out, "start it: not decided, runs without asking; type and click in its windows: not decided, asked each time; run it as a command: not decided, runs without asking") {
		t.Errorf("everything allowed: %s", out)
	}

	askEvery := choosing{inventory: inv, choices: programs.Choices{Permits: map[int64][]programs.Permit{
		ff.RowID: {{Verb: programs.VerbStart, Answer: programs.Always, Version: "146.0"}},
	}}}
	out, _ = ListPrograms{Programs: askEvery}.Execute(context.Background(), json.RawMessage(`{"name":"firefox"}`))

	if !strings.Contains(out, "start it: always, but asked each time while every action is asked") {
		t.Errorf("ask every time: %s", out)
	}
}

func TestARemovedProgramsChoicesAreSaidKept(t *testing.T) {
	inv := withRows(sampleInventory())
	krita := rowOf(t, inv, programs.Deb, "krita")

	c := choosing{inventory: inv, choices: programs.Choices{Honoured: true, Permits: map[int64][]programs.Permit{
		krita.RowID: {{Verb: programs.VerbStart, Answer: programs.Never}},
	}}}

	out, _ := ListPrograms{Programs: c}.Execute(context.Background(), json.RawMessage(`{"name":"krita"}`))
	if !strings.Contains(out, "krita (deb 5.2): kept for when it comes back: start it: never") {
		t.Errorf("%s", out)
	}
}

func TestTheOverviewNamesOnlyTheProgramsWithAChoice(t *testing.T) {
	inv := withRows(sampleInventory())
	blender := rowOf(t, inv, programs.Flatpak, "org.blender.Blender")

	out, _ := ListPrograms{Programs: choosing{inventory: inv, choices: programs.Choices{Honoured: true}}}.Execute(context.Background(), nil)
	if !strings.Contains(out, "No program has a choice of its own") {
		t.Errorf("no choices: %s", out)
	}

	c := choosing{inventory: inv, choices: programs.Choices{Honoured: true, Permits: map[int64][]programs.Permit{
		blender.RowID: {{Verb: programs.VerbChange, Answer: programs.Always, Version: "4.5"}},
	}}}

	out, _ = ListPrograms{Programs: c}.Execute(context.Background(), nil)
	if !strings.Contains(out, "Choices of their own: Blender (flatpak 4.5): run it as a command: always.") {
		t.Errorf("%s", out)
	}

	if strings.Contains(out, "GIMP (menu): start it") {
		t.Errorf("a program without a choice was listed: %s", out)
	}
}

func TestListProgramsSaysWhenEverythingIsStopped(t *testing.T) {
	since := time.Date(2026, 11, 14, 9, 30, 0, 0, time.Local)
	c := choosing{inventory: withRows(sampleInventory()), choices: programs.Choices{Honoured: true, Stopped: true, Since: since}}

	for _, args := range []string{``, `{"name":"gimp"}`, `{"removed":true}`} {
		out, _ := ListPrograms{Programs: c}.Execute(context.Background(), json.RawMessage(args))

		if !strings.HasPrefix(out, "Stopped: the owner stopped all use of other programs on 14 Nov 2026 at 09:30") {
			t.Errorf("%s: %s", args, out)
		}
	}
}

// An inventory without choices (older callers) still answers as before.
func TestListProgramsWithoutChoicesAnswersAsBefore(t *testing.T) {
	out, _ := ListPrograms{Programs: sampleInventory()}.Execute(context.Background(), json.RawMessage(`{"name":"gimp"}`))
	if strings.Contains(out, "start it") {
		t.Errorf("%s", out)
	}
}
