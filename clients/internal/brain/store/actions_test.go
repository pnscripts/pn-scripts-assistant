package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The actions its person wrote for other programs: kept with their record,
// never two of one name, taken away with their record, and not readable on
// disk in a sealed brain.
func TestTheActionsForOtherProgramsAreKeptWithTheirRecord(t *testing.T) {
	d := open(t)

	written := Deed{Tool: "other_programs_action", Summary: "written: backup", Outcome: "done", Changes: true}

	a, err := d.AddOutsideAction("backup", "rsync -a ~/work /mnt/backup", 3, written)
	if err != nil || a.ID == 0 || a.Name != "backup" || a.Command != "rsync -a ~/work /mnt/backup" || a.CreatedAt.IsZero() {
		t.Fatalf("the action: %+v, %v", a, err)
	}

	if _, err := d.AddOutsideAction("backup", "true", 3, written); !errors.Is(err, ErrActionExists) {
		t.Errorf("a second of the same name: %v", err)
	}

	for _, bad := range [][2]string{{"", "true"}, {"x", ""}} {
		if _, err := d.AddOutsideAction(bad[0], bad[1], 3, written); err == nil {
			t.Errorf("an action %q / %q was kept", bad[0], bad[1])
		}
	}

	d.AddOutsideAction("two", "true", 3, written)
	d.AddOutsideAction("three", "true", 3, written)

	if _, err := d.AddOutsideAction("four", "true", 3, written); !errors.Is(err, ErrActionsFull) {
		t.Errorf("one over: %v", err)
	}

	actions, err := d.OutsideActions()
	if err != nil || len(actions) != 3 || actions[0].Name != "backup" || actions[2].Name != "three" {
		t.Fatalf("the list: %+v, %v", actions, err)
	}

	// Each one kept wrote one line; each one refused wrote none.
	deeds, _ := d.Deeds(DeedFilter{Limit: 20})
	if len(deeds) != 3 {
		t.Fatalf("%d lines on the record for three actions kept", len(deeds))
	}

	var said OutsideAction

	if err := d.RemoveOutsideAction(a.ID, func(gone OutsideAction) Deed {
		said = gone

		return Deed{Tool: "other_programs_action", Summary: "taken away: " + gone.Name, Outcome: "done", Changes: true}
	}); err != nil || said.Name != "backup" {
		t.Fatalf("taking one away: %v, %+v", err, said)
	}

	if err := d.RemoveOutsideAction(a.ID, func(OutsideAction) Deed { return Deed{} }); !errors.Is(err, ErrNoAction) {
		t.Errorf("taking away what is gone: %v", err)
	}

	if actions, _ := d.OutsideActions(); len(actions) != 2 {
		t.Errorf("after taking one away: %+v", actions)
	}

	if deeds, _ := d.Deeds(DeedFilter{Limit: 20}); len(deeds) != 4 || deeds[0].Summary != "taken away: backup" {
		t.Errorf("the record after taking one away: %+v", deeds)
	}
}

// No record, no action: a list whose change cannot be written down is not
// changed.
func TestAnActionThatCannotBeRecordedIsNotKept(t *testing.T) {
	d := open(t)

	if _, err := d.AddOutsideAction("backup", "true", 3, Deed{Tool: "other_programs_action", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.sql().Exec(`ALTER TABLE deeds RENAME TO deeds_away`); err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddOutsideAction("second", "true", 3, Deed{Tool: "other_programs_action", Outcome: "done"}); err == nil {
		t.Error("an action was kept without its line on the record")
	}

	actions, _ := d.OutsideActions()

	if err := d.RemoveOutsideAction(actions[0].ID, func(OutsideAction) Deed { return Deed{Tool: "other_programs_action", Outcome: "done"} }); err == nil {
		t.Error("an action was taken away without its line on the record")
	}

	if actions, _ := d.OutsideActions(); len(actions) != 1 || actions[0].Name != "backup" {
		t.Errorf("the list changed without a record: %+v", actions)
	}
}

func TestAnActionIsNotReadableOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddOutsideAction("clinic-zzq", "curl https://clinic-zzq.example/visit", 3, Deed{Tool: "other_programs_action", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddOutsideAction("clinic-zzq", "true", 3, Deed{}); !errors.Is(err, ErrActionExists) {
		t.Errorf("a second of the same name in a sealed brain: %v", err)
	}

	actions, _ := d.OutsideActions()
	if len(actions) != 1 || actions[0].Name != "clinic-zzq" || actions[0].Command != "curl https://clinic-zzq.example/visit" {
		t.Fatalf("read back: %+v", actions)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	if strings.Contains(string(raw)+string(wal), "clinic-zzq") {
		t.Error("an action is readable on disk")
	}
}

// How something written down as started ended.
func TestFinishingADeedSaysHowItEnded(t *testing.T) {
	d := open(t)

	id, err := d.AddDeed(Deed{Tool: "run_action", Summary: "x", Asked: "Action: backup", Result: "Started.", Outcome: "waiting", AskedBy: "p"})
	if err != nil {
		t.Fatal(err)
	}

	if err := d.FinishDeed(id, "failed", "Ended with exit code 3 after 1.2 s.", 1200); err != nil {
		t.Fatal(err)
	}

	deeds, _ := d.Deeds(DeedFilter{Limit: 5})
	if len(deeds) != 1 || deeds[0].Outcome != "failed" || deeds[0].Result != "Ended with exit code 3 after 1.2 s." || deeds[0].Millis != 1200 ||
		deeds[0].Asked != "Action: backup" || deeds[0].AskedBy != "p" {
		t.Errorf("the line: %+v", deeds)
	}

	if err := d.FinishDeed(id+100, "done", "x", 1); err == nil {
		t.Error("a line that is not there was finished")
	}
}
