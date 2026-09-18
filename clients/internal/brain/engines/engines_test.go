package engines

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/stage"
)

// write lays scaffold files into a folder, as the workspace would.
func write(t *testing.T, dir string, files []File) {
	t.Helper()

	for _, f := range files {
		path := filepath.Join(dir, f.Path)

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeGodot is a program that answers like Godot 4.7.2 and prints what it
// is given as its complaints, so a run can be tested without the engine.
func fakeGodot(t *testing.T, says string, exit int) func() (godot.Engine, bool) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "godot4")
	script := "#!/bin/sh\ncat <<'EOF'\n" + says + "\nEOF\nexit " + string(rune('0'+exit)) + "\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return func() (godot.Engine, bool) {
		return godot.Engine{Path: path, Version: "4.7.2.stable.official.a1b2c3"}, true
	}
}

const godotSays = `Godot Engine v4.7.2.stable.official.a1b2c3 - https://godotengine.org
SCRIPT ERROR: Parse Error: Identifier "speeed" not declared in the current scope.
          at: GDScript::reload (res://main.gd:9)
ERROR: Failed to load script "res://main.gd" with error "Parse error".
   at: load (modules/gdscript/gdscript.cpp:2936)
WARNING: The variable "unused" is declared but never used.
     at: GDScript::reload (res://player.gd:3)`

func TestGodotComplaintsBecomeDiagnosticsWithAPlace(t *testing.T) {
	found := godotDiagnostics(godotSays)

	if len(found) != 3 {
		t.Fatalf("%d diagnostics: %+v", len(found), found)
	}

	if found[0].File != "main.gd" || found[0].Line != 9 || found[0].Severity != "error" {
		t.Errorf("the parse error lost its place: %+v", found[0])
	}

	// A line in the engine's own source is not a place in the project.
	if found[1].File != "" {
		t.Errorf("an engine source file was taken for a project file: %+v", found[1])
	}

	if found[2].Severity != "warning" || found[2].File != "player.gd" {
		t.Errorf("the warning: %+v", found[2])
	}
}

// A new Godot project is one the engine would open: its main scene and script
// are there and name each other, and it says which engine it is for.
func TestANewGodotProjectIsWellFormed(t *testing.T) {
	dir := t.TempDir()

	g := Godot{Find: fakeGodot(t, "", 0)}

	files, err := g.Scaffold(Scaffold{Name: "Falling Blocks", Engine: "4.7.2"})
	if err != nil {
		t.Fatal(err)
	}

	write(t, dir, files)

	p, ok := g.Detect(dir)
	if !ok || p.Name != "Falling Blocks" || p.Targets != "4.7" {
		t.Fatalf("detected as %+v", p)
	}

	if problems := g.Validate(p); len(problems) != 0 {
		t.Errorf("a new project has problems: %v", problems)
	}

	if !strings.Contains(read(filepath.Join(dir, "export_presets.cfg")), `platform="Linux"`) {
		t.Error("the export preset does not name Godot 4.7's Linux platform")
	}
}

// Godot exits zero with errors printed, so a run is judged on what it said.
func TestAGodotRunThatPrintedErrorsFailsWhateverItsExitStatus(t *testing.T) {
	dir := t.TempDir()

	g := Godot{Find: fakeGodot(t, godotSays, 0)}

	files, _ := g.Scaffold(Scaffold{Name: "x", Engine: "4.7"})
	write(t, dir, files)

	p, _ := g.Detect(dir)

	r := g.Check(context.Background(), p)

	if r.OK || r.Errors() == 0 || r.Exit != 0 {
		t.Fatalf("a run with a parse error passed: %+v", r)
	}

	if len(r.Commands) != 2 || r.Commands[1][len(r.Commands[1])-1] != "30" {
		t.Errorf("the commands were not recorded exactly: %v", r.Commands)
	}

	summary := r.Summary()

	for _, want := range []string{"failed", "main.gd:9", "exit status 0", "--headless"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary does not say %q:\n%s", want, summary)
		}
	}

	clean := Godot{Find: fakeGodot(t, "Godot Engine v4.7.2", 0)}

	if r := clean.Check(context.Background(), p); !r.OK {
		t.Errorf("a clean run failed: %s", r.Summary())
	}
}

func TestAMissingSceneIsFoundBeforeAnythingRuns(t *testing.T) {
	dir := t.TempDir()

	g := Godot{Find: fakeGodot(t, "", 0)}

	files, _ := g.Scaffold(Scaffold{Name: "x", Engine: "4.7"})
	write(t, dir, files)

	os.Remove(filepath.Join(dir, "main.gd"))

	p, _ := g.Detect(dir)

	r := g.Check(context.Background(), p)

	if r.OK || len(r.Commands) != 0 {
		t.Errorf("a project missing its script was run anyway: %+v", r)
	}
}

func TestExportingNeedsItsTemplatesAndSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()

	g := Godot{Find: fakeGodot(t, "", 0)}

	files, _ := g.Scaffold(Scaffold{Name: "x", Engine: "4.7"})
	write(t, dir, files)

	p, _ := g.Detect(dir)

	r := g.Build(context.Background(), p, filepath.Join(dir, "build"))

	if r.OK || !strings.Contains(r.Problem, "export templates") {
		t.Errorf("an export with no templates: %+v", r)
	}
}

func threeProject(t *testing.T) (Web, Project) {
	t.Helper()

	dir := t.TempDir()
	w := ThreeJS(stage.Three, stage.ThreeRevision, nil)

	files, err := w.Scaffold(Scaffold{Name: "Blocks"})
	if err != nil {
		t.Fatal(err)
	}

	write(t, dir, files)

	p, ok := w.Detect(dir)
	if !ok {
		t.Fatal("a new three.js project was not recognised")
	}

	return w, p
}

func TestANewThreeProjectCarriesItsOwnThree(t *testing.T) {
	w, p := threeProject(t)

	if p.Name != "Blocks" || p.Targets != stage.ThreeRevision {
		t.Errorf("detected as %+v", p)
	}

	if problems := w.Validate(p); len(problems) != 0 {
		t.Errorf("a new project has problems: %v", problems)
	}
}

func TestBrokenModulesAreFoundByNameAndLine(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	w, p := threeProject(t)

	if r := w.Check(context.Background(), p); !r.OK {
		t.Fatalf("a new project does not check: %s", r.Summary())
	}

	os.WriteFile(filepath.Join(p.Dir, "src", "game.js"), []byte("export class Game {\n  press(key) {\n    const = 1;\n  }\n}\n"), 0o644)
	os.WriteFile(filepath.Join(p.Dir, "src", "board.js"), []byte("import { Piece } from './piece.js';\n"), 0o644)

	r := w.Check(context.Background(), p)

	if r.OK {
		t.Fatal("broken modules passed")
	}

	var syntax, missing bool

	for _, d := range r.Diagnostics {
		if d.File == "src/game.js" && d.Line == 3 {
			syntax = true
		}

		if d.File == "src/board.js" && strings.Contains(d.Message, "./piece.js") {
			missing = true
		}
	}

	if !syntax || !missing {
		t.Errorf("the syntax error (%v) or the missing module (%v) was not placed: %+v", syntax, missing, r.Diagnostics)
	}
}

// Fetched from a CDN is a warning: it runs today, and not offline.
func TestALibraryFromTheInternetIsAWarning(t *testing.T) {
	w, p := threeProject(t)

	page := filepath.Join(p.Dir, "index.html")
	os.WriteFile(page, []byte(strings.Replace(read(page), "./vendor/three.module.js",
		"https://unpkg.com/three@0.169.0/build/three.module.js", 1)), 0o644)

	found := w.Validate(p)

	if len(found) != 1 || found[0].Severity != "warning" {
		t.Errorf("a CDN import: %+v", found)
	}
}

func TestARunPageThatThrowsFails(t *testing.T) {
	w, p := threeProject(t)

	w.Snap = func(_ context.Context, url string, _ int, png string) (PageShot, error) {
		if !strings.HasPrefix(url, "http://127.0.0.1:") {
			t.Errorf("the page was not served on the loopback address: %s", url)
		}

		os.WriteFile(png, []byte("png"), 0o644)

		return PageShot{Title: "Blocks", Errors: []string{"TypeError: x is undefined (main.js:12)"}, Canvas: 1, Picture: png}, nil
	}

	r := w.Smoke(context.Background(), p, t.TempDir())

	if r.OK || r.Screenshot == "" || r.Errors() != 1 {
		t.Errorf("a page that threw: %+v", r)
	}

	w.Snap = func(_ context.Context, _ string, _ int, png string) (PageShot, error) {
		return PageShot{Title: "Blocks", Canvas: 1, Picture: png}, nil
	}

	if r := w.Smoke(context.Background(), p, t.TempDir()); !r.OK {
		t.Errorf("a quiet page failed: %s", r.Summary())
	}
}

// Built without a build step: exactly the files the page loads.
func TestAWebGameBuildsIntoTheFolderGiven(t *testing.T) {
	w, p := threeProject(t)

	into := filepath.Join(p.Dir, "dist")

	r := w.Build(context.Background(), p, into)

	if !r.OK || !exists(filepath.Join(into, "index.html")) || !exists(filepath.Join(into, "vendor", "three.module.js")) {
		t.Fatalf("the build: %s", r.Summary())
	}

	if exists(filepath.Join(into, ".gitignore")) {
		t.Error("the build copied the project's own housekeeping")
	}
}

func TestProjectsOfEveryEngineAreFoundAndOthersAreNotEntered(t *testing.T) {
	root := t.TempDir()

	g := Godot{Find: fakeGodot(t, "", 0)}
	godotFiles, _ := g.Scaffold(Scaffold{Name: "One"})
	write(t, filepath.Join(root, "games", "one"), godotFiles)

	w := ThreeJS(stage.Three, stage.ThreeRevision, nil)
	webFiles, _ := w.Scaffold(Scaffold{Name: "Two"})
	write(t, filepath.Join(root, "web", "two"), webFiles)

	// A project inside a project's addons, and one inside node_modules, are
	// somebody else's.
	write(t, filepath.Join(root, "games", "one", "addons", "plugin"), godotFiles)
	write(t, filepath.Join(root, "node_modules", "three"), webFiles)

	os.MkdirAll(filepath.Join(root, "unity", "Three", "Assets"), 0o755)
	write(t, filepath.Join(root, "unity", "Three"), []File{{Path: "ProjectSettings/ProjectVersion.txt",
		Content: []byte("m_EditorVersion: 2022.3.10f1\n")}})

	found := Standard(nil).Find(root, 50)

	engines := map[string]string{}

	for _, p := range found {
		engines[p.Engine] = p.Targets
	}

	if len(found) != 3 || engines["unity"] != "2022.3.10f1" || engines["threejs"] != stage.ThreeRevision {
		t.Errorf("found %+v", found)
	}
}

func TestCompilerLinesBecomeDiagnostics(t *testing.T) {
	unity := csharpDiagnostics(`Assets/Scripts/Board.cs(14,9): error CS0103: The name 'pice' does not exist in the current context
Assets/Scripts/Board.cs(14,9): error CS0103: The name 'pice' does not exist in the current context`, "/p")

	if len(unity) != 1 || unity[0].Line != 14 || !strings.Contains(unity[0].Message, "CS0103") {
		t.Errorf("unity: %+v", unity)
	}

	rust := rustDiagnostics("error[E0425]: cannot find value `x` in this scope\n --> src/main.rs:3:5\n", "/p")

	if len(rust) != 1 || rust[0].File != "src/main.rs" || rust[0].Line != 3 {
		t.Errorf("rust: %+v", rust)
	}
}

func TestEveryEngineAnswersEveryQuestion(t *testing.T) {
	for _, a := range Standard(nil).All() {
		if a.ID() == "" || a.Title() == "" || len(a.Docs("", "")) == 0 {
			t.Errorf("%T says too little about itself", a)
		}
	}

	for _, id := range []string{"godot", "threejs", "unity", "unreal", "phaser", "babylonjs", "playcanvas",
		"bevy", "defold", "gamemaker"} {
		if !Standard(nil).Known(id) {
			t.Errorf("%s is not an engine", id)
		}
	}
}

// The import a model got wrong twice on this machine — three.js by a path
// from src/ — is refused with what to write instead.
func TestAPathToThreeIsAnsweredWithItsName(t *testing.T) {
	w, p := threeProject(t)

	os.WriteFile(filepath.Join(p.Dir, "src", "main.js"),
		[]byte("import * as THREE from './vendor/three.module.js';\nnew THREE.Scene();\n"), 0o644)

	r := w.Check(context.Background(), p)

	for _, d := range r.Diagnostics {
		if d.File == "src/main.js" && strings.Contains(d.Message, "import it as 'three'") {
			return
		}
	}

	t.Errorf("the wrong path to three.js was not answered with its name: %+v", r.Diagnostics)
}
