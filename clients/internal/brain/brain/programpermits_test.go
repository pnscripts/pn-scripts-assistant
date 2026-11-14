package brain

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * A program's card decides, through real file events: "always" given to
 * one version, the program removed (refused with its date, the decision
 * kept), back with a newer version (the "always" sleeps), "never" set,
 * and "ask every time" not honouring a standing yes.
 */
func TestAProgramsCardDecidesAcrossRemovalAndReinstall(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Freedom = string(permits.WhatIveAllowed)

	m := &machineInAFolder{root: t.TempDir()}
	m.installSnap(t, "blender", "4.5.3", "60")
	m.setDpkg(t, "gimp\t2.10.36\tii \n")

	b.programs.scanner = m.scanner()
	b.programs.opts = programs.Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 200 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.KeepPrograms(ctx)
	defer b.StopPrograms()

	waitForPrograms(t, "the first look", func() bool { return len(inventoryOf(t, b)) == 2 })

	start := programs.Use{Verb: programs.VerbStart, Path: "/snap/bin/blender", Name: "blender"}

	if r := b.ProgramRuling(ctx, start); r.Answer != programs.AskEach || r.Why != programs.WhyUndecided || r.Program == nil {
		t.Fatalf("nothing decided yet: %+v", r)
	}

	if _, err := b.SetProgramPermit(programs.Snap, "blender", programs.VerbStart, programs.Always); err != nil {
		t.Fatal(err)
	}

	if r := b.ProgramRuling(ctx, start); r.Answer != programs.Always {
		t.Fatalf("always did not allow: %+v", r)
	}

	// On "ask every time" a standing yes is not honoured.
	b.Cfg.Freedom = string(permits.AskEveryTime)
	if r := b.ProgramRuling(ctx, start); r.Answer != programs.AskEach {
		t.Errorf("always was honoured on ask every time: %+v", r)
	}
	b.Cfg.Freedom = string(permits.WhatIveAllowed)

	// Removed: nothing of it is left but its name.
	m.removeSnap(t, "blender", "60")
	waitForPrograms(t, "the removal", func() bool { return !inventoryOf(t, b)["snap:blender"].Present })

	r := b.ProgramRuling(ctx, programs.Use{Verb: programs.VerbStart, Name: "blender"})
	if r.Answer != programs.Never || r.Why != programs.WhyGone || !strings.Contains(r.Said, "was removed from this computer on") {
		t.Fatalf("a removed program was not refused with its date: %+v", r)
	}

	if ps, _ := b.ProgramPermits(); len(ps[inventoryOf(t, b)["snap:blender"].RowID]) != 1 {
		t.Errorf("the decision went with the program: %+v", ps)
	}

	// Back, newer: the "always" sleeps until it is given again.
	m.installSnap(t, "blender", "4.5.4", "61")
	waitForPrograms(t, "the reinstall", func() bool { return inventoryOf(t, b)["snap:blender"].Version == "4.5.4" })

	if r := b.ProgramRuling(ctx, start); r.Answer != programs.AskEach || r.Why != programs.WhyUpdated {
		t.Errorf("an always for 4.5.3 allowed 4.5.4: %+v", r)
	}

	if _, err := b.SetProgramPermit(programs.Snap, "blender", programs.VerbStart, programs.Always); err != nil {
		t.Fatal(err)
	}

	if r := b.ProgramRuling(ctx, start); r.Answer != programs.Always {
		t.Errorf("given again, it did not allow: %+v", r)
	}

	// Never, for commands only.
	if _, err := b.SetProgramPermit(programs.Snap, "blender", programs.VerbChange, programs.Never); err != nil {
		t.Fatal(err)
	}

	cmd := programs.Use{Verb: programs.VerbChange, Path: "/snap/bin/blender", Name: "blender"}
	if r := b.ProgramRuling(ctx, cmd); r.Answer != programs.Never || r.Why != programs.WhyRefused {
		t.Errorf("never was not refused: %+v", r)
	}

	if _, err := b.SetProgramPermit(programs.Snap, "nothing", programs.VerbStart, programs.Never); err != ErrNoSuchProgram {
		t.Errorf("a card for no program: %v", err)
	}
}

// A package's decision is found through the file it owns, asked of dpkg
// only when some package has a decision.
func TestAPackagesDecisionFindsItsFile(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Freedom = string(permits.Everything)

	m := &machineInAFolder{root: t.TempDir()}
	m.setDpkg(t, "gimp\t2.10.36\tii \n")

	asked := 0
	s := m.scanner()
	inner := s.Run
	s.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "dpkg-query" && len(args) > 0 && args[0] == "-S" {
			asked++
			return []byte("gimp: " + args[1] + "\n"), nil
		}

		return inner(ctx, name, args...)
	}

	b.programs.scanner = s
	b.programs.opts = programs.Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 200 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.KeepPrograms(ctx)
	defer b.StopPrograms()

	waitForPrograms(t, "the first look", func() bool { return len(inventoryOf(t, b)) == 1 })

	use := programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/gimp-console-2.10", Name: "gimp-console-2.10"}

	if r := b.ProgramRuling(ctx, use); r.Answer != programs.AskEach || asked != 0 {
		t.Fatalf("decided, or asked dpkg, with nothing decided: %+v %d", r, asked)
	}

	if _, err := b.SetProgramPermit(programs.Deb, "gimp", programs.VerbChange, programs.Never); err != nil {
		t.Fatal(err)
	}

	if r := b.ProgramRuling(ctx, use); r.Answer != programs.Never || asked != 1 {
		t.Errorf("the package's never was not found through its file: %+v %d", r, asked)
	}
}
