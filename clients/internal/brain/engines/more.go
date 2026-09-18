package engines

import (
	"pn-scripts-assistant/internal/brain/provision"

	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"pn-scripts-assistant/internal/stage"
)

/*
 * The rest: engines this program recognises and checks as far as their own
 * tools allow, through the same contract, so adding one properly later is
 * filling in methods rather than teaching the proposal, the planner and the
 * report about a new thing.
 */

// Bevy is a Rust game engine; cargo is its build tool.
type Bevy struct{}

func (Bevy) ID() string     { return "bevy" }
func (Bevy) Title() string  { return "Bevy" }
func (Bevy) Recipe() string { return "cargo" }
func (Bevy) Licensed() bool { return false }

func (Bevy) Engine() (Installed, bool) {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		return Installed{}, false
	}

	return Installed{Path: cargo, Where: "Bevy comes from each project's Cargo.toml"}, true
}

var bevyDependency = regexp.MustCompile(`(?m)^bevy\s*=\s*(?:"([^"]+)"|\{[^}]*version\s*=\s*"([^"]+)")`)

func (Bevy) Detect(dir string) (Project, bool) {
	manifest := read(filepath.Join(dir, "Cargo.toml"))

	m := bevyDependency.FindStringSubmatch(manifest)
	if m == nil {
		return Project{}, false
	}

	version := m[1]
	if version == "" {
		version = m[2]
	}

	return Project{Engine: "bevy", Dir: dir, Name: filepath.Base(dir), Targets: version}, true
}

func (Bevy) Docs(version, topic string) []Doc {
	if version == "" {
		version = "latest"
	}

	return []Doc{{Title: "Bevy " + version + " API", URL: "https://docs.rs/bevy/" + version + "/bevy/"}}
}

func (Bevy) Scaffold(Scaffold) ([]File, error) {
	return nil, fmt.Errorf("a new Bevy project is made with cargo new; there is no template here")
}

func (Bevy) Validate(p Project) []Diagnostic {
	if !exists(filepath.Join(p.Dir, "src", "main.rs")) && !exists(filepath.Join(p.Dir, "src", "lib.rs")) {
		return []Diagnostic{{Severity: "error", File: "src", Message: "there is no src/main.rs or src/lib.rs"}}
	}

	return nil
}

func (b Bevy) Check(ctx context.Context, p Project) Run {
	engine, ok := b.Engine()
	if !ok {
		return notInstalled(b, "check")
	}

	r := Run{What: "check", Engine: "bevy", Version: p.Targets}
	r.Diagnostics = b.Validate(p)

	out, exited := r.step(ctx, p.Dir, 30*time.Minute, engine.Path, "check")
	r.Diagnostics = append(r.Diagnostics, rustDiagnostics(out, p.Dir)...)
	r.withErrors(exited)

	return r
}

func (b Bevy) Build(ctx context.Context, p Project, into string) Run {
	engine, ok := b.Engine()
	if !ok {
		return notInstalled(b, "build")
	}

	r := Run{What: "build", Engine: "bevy", Version: p.Targets}

	out, exited := r.step(ctx, p.Dir, 60*time.Minute, engine.Path, "build", "--release",
		"--target-dir", into)
	r.Diagnostics = rustDiagnostics(out, p.Dir)

	if exists(filepath.Join(into, "release")) {
		r.Artifacts = append(r.Artifacts, filepath.Join(into, "release"))
	}

	r.withErrors(exited)

	return r
}

func (Bevy) Smoke(context.Context, Project, string) Run {
	return Run{What: "smoke", Engine: "bevy",
		Problem: "a Bevy game opens a window to run, and there is no headless way to try it here"}
}

// detectOnly is an engine recognised and pointed at, and nothing more: its
// tools are its own editor, which this program does not drive.
type detectOnly struct {
	id, title, marker, docs string
	licensed                bool
}

func (d detectOnly) ID() string     { return d.id }
func (d detectOnly) Title() string  { return d.title }
func (d detectOnly) Recipe() string { return "" }
func (d detectOnly) Licensed() bool { return d.licensed }

func (d detectOnly) Engine() (Installed, bool) {
	return Installed{Where: "its own editor, which this program does not drive"}, false
}

func (d detectOnly) Detect(dir string) (Project, bool) {
	matches, _ := filepath.Glob(filepath.Join(dir, d.marker))
	if len(matches) == 0 {
		return Project{}, false
	}

	return Project{Engine: d.id, Dir: dir, Name: filepath.Base(dir)}, true
}

func (d detectOnly) Docs(string, string) []Doc {
	return []Doc{{Title: d.title + " documentation", URL: d.docs}}
}

func (d detectOnly) Scaffold(Scaffold) ([]File, error) {
	return nil, fmt.Errorf("a new %s project is made in %s's own editor", d.title, d.title)
}

func (d detectOnly) Validate(Project) []Diagnostic { return nil }

func (d detectOnly) unsupported(what string) Run {
	return Run{What: what, Engine: d.id,
		Problem: d.title + " projects are built in " + d.title + "'s own editor, which this program does not drive"}
}

func (d detectOnly) Check(context.Context, Project) Run         { return d.unsupported("check") }
func (d detectOnly) Build(context.Context, Project, string) Run { return d.unsupported("build") }
func (d detectOnly) Smoke(context.Context, Project, string) Run { return d.unsupported("smoke") }

// Defold and GameMaker are recognised, and documented, and left to their
// editors.
func Defold() Adapter {
	return detectOnly{id: "defold", title: "Defold", marker: "game.project", docs: "https://defold.com/ref/stable/"}
}

func GameMaker() Adapter {
	return detectOnly{id: "gamemaker", title: "GameMaker", marker: "*.yyp",
		docs: "https://manual.gamemaker.io/", licensed: true}
}

// Standard is every engine, most specific first: a three.js page is also a
// web page, and it is the three.js adapter that should say so.
func Standard(snap Snapper) *Registry {
	return NewRegistry(
		Godot{},
		ThreeJS(stage.Three, stage.ThreeRevision, snap),
		Unity{Licence: func(path string) (string, string) {
			l := provision.UnityLicence(path)

			return l.State, l.Why
		}},
		Unreal{},
		Phaser(snap),
		Babylon(snap),
		PlayCanvas(snap),
		Bevy{},
		Defold(),
		GameMaker(),
	)
}
