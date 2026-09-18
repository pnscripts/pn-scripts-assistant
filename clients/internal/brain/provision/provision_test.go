package provision

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionRules(t *testing.T) {
	for _, c := range []struct {
		rule, version string
		ok            bool
	}{
		{">=4.2 <5", "4.7.2", true},
		{">=4.2 <5", "4.2", true},
		{">=4.2 <5", "4.1.9", false},
		{">=4.2 <5", "5.0", false},
		{">=4.2, <5", "4.10", true}, // numbers, not text
		{"4.7", "4.7.2.stable.official", true},
		{"4.7", "4.70", false},
		{"=6000.0", "6000.0.23f1", true},
		{">=18", "v24.21.0", true},
		{">=18", "v16.0.0", false},
		{"", "anything", true},
	} {
		rule, err := ParseRule(c.rule)
		if err != nil {
			t.Fatalf("%q: %v", c.rule, err)
		}

		if got := rule.Allows(c.version); got != c.ok {
			t.Errorf("%q against %q: %v, want %v", c.version, c.rule, got, c.ok)
		}
	}

	if _, err := ParseRule("around four"); err == nil {
		t.Error("a rule in words was read as a rule")
	}
}

func TestNewerIsNumeric(t *testing.T) {
	if !Newer("4.9", "4.10") || Newer("4.10", "4.9") || Newer("x", "4.1") {
		t.Error("versions compared as text, or an unreadable one called newer")
	}
}

// A fake program that answers --version, somewhere only this test knows.
func fakeProgram(t *testing.T, name, says string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)

	script := "#!/bin/sh\necho '" + says + "'\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestFoundAndWeighedAgainstTheRule(t *testing.T) {
	dir := fakeProgram(t, "enginey", "Enginey 4.3.1 (stable)")

	r := Recipe{
		ID: "enginey", Title: "Enginey", Commands: []string{"enginey"}, Places: []string{dir},
		VersionArgs: []string{"--version"},
	}

	s := r.Check(">=4.2 <5")

	if !s.Present || s.Version != "4.3.1" || !s.Compatible || !s.Ready() {
		t.Fatalf("read as %+v", s)
	}

	s = r.Check(">=4.5")

	if !s.Present || s.Compatible || !strings.Contains(s.Problem, "4.3.1") {
		t.Errorf("an old version passed, or the problem did not name it: %+v", s)
	}
}

// Present and unable to say which version is not a pass: an unchecked rule is
// not a rule.
func TestAVersionThatCannotBeReadDoesNotPass(t *testing.T) {
	dir := fakeProgram(t, "quiet", "no numbers here")

	r := Recipe{ID: "quiet", Commands: []string{"quiet"}, Places: []string{dir}, VersionArgs: []string{"--version"}}

	if s := r.Check(">=1"); s.Compatible {
		t.Errorf("passed with no version: %+v", s)
	}

	if s := r.Check(""); !s.Compatible {
		t.Errorf("no rule, and still refused: %+v", s)
	}
}

func TestNotHereSaysWhatItWouldTake(t *testing.T) {
	r := unityRecipe()
	r.Find = func() (string, string, bool) { return "", "", false }

	s := r.Check(">=2022.3")

	if s.Present || s.Installable || s.Manual == "" || s.Cost == "" || s.Licence == "" {
		t.Errorf("a licensed engine that is missing said too little, or offered to install: %+v", s)
	}
}

/*
 * An install is proved, not trusted: found afterwards, in a version the rule
 * allows, and its smoke run passes.
 */
func TestCarryProvesTheInstall(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(dir, "thing")

	r := Recipe{
		ID: "thing", Title: "Thing", Commands: []string{"thing"}, Places: []string{dir},
		VersionArgs: []string{"--version"}, Smoke: []string{"--version"},
		Install: func(context.Context, io.Writer) error {
			return os.WriteFile(program, []byte("#!/bin/sh\necho 'thing 2.0'\n"), 0o755)
		},
	}

	s, err := Carry(context.Background(), r, ">=2", io.Discard)
	if err != nil || !s.Ready() {
		t.Fatalf("a good install was not proved: %+v %v", s, err)
	}

	// An installer that says it worked and left the wrong version.
	os.Remove(program)

	r.Install = func(context.Context, io.Writer) error {
		return os.WriteFile(program, []byte("#!/bin/sh\necho 'thing 1.0'\n"), 0o755)
	}

	if _, err := Carry(context.Background(), r, ">=2", io.Discard); err == nil {
		t.Error("the wrong version was reported installed")
	}

	// And one whose smoke run fails.
	r.Install = func(context.Context, io.Writer) error {
		return os.WriteFile(program, []byte("#!/bin/sh\necho 'thing 3.0'\nexit 3\n"), 0o755)
	}

	if _, err := Carry(context.Background(), r, "", io.Discard); err == nil {
		t.Error("a program that does not run was reported installed")
	}
}

func TestNothingIsInstalledWithoutARecipe(t *testing.T) {
	r := Recipe{ID: "x", Title: "X", Manual: "ask somebody"}

	_, err := Carry(context.Background(), r, "", io.Discard)

	if err == nil || !strings.Contains(err.Error(), "ask somebody") {
		t.Errorf("got %v", err)
	}

	if err := Take(r, io.Discard); err == nil {
		t.Error("removed something this program never installed")
	}

	failing := Recipe{ID: "y", Title: "Y", Remove: func(io.Writer) error { return errors.New("stuck") }}

	if err := Take(failing, io.Discard); err == nil {
		t.Error("a failed removal was reported done")
	}
}

/*
 * Every shipped recipe says where it comes from, under what terms and at what
 * cost — those are what somebody agrees to — and only the pinned or system
 * ones can install at all.
 */
func TestEveryShippedRecipeSaysWhatItWouldCost(t *testing.T) {
	for _, r := range Standard().All() {
		if r.ID == "" || r.Title == "" || r.Kind == "" {
			t.Errorf("%+v is missing its name", r.ID)
		}

		if r.Kind == Part {
			continue
		}

		if r.Source == "" || r.Licence == "" || r.Cost == "" {
			t.Errorf("%s does not say where it comes from, its licence or its cost", r.ID)
		}

		if !r.Installable() && r.Manual == "" {
			t.Errorf("%s cannot be installed and does not say what a person does instead", r.ID)
		}
	}

	for _, licensed := range []string{"unity", "unreal", "cargo", "chrome"} {
		if r, ok := Standard().Get(licensed); !ok || r.Installable() {
			t.Errorf("%s is installable from here, and needs an account, a licence or a script", licensed)
		}
	}
}

func TestPartsAreRecipesToo(t *testing.T) {
	if !Standard().Known(PartID("Godot (making games)")) {
		t.Error("the Godot machine part is not a recipe")
	}

	if got := PartID("Voice (speaking)"); got != "part.voice-speaking" {
		t.Errorf("part id %q", got)
	}
}

// The installed Unity, asked for real when the test is asked for: whatever
// it says, it says one of three things and why.
func TestTheInstalledUnityLicenceIsToldHonestly(t *testing.T) {
	if os.Getenv("PN_TEST_UNITY_LICENCE") == "" {
		t.Skip("set PN_TEST_UNITY_LICENCE=1 to start the installed Unity editor")
	}

	path, version, ok := FindUnity()
	if !ok {
		t.Skip("no Unity here")
	}

	l := UnityLicence(path)

	t.Logf("Unity %s at %s: %s — %s", version, path, l.State, l.Why)

	if l.State != LicenceActive && l.State != LicenceInactive && l.State != LicenceUnknown || l.Why == "" {
		t.Errorf("an answer with no reason: %+v", l)
	}
}

func TestUnityFoldersAreReadForTheirVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, folder := range []string{"6000.5.4f1", "6000.5.4f1-x86_64", "2022.3.10f1", "notes"} {
		dir := filepath.Join(home, "Unity", "Hub", "Editor", folder, "Editor")
		os.MkdirAll(filepath.Join(dir, "Data", "PlaybackEngines", "LinuxStandaloneSupport"), 0o755)
		os.WriteFile(filepath.Join(dir, "Unity"), []byte("#!/bin/sh\n"), 0o755)
	}

	os.MkdirAll(filepath.Join(home, "Unity", "Hub", "Editor", "6000.5.4f1", "Editor", "Data", "PlaybackEngines", "WebGLSupport"), 0o755)

	all := UnityInstalls()

	if len(all) != 3 {
		t.Fatalf("found %d editors: %+v", len(all), all)
	}

	if all[0].Folder != "6000.5.4f1" || all[0].Version != "6000.5.4f1" {
		t.Errorf("chose %s (%s), not the newest with the most modules", all[0].Folder, all[0].Version)
	}

	for _, in := range all {
		if strings.Contains(in.Version, "x86") {
			t.Errorf("the architecture became part of the version: %s", in.Version)
		}
	}
}
