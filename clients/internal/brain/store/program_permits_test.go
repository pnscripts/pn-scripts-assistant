package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

func oneProgram(t *testing.T, d *DB, kind programs.Kind, id, version string) programs.Record {
	t.Helper()

	save, _ := programs.Reconcile(kind, nil, []programs.Program{{Kind: kind, ID: id, Name: id, Version: version}}, time.Now())
	if err := d.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	all, err := d.Programs(kind)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range all {
		if r.ID == id {
			return r
		}
	}

	t.Fatalf("%s was not written", id)

	return programs.Record{}
}

func TestAProgramsPermitsAreKeptChangedAndCleared(t *testing.T) {
	d := openDeeds(t)
	krita := oneProgram(t, d, programs.Deb, "krita", "5.2")
	gimp := oneProgram(t, d, programs.Deb, "gimp", "2.10")

	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

	if err := d.SetProgramPermit(krita.RowID, programs.VerbStart, programs.Always, "5.2", at); err != nil {
		t.Fatal(err)
	}

	if err := d.SetProgramPermit(krita.RowID, programs.VerbInput, programs.Never, "5.2", at); err != nil {
		t.Fatal(err)
	}

	if err := d.SetProgramPermit(gimp.RowID, programs.VerbChange, programs.Never, "2.10", at); err != nil {
		t.Fatal(err)
	}

	// Changed in place: one row per program and verb.
	later := at.Add(time.Hour)
	if err := d.SetProgramPermit(krita.RowID, programs.VerbInput, programs.Always, "5.3", later); err != nil {
		t.Fatal(err)
	}

	all, err := d.ProgramPermits()
	if err != nil {
		t.Fatal(err)
	}

	k := all[krita.RowID]
	if len(k) != 2 || k[0].Verb != programs.VerbStart || k[1].Verb != programs.VerbInput ||
		k[1].Answer != programs.Always || k[1].Version != "5.3" || !k[1].At.Equal(later) {
		t.Fatalf("krita's permits %+v", k)
	}

	if len(all[gimp.RowID]) != 1 {
		t.Fatalf("gimp's permits %+v", all[gimp.RowID])
	}

	// Asking each time is no row at all.
	if err := d.SetProgramPermit(krita.RowID, programs.VerbStart, programs.AskEach, "", later); err != nil {
		t.Fatal(err)
	}

	all, _ = d.ProgramPermits()
	if len(all[krita.RowID]) != 1 || all[krita.RowID][0].Verb != programs.VerbInput {
		t.Fatalf("ask each time left a row: %+v", all[krita.RowID])
	}
}

func TestAPermitNeedsAKnownProgramVerbAndAnswer(t *testing.T) {
	d := openDeeds(t)
	krita := oneProgram(t, d, programs.Deb, "krita", "5.2")
	now := time.Now()

	if err := d.SetProgramPermit(krita.RowID+100, programs.VerbStart, programs.Never, "", now); err == nil {
		t.Error("a permit for no program was written")
	}

	if err := d.SetProgramPermit(krita.RowID, "format-disk", programs.Never, "", now); err == nil {
		t.Error("an unknown verb was written")
	}

	if err := d.SetProgramPermit(krita.RowID, programs.VerbStart, "maybe", "", now); err == nil {
		t.Error("an unknown answer was written")
	}
}

// A removed program keeps its permits: the decision is the person's, and it
// counts again when the program comes back.
func TestARemovedProgramKeepsItsPermits(t *testing.T) {
	d := openDeeds(t)
	krita := oneProgram(t, d, programs.Deb, "krita", "5.2")

	if err := d.SetProgramPermit(krita.RowID, programs.VerbStart, programs.Never, "5.2", time.Now()); err != nil {
		t.Fatal(err)
	}

	known, _ := d.Programs(programs.Deb)
	save, _ := programs.Reconcile(programs.Deb, known, nil, time.Now())

	if err := d.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	all, _ := d.ProgramPermits()
	if len(all[krita.RowID]) != 1 {
		t.Fatalf("removal took the permit: %+v", all)
	}
}

func TestProgramPermitsAreSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	r := oneProgram(t, d, programs.Deb, "clinic-records", "7.7.7-privatebuild")

	if err := d.SetProgramPermit(r.RowID, programs.VerbStart, programs.Always, "7.7.7-privatebuild", time.Now()); err != nil {
		t.Fatal(err)
	}

	back, _ := d.ProgramPermits()
	if len(back[r.RowID]) != 1 || back[r.RowID][0].Version != "7.7.7-privatebuild" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	if strings.Contains(string(raw)+string(wal), "privatebuild") {
		t.Error("the version an always was given to is readable on disk")
	}
}
