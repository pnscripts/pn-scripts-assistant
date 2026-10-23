package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// placeProgram pretends the running program is dir/pn-scripts-assistant and
// gives the test a home and a config folder of its own.
func placeProgram(t *testing.T, dir string) {
	t.Helper()

	real := executable
	executable = func() (string, error) { return filepath.Join(dir, Name), nil }
	t.Cleanup(func() { executable = real })

	realMounts := mounts
	mounts = func() []string { return nil }
	t.Cleanup(func() { mounts = realMounts })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PN_SCRIPTS_ASSISTANT_DATA_ROOT", "")
	t.Setenv("PN_BRAIN_DATA_ROOT", "")
}

func TestAnInstalledProgramIsNotPortable(t *testing.T) {
	placeProgram(t, t.TempDir())

	if dir, ok := PortableRoot(); ok {
		t.Fatalf("a program with no PORTABLE file and no data beside it is portable, at %s", dir)
	}
}

func TestAPortableCopyKeepsItsBrainBesideItAndLeavesNothingBehind(t *testing.T) {
	stick := t.TempDir()
	placeProgram(t, stick)

	// The machine already has a brain of its own, which must not be opened.
	own, err := Choose(HomeRoot())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(stick, PortableFlag), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(stick, DataFolder)
	if r.Path != want {
		t.Fatalf("portable brain at %s, want %s beside the program", r.Path, want)
	}

	if !r.Borrowed {
		t.Fatal("a portable brain is not borrowed, so the machine would remember the stick")
	}

	if was, _ := LastKnown(); was != own.Path {
		t.Fatalf("the machine now remembers %s instead of its own %s", was, own.Path)
	}

	// The second start finds the same brain, even with the flag gone: the
	// data folder beside the program is enough.
	if err := os.Remove(filepath.Join(stick, PortableFlag)); err != nil {
		t.Fatal(err)
	}

	again, err := FindOrCreate()
	if err != nil {
		t.Fatal(err)
	}

	if again.Path != want || again.ID != r.ID {
		t.Fatalf("second start opened %s (%s), want the same portable brain %s (%s)", again.Path, again.ID, want, r.ID)
	}
}

func TestSetupChoosingThePortableFolderIsNotRemembered(t *testing.T) {
	stick := t.TempDir()
	placeProgram(t, stick)

	if err := os.WriteFile(filepath.Join(stick, PortableFlag), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Choose(filepath.Join(stick, DataFolder))
	if err != nil {
		t.Fatal(err)
	}

	if !r.Borrowed {
		t.Fatal("setup choosing the portable folder made it this machine's brain")
	}

	if was, ok := LastKnown(); ok {
		t.Fatalf("the machine remembers %s after a portable setup", was)
	}
}
