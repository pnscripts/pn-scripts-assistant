package engines

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/display"
	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/provision"
)

/*
 * Godot, the first engine and the one the contract was drawn from.
 *
 * Everything the three Godot tools did is here — finding the engine, finding
 * projects, the class reference for the installed version, a headless run, an
 * export — with the parts they did not: a new project that opens, a project
 * checked before it is run, a run that reports the script and the line rather
 * than a paragraph, and a picture when there is a display to take one on.
 */
type Godot struct {
	// Find is how the engine is found; godot.Find unless a test says.
	Find func() (godot.Engine, bool)

	// Limit bounds anything that runs the engine.
	Limit time.Duration

	// Preset and File are an export's preset and exact output file, when a
	// caller names them; otherwise the project's first preset, named for the
	// project, in the folder given.
	Preset string
	File   string
}

// HowLongGodotMayTake bounds one run of the engine — an export of a large
// project is minutes, a game left running is forever.
const HowLongGodotMayTake = 20 * time.Minute

func (Godot) ID() string     { return "godot" }
func (Godot) Title() string  { return "Godot" }
func (Godot) Recipe() string { return "godot" }
func (Godot) Licensed() bool { return false }
func (g Godot) limit() time.Duration {
	if g.Limit > 0 {
		return g.Limit
	}

	return HowLongGodotMayTake
}

var godotVersion = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)

func (g Godot) Engine() (Installed, bool) {
	find := g.Find
	if find == nil {
		find = godot.Find
	}

	e, ok := find()
	if !ok {
		return Installed{}, false
	}

	version := e.Version

	if m := godotVersion.FindStringSubmatch(version); m != nil {
		version = m[1]
	}

	return Installed{Path: e.Path, Version: version}, true
}

var godotFeatures = regexp.MustCompile(`(?m)^config/features=PackedStringArray\(([^)]*)\)`)

func (Godot) Detect(dir string) (Project, bool) {
	config := read(filepath.Join(dir, "project.godot"))
	if config == "" && !exists(filepath.Join(dir, "project.godot")) {
		return Project{}, false
	}

	p := Project{Engine: "godot", Dir: dir, Name: godotName(dir, config), Details: map[string]string{}}

	if m := godotFeatures.FindStringSubmatch(config); m != nil {
		for _, part := range strings.Split(m[1], ",") {
			part = strings.Trim(strings.TrimSpace(part), `"`)

			if regexp.MustCompile(`^\d+\.\d+$`).MatchString(part) {
				p.Targets = part
			}
		}
	}

	if m := regexp.MustCompile(`(?m)^run/main_scene="([^"]*)"`).FindStringSubmatch(config); m != nil {
		p.Details["main_scene"] = m[1]
	}

	return p, true
}

func godotName(dir, config string) string {
	if m := regexp.MustCompile(`(?m)^config/name="([^"]*)"`).FindStringSubmatch(config); m != nil && m[1] != "" {
		return m[1]
	}

	return filepath.Base(dir)
}

func (Godot) Docs(version, topic string) []Doc {
	branch := majorMinor(version)
	if branch == "" {
		branch = "stable"
	}

	base := "https://docs.godotengine.org/en/" + branch

	var out []Doc

	if topic = strings.TrimSpace(topic); topic != "" {
		out = append(out, Doc{Title: topic + " in the Godot " + branch + " class reference",
			URL: base + "/classes/class_" + strings.ToLower(topic) + ".html"})
	}

	return append(out,
		Doc{Title: "Godot " + branch + " class reference", URL: base + "/classes/index.html"},
		Doc{Title: "GDScript reference", URL: base + "/tutorials/scripting/gdscript/gdscript_basics.html"})
}

/*
 * Scaffold is a project that opens, runs and exports.
 *
 * Small on purpose: a main scene, its script, and an export preset for this
 * machine. What the game is belongs to whoever writes it; what makes it a
 * Godot project that runs is this.
 */
func (Godot) Scaffold(s Scaffold) ([]File, error) {
	version := majorMinor(s.Engine)
	if version == "" {
		version = "4.7"
	}

	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = "Game"
	}

	// The preset's platform was renamed in 4.3.
	platform := "Linux"

	if provision.Newer(version, "4.3") {
		platform = "Linux/X11"
	}

	project := fmt.Sprintf(`; Engine configuration file.
; Made by PN Scripts Assistant for Godot %[1]s.

config_version=5

[application]

config/name=%[2]q
run/main_scene="res://main.tscn"
config/features=PackedStringArray("%[1]s", "GL Compatibility")

[display]

window/size/viewport_width=640
window/size/viewport_height=960

[rendering]

renderer/rendering_method="gl_compatibility"
renderer/rendering_method.mobile="gl_compatibility"
`, version, name)

	scene := `[gd_scene load_steps=2 format=3]

[ext_resource type="Script" path="res://main.gd" id="1"]

[node name="Main" type="Node2D"]
script = ExtResource("1")
`

	script := fmt.Sprintf(`extends Node2D

# %s starts here. Build the game from code in _ready(), or add scenes beside
# this one and instance them.

func _ready() -> void:
	print("%s started")


func _process(_delta: float) -> void:
	pass
`, name, name)

	presets := fmt.Sprintf(`[preset.0]

name="Linux"
platform=%q
runnable=true
custom_features=""
export_filter="all_resources"
include_filter=""
exclude_filter=""
export_path="build/%s.x86_64"
encryption_include_filters=""
encryption_exclude_filters=""
encrypt_pck=false
encrypt_directory=false

[preset.0.options]

binary_format/embed_pck=true
`, platform, safeName(name))

	return []File{
		{Path: "project.godot", Content: []byte(project)},
		{Path: "main.tscn", Content: []byte(scene)},
		{Path: "main.gd", Content: []byte(script)},
		{Path: "export_presets.cfg", Content: []byte(presets)},
		{Path: ".gitignore", Content: []byte(".godot/\nbuild/\n")},
	}, nil
}

var godotResource = regexp.MustCompile(`path="res://([^"]+)"`)

// Validate reads the project without running anything.
func (g Godot) Validate(p Project) []Diagnostic {
	var found []Diagnostic

	config := read(filepath.Join(p.Dir, "project.godot"))

	if m := regexp.MustCompile(`(?m)^config_version=(\d+)`).FindStringSubmatch(config); m != nil && m[1] != "5" {
		found = append(found, Diagnostic{Severity: "warning", File: "project.godot",
			Message: "config_version " + m[1] + ": this project was made for Godot 3, and Godot 4 will convert it"})
	}

	switch main := p.Details["main_scene"]; {
	case main == "":
		found = append(found, Diagnostic{Severity: "warning", File: "project.godot",
			Message: "no main scene: running the project opens nothing"})
	case !exists(filepath.Join(p.Dir, strings.TrimPrefix(main, "res://"))):
		found = append(found, Diagnostic{Severity: "error", File: "project.godot",
			Message: "the main scene " + main + " does not exist"})
	}

	if engine, ok := g.Engine(); ok && p.Targets != "" {
		have := majorMinor(engine.Version)

		switch {
		case provision.Newer(have, p.Targets):
			found = append(found, Diagnostic{Severity: "error", File: "project.godot",
				Message: fmt.Sprintf("made for Godot %s, and Godot %s is installed", p.Targets, have)})
		case provision.Newer(p.Targets, have):
			found = append(found, Diagnostic{Severity: "warning", File: "project.godot",
				Message: fmt.Sprintf("made for Godot %s; Godot %s will upgrade it when it is opened", p.Targets, have)})
		}
	}

	// Every resource a scene names has to be there, or loading it fails at
	// run time with a message about the scene rather than the missing file.
	filepath.WalkDir(p.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if name := d.Name(); path != p.Dir && (strings.HasPrefix(name, ".") || name == "addons" || name == "build") {
				return filepath.SkipDir
			}

			return nil
		}

		if ext := filepath.Ext(path); ext != ".tscn" && ext != ".tres" {
			return nil
		}

		for _, m := range godotResource.FindAllStringSubmatch(read(path), -1) {
			if !exists(filepath.Join(p.Dir, m[1])) {
				found = append(found, Diagnostic{Severity: "error", File: relative(p.Dir, path),
					Message: "it names res://" + m[1] + ", which does not exist"})
			}
		}

		return nil
	})

	return found
}

var godotAt = regexp.MustCompile(`\(res://([^:)]+):(\d+)\)`)

/*
 * godotDiagnostics reads the engine's complaints out of its chatter.
 *
 * Godot prints an error and, on the next line, where it happened — in the
 * project for a script error, inside the engine's own source for most
 * others. Only a place in the project is kept as a place: a line number in
 * gdscript.cpp is no use to anybody fixing a game.
 */
func godotDiagnostics(out string) []Diagnostic {
	lines := strings.Split(out, "\n")

	var found []Diagnostic

	seen := map[string]bool{}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		severity := ""
		message := ""

		for _, mark := range []struct{ prefix, severity string }{
			{"SCRIPT ERROR:", "error"}, {"USER ERROR:", "error"}, {"ERROR:", "error"},
			{"USER WARNING:", "warning"}, {"WARNING:", "warning"}, {"Parse Error:", "error"},
		} {
			if rest, ok := strings.CutPrefix(trimmed, mark.prefix); ok {
				severity, message = mark.severity, strings.TrimSpace(rest)

				break
			}
		}

		if severity == "" {
			continue
		}

		d := Diagnostic{Severity: severity, Message: message}

		// Where is on the line after, and only there: two lines on is the
		// next complaint's.
		if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "at:") {
			if m := godotAt.FindStringSubmatch(lines[i+1]); m != nil {
				d.File = m[1]
				d.Line, _ = strconv.Atoi(m[2])
			}
		}

		if key := d.String(); !seen[key] {
			seen[key] = true
			found = append(found, d)
		}
	}

	return found
}

// importFirst is how a project's resources are imported before it runs:
// --import from 4.3, the editor started and quit before that.
func importFirst(engine Installed) []string {
	if provision.Newer(majorMinor(engine.Version), "4.3") {
		return []string{"--headless", "--editor", "--quit"}
	}

	return []string{"--headless", "--import"}
}

/*
 * Check imports the project and runs thirty frames of it, headless.
 *
 * Headless always: "does it load" is answered without putting a window on
 * somebody's screen. Thirty frames rather than one, because a script that
 * fails in _process fails on the first frame that calls it, not while loading.
 */
func (g Godot) Check(ctx context.Context, p Project) Run {
	engine, ok := g.Engine()
	if !ok {
		return notInstalled(g, "check")
	}

	r := Run{What: "check", Engine: "godot", Version: engine.Version}
	r.Diagnostics = g.Validate(p)

	if r.Errors() > 0 {
		r.Notes = append(r.Notes, "not run: the project has errors that stop it loading")

		return r
	}

	out, _ := r.step(ctx, p.Dir, g.limit(), append([]string{engine.Path}, append(importFirst(engine), "--path", p.Dir)...)...)
	r.add(godotDiagnostics(out)...)

	if r.Problem != "" {
		return r
	}

	out, exited := r.step(ctx, p.Dir, g.limit(), engine.Path, "--headless", "--path", p.Dir, "--quit-after", "30")
	r.add(godotDiagnostics(out)...)

	r.withErrors(exited)

	return r
}

var presetName = regexp.MustCompile(`(?m)^name="([^"]+)"`)

// Build exports the project with its first preset, into a folder given.
func (g Godot) Build(ctx context.Context, p Project, into string) Run {
	engine, ok := g.Engine()
	if !ok {
		return notInstalled(g, "export")
	}

	r := Run{What: "export", Engine: "godot", Version: engine.Version}

	presets := read(filepath.Join(p.Dir, "export_presets.cfg"))

	m := presetName.FindStringSubmatch(presets)
	if m == nil {
		r.Problem = "the project has no export preset — export_presets.cfg is missing or empty"

		return r
	}

	if g.Preset != "" {
		m[1] = g.Preset
	}

	if templates := templatesFor(engine.Version); !exists(templates) {
		r.Problem = fmt.Sprintf("the export templates for Godot %s are not installed (%s) — "+
			"check_requirements godot-templates says what it would take", engine.Version, templates)

		return r
	}

	if err := os.MkdirAll(into, 0o755); err != nil {
		r.Problem = err.Error()

		return r
	}

	target := filepath.Join(into, safeName(p.Name)+".x86_64")

	if g.File != "" {
		target = g.File
	}

	out, exited := r.step(ctx, p.Dir, g.limit(), engine.Path, "--headless", "--path", p.Dir,
		"--export-release", m[1], target)
	r.Diagnostics = godotDiagnostics(out)

	if exists(target) {
		r.Artifacts = append(r.Artifacts, target)
	} else {
		exited = false
		r.Notes = append(r.Notes, "the export finished without writing "+target)
	}

	r.withErrors(exited)

	return r
}

// templatesFor is where Godot looks for its export templates: under
// XDG_DATA_HOME when that is set, as Godot itself does.
func templatesFor(version string) string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, _ := os.UserHomeDir()
		data = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(data, "godot", "export_templates", version+".stable")
}

/*
 * Smoke runs the game for a few seconds, and photographs it when it can.
 *
 * The run is headless and says whether it survives. The picture needs a
 * display to draw on, and putting a game window on somebody's screen is not
 * what a check is for — so it is taken on a virtual display when one is
 * installed, and otherwise the report says that is why there is none.
 */
func (g Godot) Smoke(ctx context.Context, p Project, pictures string) Run {
	engine, ok := g.Engine()
	if !ok {
		return notInstalled(g, "smoke")
	}

	r := Run{What: "smoke", Engine: "godot", Version: engine.Version}

	out, exited := r.step(ctx, p.Dir, g.limit(), engine.Path, "--headless", "--path", p.Dir, "--quit-after", "180")
	r.Diagnostics = godotDiagnostics(out)
	r.withErrors(exited)

	if !r.OK || pictures == "" {
		return r
	}

	if err := os.MkdirAll(pictures, 0o755); err != nil {
		r.Notes = append(r.Notes, "no picture: "+err.Error())

		return r
	}

	// A display of its own, never somebody's screen: see package display.
	screen, err := display.Start(ctx, 1280, 960)
	if err != nil {
		r.Notes = append(r.Notes, "no picture: "+err.Error())

		return r
	}

	defer screen.Stop()

	frames := filepath.Join(pictures, "frame.png")

	argv := []string{engine.Path, "--path", p.Dir, "--rendering-driver", "opengl3",
		"--write-movie", frames, "--fixed-fps", "30", "--quit-after", "60"}

	var env []string

	switch screen.Kind {
	case display.Mutter:
		argv = append([]string{engine.Path, "--display-driver", "wayland"}, argv[1:]...)
		env = screen.Env
	case display.Xvfb:
		argv = append(append([]string{}, screen.Wrap...), argv...)
	}

	shot := Run{}
	out, _ = shot.stepWith(ctx, p.Dir, 2*time.Minute, env, argv...)

	r.Commands = append(r.Commands, shot.Commands...)

	// What went wrong while it was being watched counts as much as what went
	// wrong headless.
	r.add(godotDiagnostics(out)...)

	if last := lastFrame(pictures); last != "" {
		r.Screenshot = last
		r.Notes = append(r.Notes, "photographed on a private "+string(screen.Kind)+" display")

		if d, flat := flatPicture(last); flat {
			r.add(d)
		}
	} else {
		r.Notes = append(r.Notes, "no picture: the engine wrote no frame — "+tail(shot.Output, 200))
	}

	r.withErrors(r.OK)

	return r
}

// lastFrame keeps the final frame of a movie and removes the rest: one
// picture is the evidence, sixty are a folder somebody has to clean up.
func lastFrame(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "frame*.png"))

	if len(matches) == 0 {
		return ""
	}

	sort.Strings(matches)

	last := matches[len(matches)-1]

	for _, m := range matches[:len(matches)-1] {
		os.Remove(m)
	}

	final := filepath.Join(dir, "godot.png")

	if err := os.Rename(last, final); err != nil {
		return last
	}

	return final
}
