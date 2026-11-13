package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

func TestTheProgramInventoryIsKeptAndChanged(t *testing.T) {
	db := openDeeds(t)

	day1 := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

	save, _ := programs.Reconcile(programs.Deb, nil, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"},
		{Kind: programs.Deb, ID: "krita", Name: "krita", Version: "5.2"},
	}, day1)

	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	snaps, _ := programs.Reconcile(programs.Snap, nil, []programs.Program{
		{Kind: programs.Snap, ID: "firefox", Name: "Firefox", Version: "145", Path: "/snap/firefox"},
	}, day1)

	if err := db.SavePrograms(snaps); err != nil {
		t.Fatal(err)
	}

	debs, err := db.Programs(programs.Deb)
	if err != nil {
		t.Fatal(err)
	}

	if len(debs) != 2 {
		t.Fatalf("debs %+v", debs)
	}

	for _, r := range debs {
		if r.RowID == 0 || !r.Present || !r.FirstSeen.Equal(day1) || r.Kind != programs.Deb {
			t.Errorf("read back %+v", r)
		}
	}

	all, _ := db.Programs("")
	if len(all) != 3 {
		t.Errorf("every kind: %d", len(all))
	}

	// krita removed: the stored row is updated, not added again.
	day2 := day1.Add(24 * time.Hour)

	save, changes := programs.Reconcile(programs.Deb, debs, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"},
	}, day2)

	if len(changes) != 1 || changes[0].Kind != programs.Removed {
		t.Fatalf("changes %+v", changes)
	}

	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	debs, _ = db.Programs(programs.Deb)
	if len(debs) != 2 {
		t.Fatalf("a removed program is remembered, once: %+v", debs)
	}

	for _, r := range debs {
		if r.ID == "krita" && (r.Present || r.RemovedAt == nil || !r.RemovedAt.Equal(day2) || r.Version != "5.2") {
			t.Errorf("removed krita %+v", r)
		}
	}

	// Back with a new version: the same row, present again.
	save, changes = programs.Reconcile(programs.Deb, debs, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"},
		{Kind: programs.Deb, ID: "krita", Name: "krita", Version: "5.3"},
	}, day2.Add(time.Hour))

	if len(changes) != 1 || changes[0].Kind != programs.Returned || changes[0].WasRemovedAt == nil {
		t.Fatalf("changes %+v", changes)
	}

	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	debs, _ = db.Programs(programs.Deb)
	for _, r := range debs {
		if r.ID == "krita" && (!r.Present || r.RemovedAt != nil || r.Version != "5.3" || r.PreviousVersion != "5.2") {
			t.Errorf("returned krita %+v", r)
		}
	}

	if len(debs) != 2 {
		t.Errorf("still two rows: %d", len(debs))
	}
}

func TestSavingAProgramNobodyKnowsIsRefused(t *testing.T) {
	db := openDeeds(t)

	if err := db.SavePrograms([]programs.Record{{Program: programs.Program{Kind: programs.Deb, ID: "x"}, RowID: 999}}); err == nil {
		t.Error("an update of a row that is not there must fail, not pass silently")
	}

	if err := db.SavePrograms([]programs.Record{{Program: programs.Program{Kind: programs.Deb}}}); err == nil {
		t.Error("a program without an id was saved")
	}
}

// Which programs somebody has says things about them (a clinic's software,
// a game, a dating app): sealed like the rest.
func TestTheProgramInventoryIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	save, _ := programs.Reconcile(programs.App, nil, []programs.Program{{
		Kind: programs.App, ID: "secret-clinic.desktop", Name: "Private Clinic Records",
		Exec: "clinic-records-bin", Path: "/opt/clinicsoft/entry.desktop",
	}}, time.Now())

	if err := d.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	back, _ := d.Programs(programs.App)
	if len(back) != 1 || back[0].Name != "Private Clinic Records" || back[0].ID != "secret-clinic.desktop" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	for _, word := range []string{"secret-clinic", "Private Clinic", "clinic-records-bin", "clinicsoft"} {
		if strings.Contains(string(raw)+string(wal), word) {
			t.Errorf("%q is readable on disk", word)
		}
	}
}
