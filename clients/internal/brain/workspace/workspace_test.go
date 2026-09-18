package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/undo"
)

func TestNoProjectIsMadeWhereNoProjectBelongs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brain := filepath.Join(home, "brain")
	os.MkdirAll(brain, 0o700)

	for dir, why := range map[string]string{
		"/":                                  "system",
		"/etc/game":                          "system",
		"/usr/local/game":                    "system",
		home:                                 "whole home folder",
		filepath.Join(home, ".config", "x"):  "hidden folder",
		filepath.Join(brain, "projects"):     "own memory",
		filepath.Join(home, "Projects", "x"): "",
	} {
		got := Inspect(dir, engines.NewRegistry(), brain).Refused

		if (why == "") != (got == "") || !strings.Contains(got, why) {
			t.Errorf("%s: refused %q, want something about %q", dir, got, why)
		}
	}

	if r := Inspect("games/tetris", engines.NewRegistry()); r.Refused == "" {
		t.Error("a relative path was accepted as a place")
	}
}

func TestAFolderIsDescribedBeforeAnythingIsWritten(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"build":"vite build","test":"vitest"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644)

	for _, argv := range [][]string{{"init", "-q"}, {"add", "README.md"}} {
		exec.Command("git", append([]string{"-C", dir}, argv...)...).Run()
	}

	r := Inspect(dir, engines.NewRegistry())

	if !r.Exists || r.Empty || !r.Git || r.Dirty == 0 {
		t.Fatalf("read as %+v", r)
	}

	if strings.Join(r.Commands["build"], " ") != "npm run build" || strings.Join(r.Commands["test"], " ") != "npm test" {
		t.Errorf("its own commands were not found: %v", r.Commands)
	}

	text := r.Text()

	for _, want := range []string{"node", "uncommitted", "README.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("the report does not say %q:\n%s", want, text)
		}
	}

	// Looking wrote nothing.
	if _, err := os.Stat(filepath.Join(dir, Folder)); err == nil {
		t.Error("inspecting wrote the settings folder")
	}
}

// All or nothing: one file already there, and nothing is written at all.
func TestLayingAProjectNeverOverwritesAnything(t *testing.T) {
	dir := t.TempDir()
	root := t.TempDir()

	os.WriteFile(filepath.Join(dir, "main.gd"), []byte("mine"), 0o644)

	files := []engines.File{
		{Path: "project.godot", Content: []byte("x")},
		{Path: "main.gd", Content: []byte("theirs")},
	}

	if _, err := Lay(dir, files, Config{Name: "x", Kind: "game"}, root); err == nil {
		t.Fatal("a file that was there was overwritten, or the clash was not said")
	}

	if got, _ := os.ReadFile(filepath.Join(dir, "main.gd")); string(got) != "mine" {
		t.Errorf("the existing file became %q", got)
	}

	if _, err := os.Stat(filepath.Join(dir, "project.godot")); err == nil {
		t.Error("half of the project was written before the clash was found")
	}

	if _, err := Lay(dir, []engines.File{{Path: "../escape.txt"}}, Config{}, root); err == nil {
		t.Error("a file outside the project was written")
	}
}

func TestALaidProjectCanBeTakenBackFileByFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tetris")
	root := t.TempDir()

	files, _ := Template("book", "A Book")

	written, err := Lay(dir, files, Config{Name: "A Book", Kind: "book", Outputs: []string{"export"}}, root)
	if err != nil {
		t.Fatal(err)
	}

	if len(written) != len(files)+1 {
		t.Errorf("wrote %d files", len(written))
	}

	c, err := Load(dir)
	if err != nil || c.Kind != "book" || c.Roots[0] != "." {
		t.Fatalf("the settings read back as %+v %v", c, err)
	}

	kept := undo.List(root)

	if len(kept) != len(written) {
		t.Fatalf("%d of %d files can be put back", len(kept), len(written))
	}

	for _, change := range kept {
		if !change.New {
			t.Errorf("%s was recorded as overwritten", change.Path)
		}
	}
}

/*
 * Work on a project stays in it, however the path is spelled: "..", a
 * relative path and a link that points out of the project are all where they
 * really go.
 */
func TestTheScopeIsWhereAPathReallyIs(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.Symlink(outside, filepath.Join(dir, "assets"))

	s := ScopeOf(dir, Config{Roots: []string{"."}, Outputs: []string{"build"}})

	for path, want := range map[string]bool{
		filepath.Join(dir, "src", "main.gd"):         true,
		filepath.Join(dir, "new", "folder", "x.txt"): true,
		filepath.Join(dir, "src", "..", "..", "x"):   false,
		filepath.Join(dir, "assets", "secret.txt"):   false,
		outside:                            false,
		filepath.Join(dir+"-sibling", "x"): false,
	} {
		if got := s.Allows(path); got != want {
			t.Errorf("%s: allowed %v, want %v", path, got, want)
		}
	}

	if !s.Output(filepath.Join(dir, "build", "game.x86_64")) || s.Output(filepath.Join(dir, "src", "x")) {
		t.Error("the outputs are not where builds may go")
	}

	if !s.Output(filepath.Join(dir, Evidence, "shot.png")) {
		t.Error("pictures of checks have nowhere to go")
	}
}

func TestSettingsThatWidenTheScopeAreRefused(t *testing.T) {
	for _, c := range []Config{
		{Roots: []string{"/home"}},
		{Roots: []string{"../other"}},
		{Outputs: []string{"../../"}},
		{Commands: map[string][]string{"build": {}}},
	} {
		if err := c.Check(); err == nil {
			t.Errorf("%+v was accepted", c)
		}
	}

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, Folder), 0o755)
	os.WriteFile(Path(dir), []byte(`{"version":1,"roots":["..", "."]}`), 0o644)

	if _, err := Load(dir); err == nil {
		t.Error("a settings file reaching out of its project was loaded")
	}
}

func TestProjectMemoryIsThisProjects(t *testing.T) {
	dir := t.TempDir()

	for _, line := range []string{"Input goes through actions", "Export needs templates 4.7.2"} {
		if err := Remember(dir, line); err != nil {
			t.Fatal(err)
		}
	}

	got := Memory(dir, 1)

	if len(got) != 1 || !strings.Contains(got[0], "templates") {
		t.Errorf("memory %v", got)
	}
}

func TestAProjectsOwnSkillsAreRead(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, Folder, "skills"), 0o755)
	os.WriteFile(filepath.Join(dir, Folder, "skills", "levels.md"), []byte("Levels are generated: edit gen.py, not levels/*.json"), 0o644)

	got := Skills(dir, 3)

	if len(got) != 1 || !strings.HasPrefix(got[0], "levels: Levels are generated") {
		t.Errorf("skills %v", got)
	}
}
