package programs

import (
	"testing"
	"time"
)

func deb(id, version string) Program { return Program{Kind: Deb, ID: id, Name: id, Version: version} }

func byKey(rs []Record) map[string]Record {
	out := map[string]Record{}
	for _, r := range rs {
		out[r.Key()] = r
	}

	return out
}

func kinds(cs []Change) map[string]ChangeKind {
	out := map[string]ChangeKind{}
	for _, c := range cs {
		out[c.Program.Key()] = c.Kind
	}

	return out
}

func TestTheFirstLookRecordsEverythingAsAdded(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

	save, changes := Reconcile(Deb, nil, []Program{deb("gimp", "2.10"), deb("ffmpeg", "6.1")}, now)

	if len(save) != 2 || len(changes) != 2 {
		t.Fatalf("save %d changes %d", len(save), len(changes))
	}

	for _, r := range save {
		if !r.Present || !r.FirstSeen.Equal(now) || !r.ChangedAt.Equal(now) || r.RemovedAt != nil || r.RowID != 0 {
			t.Errorf("new record %+v", r)
		}
	}

	for _, c := range changes {
		if c.Kind != Added {
			t.Errorf("%s: %s", c.Program.Key(), c.Kind)
		}
	}
}

func TestRemovalReinstallAndUpgradeAreToldApart(t *testing.T) {
	day1 := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	day3 := day2.Add(24 * time.Hour)
	day4 := day3.Add(24 * time.Hour)

	known, _ := Reconcile(Deb, nil, []Program{deb("gimp", "2.10"), deb("krita", "5.2"), deb("ffmpeg", "6.1")}, day1)
	for i := range known {
		known[i].RowID = int64(i + 1)
	}

	// Day 2: krita removed, ffmpeg upgraded, gimp unchanged.
	save, changes := Reconcile(Deb, known, []Program{deb("gimp", "2.10"), deb("ffmpeg", "7.0")}, day2)

	got := kinds(changes)
	if len(changes) != 2 || got["deb:krita"] != Removed || got["deb:ffmpeg"] != Updated {
		t.Fatalf("day 2 changes %v", got)
	}

	saved := byKey(save)
	if _, ok := saved["deb:gimp"]; ok {
		t.Error("an unchanged program is not written again")
	}

	krita := saved["deb:krita"]
	if krita.Present || krita.RemovedAt == nil || !krita.RemovedAt.Equal(day2) || krita.RowID == 0 {
		t.Errorf("removed krita %+v", krita)
	}

	if krita.Version != "5.2" {
		t.Errorf("a removed program keeps the version it had: %q", krita.Version)
	}

	ff := saved["deb:ffmpeg"]
	if ff.Version != "7.0" || ff.PreviousVersion != "6.1" || !ff.ChangedAt.Equal(day2) || !ff.FirstSeen.Equal(day1) {
		t.Errorf("upgraded ffmpeg %+v", ff)
	}

	for _, c := range changes {
		if c.Kind == Updated && c.From != "6.1" {
			t.Errorf("an update says where it came from: %q", c.From)
		}

		if c.Kind == Removed && !c.At.Equal(day2) {
			t.Errorf("removal time %v", c.At)
		}
	}

	known = merge(known, save)

	// Day 3: nothing seen of krita again; a removed program stays removed
	// and is not "removed" a second time.
	save, changes = Reconcile(Deb, known, []Program{deb("gimp", "2.10"), deb("ffmpeg", "7.0")}, day3)
	if len(save) != 0 || len(changes) != 0 {
		t.Fatalf("day 3 should change nothing: %v %v", save, changes)
	}

	// Day 4: krita comes back with the same version, then a second program
	// comes back with a new one.
	known = merge(known, save)

	save, changes = Reconcile(Deb, known, []Program{deb("gimp", "2.10"), deb("ffmpeg", "7.0"), deb("krita", "5.2")}, day4)
	if len(changes) != 1 || changes[0].Kind != Returned || changes[0].From != "" {
		t.Fatalf("same version back: %+v", changes)
	}

	if changes[0].WasRemovedAt == nil || !changes[0].WasRemovedAt.Equal(day2) {
		t.Errorf("a return says when it had gone: %v", changes[0].WasRemovedAt)
	}

	back := byKey(save)["deb:krita"]
	if !back.Present || back.RemovedAt != nil || !back.FirstSeen.Equal(day1) || back.RowID == 0 {
		t.Errorf("returned krita %+v", back)
	}

	known = merge(known, save)

	// Removed once more, then back with a new version.
	save, _ = Reconcile(Deb, known, []Program{deb("gimp", "2.10"), deb("ffmpeg", "7.0")}, day4.Add(time.Hour))
	known = merge(known, save)

	save, changes = Reconcile(Deb, known, []Program{deb("gimp", "2.10"), deb("ffmpeg", "7.0"), deb("krita", "5.3")}, day4.Add(2*time.Hour))
	if len(changes) != 1 || changes[0].Kind != Returned || changes[0].From != "5.2" {
		t.Fatalf("new version back: %+v", changes)
	}

	if r := byKey(save)["deb:krita"]; r.Version != "5.3" || r.PreviousVersion != "5.2" {
		t.Errorf("returned with a new version %+v", r)
	}
}

// merge applies saved records over known ones, as the store does.
func merge(known, save []Record) []Record {
	idx := map[string]int{}
	for i, r := range known {
		idx[r.Key()] = i
	}

	out := append([]Record(nil), known...)

	for _, r := range save {
		if i, ok := idx[r.Key()]; ok {
			out[i] = r

			continue
		}

		r.RowID = int64(len(out) + 1)
		out = append(out, r)
	}

	return out
}

func TestOnlyOneSourceIsJudgedAtATime(t *testing.T) {
	now := time.Now()

	known := []Record{
		{Program: Program{Kind: Snap, ID: "firefox", Name: "firefox", Version: "1"}, RowID: 1, Present: true},
		{Program: deb("gimp", "2.10"), RowID: 2, Present: true},
	}

	// The dpkg list says nothing of snaps: firefox must not be "removed".
	save, changes := Reconcile(Deb, known, []Program{deb("gimp", "2.10")}, now)
	if len(save) != 0 || len(changes) != 0 {
		t.Fatalf("other sources untouched: %v %v", save, changes)
	}

	// A program of another kind among the seen ones is ignored.
	save, _ = Reconcile(Deb, known, []Program{deb("gimp", "2.10"), {Kind: App, ID: "x.desktop", Name: "X"}}, now)
	if len(save) != 0 {
		t.Errorf("a stray kind was saved: %v", save)
	}
}

func TestQuietDetailsAreKeptWithoutCallingThemAChange(t *testing.T) {
	now := time.Now()
	known := []Record{{Program: Program{Kind: App, ID: "g.desktop", Name: "GIMP", Exec: "gimp", Path: "/a"}, RowID: 7, Present: true}}

	save, changes := Reconcile(App, known, []Program{{Kind: App, ID: "g.desktop", Name: "GIMP 3", Exec: "gimp-3", Path: "/b"}}, now)
	if len(changes) != 0 {
		t.Errorf("a renamed menu entry is not news: %v", changes)
	}

	if len(save) != 1 || save[0].Name != "GIMP 3" || save[0].Exec != "gimp-3" || save[0].Path != "/b" || save[0].RowID != 7 {
		t.Errorf("but it is written down: %+v", save)
	}
}

func TestTheSameProgramSeenTwiceCountsOnce(t *testing.T) {
	save, changes := Reconcile(Deb, nil, []Program{deb("a", "1"), deb("a", "2"), {Kind: Deb, ID: ""}}, time.Now())
	if len(save) != 1 || len(changes) != 1 || save[0].Version != "1" {
		t.Errorf("first wins, empty ids dropped: %+v", save)
	}
}
