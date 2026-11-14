package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

// "Stop using other programs" is one standing switch: on with the time it
// was pressed, off again, and the same after the brain is opened again.
func TestStoppingProgramsIsRememberedUntilUndone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, stopped, err := d.ProgramsStopped(); err != nil || stopped {
		t.Fatalf("a new brain starts stopped (%v, %v)", stopped, err)
	}

	at := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	if err := d.StopPrograms(at); err != nil {
		t.Fatal(err)
	}

	// Pressed again later: still since the first time.
	if err := d.StopPrograms(at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	d.Close()

	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}

	defer d.Close()

	since, stopped, err := d.ProgramsStopped()
	if err != nil || !stopped || !since.Equal(at) {
		t.Fatalf("after opening again: %v %v %v", since, stopped, err)
	}

	if err := d.UseProgramsAgain(); err != nil {
		t.Fatal(err)
	}

	if _, stopped, _ := d.ProgramsStopped(); stopped {
		t.Error("still stopped after being undone")
	}

	// Undoing what is not stopped is fine.
	if err := d.UseProgramsAgain(); err != nil {
		t.Error(err)
	}
}

// A question about a program keeps which program, which use, the version
// it was asked about and whether that program's Always would answer it.
func TestAQuestionKeepsItsProgram(t *testing.T) {
	d := openDeeds(t)
	krita := oneProgram(t, d, programs.Deb, "krita", "5.2")

	id, err := d.RecordInvocation(1, "open_app", `{"name":"krita"}`, "Krita: Open krita", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	if err := d.InvocationProgram(id, krita.RowID, programs.VerbStart, "5.2", true); err != nil {
		t.Fatal(err)
	}

	one, _ := d.Invocation(id)
	if one.ProgramID != krita.RowID || one.ProgramVerb != "start" || one.ProgramVersion != "5.2" || !one.ProgramAlways {
		t.Fatalf("one: %+v", one)
	}

	pending, _ := d.PendingInvocations()
	if len(pending) != 1 || pending[0].ProgramID != krita.RowID || !pending[0].ProgramAlways || pending[0].ProgramVersion != "5.2" {
		t.Fatalf("pending: %+v", pending)
	}

	// Only while it waits.
	if _, err := d.DecideInvocation(id, InvocationDenied); err != nil {
		t.Fatal(err)
	}

	other := oneProgram(t, d, programs.Deb, "gimp", "2.10")
	if err := d.InvocationProgram(id, other.RowID, programs.VerbStart, "2.10", true); err != nil {
		t.Fatal(err)
	}

	if one, _ = d.Invocation(id); one.ProgramID != krita.RowID {
		t.Errorf("a decided question was given another program: %+v", one)
	}

	if err := d.InvocationProgram(id, 0, programs.VerbStart, "", true); err == nil {
		t.Error("no program was accepted")
	}

	if err := d.InvocationProgram(id, krita.RowID, "format-disk", "", true); err == nil {
		t.Error("an unknown use was accepted")
	}
}

func TestTheVersionAQuestionWasAskedAboutIsSealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	d, err := OpenSealed(path, sealKey(t))
	if err != nil {
		t.Fatal(err)
	}

	r := oneProgram(t, d, programs.Deb, "clinic-records", "8.8.8-privatebuild")

	id, _ := d.RecordInvocation(1, "open_app", `{}`, "x", "mutating")
	if err := d.InvocationProgram(id, r.RowID, programs.VerbStart, "8.8.8-privatebuild", true); err != nil {
		t.Fatal(err)
	}

	back, _ := d.Invocation(id)
	if back.ProgramVersion != "8.8.8-privatebuild" {
		t.Fatalf("%+v", back)
	}

	d.Close()

	raw, _ := os.ReadFile(path)
	wal, _ := os.ReadFile(path + "-wal")

	if strings.Contains(string(raw)+string(wal), "privatebuild") {
		t.Error("the version a question was asked about is readable on disk")
	}
}
