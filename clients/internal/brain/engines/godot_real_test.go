package engines

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/display"
	"pn-scripts-assistant/internal/brain/godot"
)

/*
 * The real engine, when there is one.
 *
 * Every other Godot test here talks to a shell script that prints what Godot
 * prints, and one of them passed for months while real Godot refused the very
 * project it approved: the preset was missing two keys the script did not know
 * about. So when a Godot is installed, a new project is checked by it — and
 * its settings folders are this test's own, so nobody's editor is touched.
 */
func realGodot(t *testing.T) Godot {
	t.Helper()

	engine, ok := godot.Find()
	if at := os.Getenv("PN_TEST_GODOT"); at != "" {
		engine, ok = godot.Engine{Path: at, Version: "4"}, true
	}

	if !ok {
		t.Skip("no Godot on this machine")
	}

	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, t.TempDir())
	}

	return Godot{Find: func() (godot.Engine, bool) { return engine, true }}
}

func scaffoldInto(t *testing.T, a Adapter, name string) Project {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "game")

	files, err := a.Scaffold(Scaffold{Name: name})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		path := filepath.Join(dir, f.Path)
		os.MkdirAll(filepath.Dir(path), 0o755)

		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p, ok := a.Detect(dir)
	if !ok {
		t.Fatalf("a new %s project is not recognised as one", a.Title())
	}

	return p
}

func TestARealGodotAcceptsANewProject(t *testing.T) {
	g := realGodot(t)
	p := scaffoldInto(t, g, "Tetris")

	r := g.Check(context.Background(), p)
	if !r.OK {
		t.Fatalf("real Godot refused a new project:\n%s", r.Summary())
	}
}

func TestARealGodotNamesTheLineThatBroke(t *testing.T) {
	g := realGodot(t)
	p := scaffoldInto(t, g, "Tetris")

	script := filepath.Join(p.Dir, "main.gd")
	os.WriteFile(script, []byte("extends Node2D\n\nfunc _process(_delta: float) -> void:\n\tvar n = null\n\tn.rotate(1.0)\n"), 0o644)

	r := g.Check(context.Background(), p)
	if r.OK {
		t.Fatal("a runtime error passed the check")
	}

	found := false

	for _, d := range r.Diagnostics {
		if d.File == "main.gd" && d.Line == 5 {
			found = true
		}
	}

	if !found {
		t.Fatalf("the error was not placed at main.gd:5:\n%s", r.Summary())
	}

	os.WriteFile(script, []byte("extends Node2D\n\nfunc _ready() -> void:\n\tprint(\"x\"\n"), 0o644)

	r = g.Check(context.Background(), p)

	seen := map[string]int{}

	for _, d := range r.Diagnostics {
		seen[d.String()]++
	}

	for d, n := range seen {
		if n > 1 {
			t.Errorf("reported %d times: %s", n, d)
		}
	}

	if r.OK || !strings.Contains(r.Summary(), "main.gd:4") {
		t.Fatalf("a parse error was not placed at main.gd:4:\n%s", r.Summary())
	}
}

/*
 * A picture on a display nobody sees — and a picture that has to show
 * something. The template draws nothing, and a run of it is not evidence of a
 * game; the same project drawing a falling block is.
 */
func TestARealGodotIsPhotographedAndMustDrawSomething(t *testing.T) {
	g := realGodot(t)

	if _, err := display.Available(); err != nil {
		t.Skip(err)
	}

	p := scaffoldInto(t, g, "Tetris")

	empty := g.Smoke(context.Background(), p, filepath.Join(p.Dir, "shots-empty"))
	if empty.OK || empty.Screenshot == "" {
		t.Fatalf("a game that draws nothing passed, or was not photographed:\n%s", empty.Summary())
	}

	os.WriteFile(filepath.Join(p.Dir, "main.gd"), []byte(`extends Node2D

var y := 0.0

func _process(delta: float) -> void:
	y += 200.0 * delta
	queue_redraw()

func _draw() -> void:
	draw_rect(Rect2(0, 0, 640, 960), Color(0.05, 0.05, 0.1))
	draw_rect(Rect2(260, fmod(y, 900.0), 120, 60), Color(0.9, 0.2, 0.3))
`), 0o644)

	drawn := g.Smoke(context.Background(), p, filepath.Join(p.Dir, "shots"))
	if !drawn.OK || drawn.Screenshot == "" {
		t.Fatalf("a game that draws was not passed with its picture:\n%s", drawn.Summary())
	}

	if flat, err := Flat(drawn.Screenshot); err != nil || flat {
		t.Errorf("the picture of a drawing game is flat (%v)", err)
	}
}
