package environs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A program that is on the PATH is found, and says which version it is.
func TestItFindsWhatIsInstalledAndAsksItsVersion(t *testing.T) {
	bin := t.TempDir()
	write(t, filepath.Join(bin, "pretend-engine"), "#!/bin/sh\necho 'Pretend Engine v4.3.1 stable'\n", 0o755)
	t.Setenv("PATH", bin)

	things := Look(context.Background(), []Program{{
		ID: "pretend", Title: "Pretend Engine", Kind: Engine,
		Names: []string{"pretend-engine"}, Version: []string{"--version"},
	}})

	if len(things) != 1 {
		t.Fatalf("expected one answer, got %d", len(things))
	}

	got := things[0]

	if got.State != Here {
		t.Errorf("an installed program is %q", got.State)
	}

	if got.Version != "4.3.1" {
		t.Errorf("the version is %q, not the number out of the line", got.Version)
	}

	if !strings.HasPrefix(got.Path, bin) {
		t.Errorf("it was found at %q", got.Path)
	}

	if got.Observed.IsZero() {
		t.Error("it did not say when it looked")
	}
}

// Something absent says so, and says what would bring it.
func TestSomethingAbsentSaysWhatItWouldTake(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	things := Look(context.Background(), []Program{{
		ID: "nothing", Title: "Nothing At All", Kind: Tool,
		Names: []string{"a-program-nobody-has"}, Needs: "sudo apt install nothing",
	}})

	if things[0].State != WantsInstalling {
		t.Errorf("a missing program is %q", things[0].State)
	}

	if things[0].Needs == "" {
		t.Error("it did not say what would bring it")
	}
}

/*
 * Here and unusable is its own answer.
 *
 * The same distinction provision makes about the things this program can
 * install: a file without its executable bit is not a machine that needs an
 * install, it is a machine that needs a chmod, and telling somebody to
 * download what they already have is the fault this avoids.
 */
func TestAProgramThatCannotRunIsNotCalledMissing(t *testing.T) {
	dir := t.TempDir()
	at := filepath.Join(dir, "stuck")
	write(t, at, "#!/bin/sh\n", 0o600)
	t.Setenv("PATH", t.TempDir())

	things := Look(context.Background(), []Program{{
		ID: "stuck", Title: "Stuck", Kind: Tool,
		Names: []string{"stuck"}, Places: []string{dir},
	}})

	if things[0].State != Blocked {
		t.Fatalf("an unusable program is %q", things[0].State)
	}

	if !strings.Contains(things[0].Why, "chmod") {
		t.Errorf("it did not say what to do: %q", things[0].Why)
	}
}

// What cannot be seen from the machine is supplied from above, and lands in
// the same list with the same shape.
func TestThingsThisPackageCannotSeeAreAddedBySources(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	world := New(Sources{
		Programs: []Program{},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				return []Thing{
					{ID: "anthropic", Kind: Service, Title: "Anthropic", State: WantsSetting,
						Needs: "an API key in the System tab"},
					{ID: "mailbox", Kind: Ability, Title: "Reading your mail", State: WantsSetting,
						Needs: "the mailbox address and password"},
				}
			},
		},
	})

	all := world.All(context.Background())

	if len(all) != 2 {
		t.Fatalf("expected the two supplied things, got %d", len(all))
	}

	for _, thing := range all {
		if thing.State != WantsSetting || thing.Needs == "" {
			t.Errorf("%s came through as %+v", thing.ID, thing)
		}
	}
}

/*
 * The better answer about one thing wins.
 *
 * Ollama is a program on the PATH and also the thing holding the models, and
 * two sources describe it honestly and differently. A source that found it
 * running knows more than one that looked for a file.
 */
func TestTwoAnswersAboutOneThingSettleOnTheBetterOne(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	world := New(Sources{
		Programs: []Program{{ID: "ollama", Title: "Ollama", Kind: Model, Names: []string{"ollama"}}},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				return []Thing{{ID: "ollama", Kind: Model, Title: "Ollama",
					State: Here, Version: "0.12.3", Local: true}}
			},
		},
	})

	all := world.All(context.Background())

	if len(all) != 1 {
		t.Fatalf("one thing became %d", len(all))
	}

	if all[0].State != Here || all[0].Version != "0.12.3" {
		t.Errorf("the worse answer won: %+v", all[0])
	}
}

// Somebody's settings can take something out, in one place rather than in
// every reader.
func TestSomethingTurnedOffIsNotInTheList(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	world := New(Sources{
		Programs: []Program{},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				return []Thing{
					{ID: "docker", Kind: Tool, Title: "Docker", State: Here},
					{ID: "git", Kind: Tool, Title: "Git", State: Here},
				}
			},
		},
		Hidden: func(id string) bool { return id == "docker" },
	})

	for _, thing := range world.All(context.Background()) {
		if thing.ID == "docker" {
			t.Error("something switched off was still offered")
		}
	}

	if _, found := world.Find(context.Background(), "git"); !found {
		t.Error("hiding one thing hid another")
	}
}

/*
 * Reading the machine is expensive, so it is kept — and asking again is
 * possible, because the moment after installing something is exactly when
 * the old answer is wrong.
 */
func TestTheReadingIsKeptAndCanBeTakenAgain(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var asked atomic.Int32

	world := New(Sources{
		Programs: []Program{},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				asked.Add(1)

				return []Thing{{ID: "git", Kind: Tool, Title: "Git", State: Here}}
			},
		},
	})

	ctx := context.Background()

	world.All(ctx)
	world.All(ctx)
	world.All(ctx)

	if got := asked.Load(); got != 1 {
		t.Errorf("it read the machine %d times for three questions", got)
	}

	world.Again(ctx)

	if got := asked.Load(); got != 2 {
		t.Errorf("asking again read it %d times in total", got)
	}
}

// An old reading is taken again by itself.
func TestAnOldReadingIsRefreshed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var asked atomic.Int32

	world := New(Sources{
		Programs: []Program{},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				asked.Add(1)

				return nil
			},
		},
	})

	world.Fresh = 10 * time.Millisecond

	world.All(context.Background())
	time.Sleep(25 * time.Millisecond)
	world.All(context.Background())

	if got := asked.Load(); got != 2 {
		t.Errorf("a reading older than its life was used anyway (%d readings)", got)
	}
}

// Usable is asked about things by name, and an unknown name is not usable —
// the safe reading and the honest one.
func TestUsableAnswersAboutOneThing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	world := New(Sources{
		Programs: []Program{},
		More: []func(context.Context) []Thing{
			func(context.Context) []Thing {
				return []Thing{
					{ID: "godot", Kind: Engine, Title: "Godot", State: Here},
					{ID: "unity", Kind: Engine, Title: "Unity", State: WantsLicence},
				}
			},
		},
	})

	ctx := context.Background()

	if !world.Usable(ctx, "godot") {
		t.Error("something available is not usable")
	}

	if world.Usable(ctx, "unity") {
		t.Error("something unlicensed is usable")
	}

	if world.Usable(ctx, "never-heard-of-it") {
		t.Error("something unknown is usable")
	}
}

/*
 * And a real reading of this machine, because a discovery that only ever
 * meets its own fixtures discovers fixtures.
 */
func TestItReadsThisMachine(t *testing.T) {
	things := New(Sources{}).All(context.Background())

	if len(things) < 20 {
		t.Fatalf("only %d things were looked for", len(things))
	}

	var here int

	for _, thing := range things {
		if thing.State == Here {
			here++

			if thing.Path == "" {
				t.Errorf("%s is here and has no path", thing.ID)
			}
		}
	}

	if here == 0 {
		t.Fatal("nothing at all was found on a machine that builds this program")
	}

	// Go built this test, so Go is here, with a version.
	found, known := New(Sources{}).Find(context.Background(), "go")

	if !known || found.State != Here {
		t.Fatalf("Go is %v on the machine compiling this: %+v", known, found)
	}

	if found.Version == "" {
		t.Error("Go did not say which version it is")
	}

	t.Logf("%d of %d found; Go %s at %s", here, len(things), found.Version, found.Path)
}

func write(t *testing.T, at, body string, mode os.FileMode) {
	t.Helper()

	if err := os.WriteFile(at, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

/*
 * A warning is not a version.
 *
 * Codex prints one before saying what it is, and taking the first line gave
 * "WARNING: proceeding, even though we coul" as a version — worse than saying
 * nothing, because it reads like one.
 */
func TestAWarningIsNotTakenForAVersion(t *testing.T) {
	bin := t.TempDir()

	write(t, filepath.Join(bin, "noisy"),
		"#!/bin/sh\necho 'WARNING: proceeding, even though we could not check'\necho 'codex-cli 0.155.0'\n", 0o755)
	write(t, filepath.Join(bin, "grumpy"),
		"#!/bin/sh\necho 'WARNING: this program will not say'\n", 0o755)

	t.Setenv("PATH", bin)

	things := Look(context.Background(), []Program{
		{ID: "noisy", Title: "Noisy", Kind: Tool, Names: []string{"noisy"}, Version: []string{"--version"}},
		{ID: "grumpy", Title: "Grumpy", Kind: Tool, Names: []string{"grumpy"}, Version: []string{"--version"}},
	})

	if things[0].Version != "0.155.0" {
		t.Errorf("the version behind the warning is %q", things[0].Version)
	}

	if things[1].Version != "" {
		t.Errorf("a program that only warned was given the version %q", things[1].Version)
	}

	// And it is still installed, which is the point: a program that will not
	// say its version is installed all the same.
	if things[1].State != Here {
		t.Errorf("a program that only warned is %q", things[1].State)
	}
}
