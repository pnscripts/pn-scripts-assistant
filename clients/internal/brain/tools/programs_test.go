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
