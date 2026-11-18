package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A deed and a waiting question keep which program asked, and one asked at
// the desk keeps nothing there.
func TestWhoAskedIsKeptWithTheDeedAndTheQuestion(t *testing.T) {
	d := open(t)

	if _, err := d.AddDeed(Deed{Tool: "list_programs", Summary: "Another program asked which programs are installed",
		AskedBy: "claude-code 2.1.0", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddDeed(Deed{Tool: "read_file", Summary: "Read notes.txt", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	deeds, err := d.Deeds(DeedFilter{})
	if err != nil || len(deeds) != 2 {
		t.Fatalf("%v %+v", err, deeds)
	}

	if deeds[0].AskedBy != "" || deeds[1].AskedBy != "claude-code 2.1.0" {
		t.Errorf("who asked: %q, %q", deeds[0].AskedBy, deeds[1].AskedBy)
	}

	// Only what other programs asked.
	theirs, err := d.Deeds(DeedFilter{Outside: true})
	if err != nil || len(theirs) != 1 || theirs[0].Tool != "list_programs" {
		t.Fatalf("what other programs asked: %v %+v", err, theirs)
	}

	id, _ := d.RecordInvocation(1, "write_file", `{}`, "Write a file", "mutating")
	mine, _ := d.RecordInvocation(1, "write_file", `{}`, "Write a file", "mutating")

	if err := d.InvocationAskedBy(id, "Cursor 3.0"); err != nil {
		t.Fatal(err)
	}

	one, _ := d.Invocation(id)
	other, _ := d.Invocation(mine)

	if one.AskedBy != "Cursor 3.0" || other.AskedBy != "" {
		t.Errorf("the questions: %q, %q", one.AskedBy, other.AskedBy)
	}

	pending, _ := d.PendingInvocations()
	if len(pending) != 2 || pending[0].AskedBy != "Cursor 3.0" || pending[1].AskedBy != "" {
		t.Errorf("waiting: %+v", pending)
	}

	// Who asked is settled when the question is put, not after the answer.
	d.DecideInvocation(id, InvocationApproved)

	if err := d.InvocationAskedBy(id, "somebody else"); err != nil {
		t.Fatal(err)
	}

	if one, _ = d.Invocation(id); one.AskedBy != "Cursor 3.0" {
		t.Errorf("who asked changed after the decision: %q", one.AskedBy)
	}

	if err := d.InvocationAskedBy(mine, ""); err == nil {
		t.Error("nobody was accepted as the program that asked")
	}
}

func TestTheNameOfTheProgramThatAskedIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.AddDeed(Deed{Tool: "list_programs", Summary: "x", AskedBy: "clinic-helper-zzq 1", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	id, _ := d.RecordInvocation(1, "write_file", `{}`, "x", "mutating")
	if err := d.InvocationAskedBy(id, "clinic-helper-zzq 1"); err != nil {
		t.Fatal(err)
	}

	deeds, _ := d.Deeds(DeedFilter{Outside: true})
	back, _ := d.Invocation(id)

	if len(deeds) != 1 || deeds[0].AskedBy != "clinic-helper-zzq 1" || back.AskedBy != "clinic-helper-zzq 1" {
		t.Fatalf("%+v %+v", deeds, back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	if strings.Contains(string(raw)+string(wal), "helper-zzq") {
		t.Error("the name of the program that asked is readable on disk")
	}
}

// What other programs may ask: nothing until allowed, remembered with the
// time, taken back at once.
func TestWhatOtherProgramsMayAskIsOffUntilAllowed(t *testing.T) {
	d := open(t)

	may, err := d.OutsideAllowed()
	if err != nil || len(may) != 0 {
		t.Fatalf("allowed before anybody allowed anything: %v %v", may, err)
	}

	first := time.Now().Add(-time.Hour).Truncate(time.Second)

	if err := d.AllowOutside("programs", first); err != nil {
		t.Fatal(err)
	}

	// Allowed again while allowed keeps the first time.
	if err := d.AllowOutside("programs", time.Now()); err != nil {
		t.Fatal(err)
	}

	may, err = d.OutsideAllowed()
	if err != nil || len(may) != 1 || !may["programs"].Equal(first) {
		t.Fatalf("allowed: %v %v", may, err)
	}

	if err := d.DisallowOutside("programs"); err != nil {
		t.Fatal(err)
	}

	// Taking back what is not allowed is fine.
	if err := d.DisallowOutside("programs"); err != nil {
		t.Fatal(err)
	}

	if may, _ = d.OutsideAllowed(); len(may) != 0 {
		t.Errorf("still allowed: %v", may)
	}

	if err := d.AllowOutside("", time.Now()); err == nil {
		t.Error("a switch with no name was kept")
	}
}

// A switch whose time cannot be read is not a yes.
func TestASwitchThatCannotBeReadIsNotAllowed(t *testing.T) {
	d := open(t)

	if _, err := d.sql().Exec(`INSERT INTO outside_may (subject, since) VALUES ('programs', 'yesterday')`); err != nil {
		t.Fatal(err)
	}

	if may, err := d.OutsideAllowed(); err == nil || len(may) != 0 {
		t.Errorf("read as allowed: %v %v", may, err)
	}
}

/*
 * A name that looks like the store's own sealed text is not kept as one.
 * Text beginning with the sealed mark is written as it is (it is taken
 * for already sealed) and then cannot be read back, which would take the
 * whole record with it; and the name is whatever a program chose to send.
 */
func TestANameThatLooksSealedDoesNotBreakTheRecord(t *testing.T) {
	for name, opened := range map[string]func(*testing.T) *DB{
		"a sealed brain": func(t *testing.T) *DB {
			d, err := OpenSealed(filepath.Join(t.TempDir(), "brain.sqlite"), sealKey(t))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { d.Close() })

			return d
		},
		"a brain with no key": open,
	} {
		t.Run(name, func(t *testing.T) {
			d := opened(t)

			for _, by := range []string{"pnsealed1:x", "pnsealed1:pnsealed1:AAAA", "pnsealed1:", "  pnsealed1:pnsealed1:"} {
				if _, err := d.AddDeed(Deed{Tool: "assistant_status", Summary: "x", AskedBy: by, Outcome: "done"}); err != nil {
					t.Fatal(err)
				}
			}

			deeds, err := d.Deeds(DeedFilter{Outside: true})
			if err != nil {
				t.Fatalf("the record cannot be read any more: %v", err)
			}

			if len(deeds) != 4 || deeds[3].AskedBy != "x" || deeds[2].AskedBy != "AAAA" || deeds[1].AskedBy != NoName || deeds[0].AskedBy != NoName {
				t.Errorf("the names kept: %q %q %q %q", deeds[3].AskedBy, deeds[2].AskedBy, deeds[1].AskedBy, deeds[0].AskedBy)
			}

			id, _ := d.RecordInvocation(1, "write_file", `{}`, "x", "mutating")
			if err := d.InvocationAskedBy(id, "pnsealed1:Cursor"); err != nil {
				t.Fatal(err)
			}

			pending, err := d.PendingInvocations()
			if err != nil || len(pending) != 1 || pending[0].AskedBy != "Cursor" {
				t.Errorf("the waiting question: %+v %v", pending, err)
			}

			other, _ := d.RecordInvocation(1, "write_file", `{}`, "x", "mutating")
			if err := d.InvocationAskedBy(other, "pnsealed1:"); err != nil {
				t.Fatal(err)
			}

			if q, err := d.Invocation(other); err != nil || q.AskedBy != NoName {
				t.Errorf("a name that is only the mark: %+v %v", q, err)
			}
		})
	}
}

/*
 * A change of what other programs may ask and its line on the record are
 * one thing: both happen or neither does. The record is how its person
 * sees a change they did not make, so a change with no record is refused.
 */
func TestASwitchIsNotTurnedWithoutItsRecord(t *testing.T) {
	d := open(t)
	now := time.Now()

	turned := Deed{Tool: "other_programs_switch", Summary: "Other programs may now ask which programs are installed", Outcome: "done", Changes: true}

	changed, err := d.SwitchOutside("programs", true, now, turned)
	if err != nil || !changed {
		t.Fatalf("turning it on: %v %v", changed, err)
	}

	// Turned on while on: nothing changes and nothing more is written.
	if changed, err = d.SwitchOutside("programs", true, now.Add(time.Hour), turned); err != nil || changed {
		t.Fatalf("turning on what is on: %v %v", changed, err)
	}

	deeds, _ := d.Deeds(DeedFilter{})
	may, _ := d.OutsideAllowed()

	if len(deeds) != 1 || deeds[0].Tool != "other_programs_switch" || !deeds[0].Changes || !may["programs"].Equal(now.UTC().Truncate(time.Nanosecond)) {
		t.Fatalf("after turning it on: %+v %v", deeds, may)
	}

	if changed, err = d.SwitchOutside("programs", false, now, turned); err != nil || !changed {
		t.Fatalf("turning it off: %v %v", changed, err)
	}

	if may, _ = d.OutsideAllowed(); len(may) != 0 {
		t.Fatalf("still on: %v", may)
	}

	// The record cannot be written: the switch stays where it was, both ways.
	if _, err := d.sql().Exec(`ALTER TABLE deeds RENAME TO deeds_away`); err != nil {
		t.Fatal(err)
	}

	if changed, err = d.SwitchOutside("programs", true, now, turned); err == nil || changed {
		t.Errorf("turned on with no record: %v %v", changed, err)
	}

	if may, _ = d.OutsideAllowed(); len(may) != 0 {
		t.Errorf("on, with no record of it: %v", may)
	}

	d.sql().Exec(`ALTER TABLE deeds_away RENAME TO deeds`)
	d.SwitchOutside("programs", true, now, turned)
	d.sql().Exec(`ALTER TABLE deeds RENAME TO deeds_away`)

	if changed, err = d.SwitchOutside("programs", false, now, turned); err == nil || changed {
		t.Errorf("turned off with no record: %v %v", changed, err)
	}

	if may, _ = d.OutsideAllowed(); len(may) != 1 {
		t.Errorf("off, with no record of it: %v", may)
	}

	if _, err := d.SwitchOutside("", true, now, turned); err == nil {
		t.Error("a switch with no name was turned")
	}
}
