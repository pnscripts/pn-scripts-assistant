package brain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * From the command the model writes to the ruling the loop acts on: a file
 * that only shares a program's name is not answered by that program's
 * "Always", and is not shown under its name (1.1.0 security review). The
 * program's own "never" still refuses it.
 */
func TestACommandForASameNamedFileIsNotAnsweredByTheProgramsCard(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Freedom = string(permits.WhatIveAllowed)

	const name = "zz-samename-demo"

	save, _ := programs.Reconcile(programs.App, nil,
		[]programs.Program{{Kind: programs.App, ID: name + ".desktop", Name: "Same Name Demo", Exec: name}}, time.Now())

	if err := b.DB.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	if _, err := b.SetProgramPermit(programs.App, name+".desktop", programs.VerbChange, programs.Always); err != nil {
		t.Fatal(err)
	}

	// A file of that name in a folder anybody can write, runnable.
	folder := t.TempDir()
	file := filepath.Join(folder, name)

	if err := os.WriteFile(file, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ruling := func(argv []string, dir string) programs.Ruling {
		raw, _ := json.Marshal(map[string]any{"argv": argv, "dir": dir})

		use, bound := tools.ProgramUseOf(context.Background(), tools.RunCommand{}, raw)
		if !bound {
			t.Fatal("run_command no longer says which program it runs")
		}

		return b.ProgramRuling(context.Background(), use)
	}

	for _, c := range []struct {
		what string
		argv []string
		dir  string
	}{
		{"by its full place", []string{file, "--batch"}, ""},
		{"from its folder", []string{"./" + name}, folder},
		{"behind env", []string{"env", "-i", file}, ""},
		{"behind a shell", []string{"sh", "-c", file + " --batch"}, ""},
	} {
		r := ruling(c.argv, c.dir)

		if r.Answer != programs.AskEach || r.Program != nil {
			t.Errorf("a same-named file %s was answered by the program's card: %+v", c.what, r)
		}
	}

	if _, err := b.SetProgramPermit(programs.App, name+".desktop", programs.VerbChange, programs.Never); err != nil {
		t.Fatal(err)
	}

	if r := ruling([]string{file}, ""); r.Answer != programs.Never {
		t.Errorf("the program's never did not reach a same-named file: %+v", r)
	}
}
