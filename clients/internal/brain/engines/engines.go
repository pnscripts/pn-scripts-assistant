/*
 * Package engines is every game engine this program can work with, behind one
 * contract.
 *
 * Godot came first and was three tools that knew about Godot. That is fine for
 * one engine and wrong for five: a planner, a proposal and a report should not
 * each learn what Unity's log looks like. So every engine answers the same
 * questions — is it here and which version, is this folder one of its
 * projects, where is the documentation for that version, what does a new
 * project look like, is this one well formed, does it load, build it, run it
 * for a moment and take a picture — and answers them the same way: with a Run
 * that says exactly which command ran, how it ended, what the engine
 * complained about in a form a person can act on, and what it produced.
 *
 * Nothing here decides whether something may run. The tools that call this
 * are gated like every other tool, and the folders they may touch are decided
 * by the workspace. This is the part that knows the engines.
 */
package engines

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
)

// Installed is an engine found on this machine.
type Installed struct {
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`

	// Where says it in words, for the ones that are not a program — three.js
	// lives in each project, and a copy ships with this one.
	Where string `json:"where,omitempty"`
}

// Project is one project of one engine.
type Project struct {
	Engine string `json:"engine"`
	Dir    string `json:"dir"`
	Name   string `json:"name"`

	// Targets is the engine version the project says it was made for.
	Targets string `json:"targets,omitempty"`

	Details map[string]string `json:"details,omitempty"`
}

// Diagnostic is one thing an engine complained about.
type Diagnostic struct {
	Severity string `json:"severity"` // error or warning
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

func (d Diagnostic) String() string {
	where := d.File

	if where != "" && d.Line > 0 {
		where += ":" + strconv.Itoa(d.Line)
	}

	if where != "" {
		where = " (" + where + ")"
	}

	return d.Severity + ": " + d.Message + where
}

// Run is what came of asking an engine to do something: the evidence.
type Run struct {
	What    string `json:"what"`
	Engine  string `json:"engine"`
	Version string `json:"version,omitempty"`

	// Commands is every command that ran, exactly, in order.
	Commands [][]string `json:"commands,omitempty"`
	Dir      string     `json:"dir,omitempty"`
	Exit     int        `json:"exit"`
	Took     string     `json:"took,omitempty"`

	// Output is the end of what the engine printed, where it says what went
	// wrong.
	Output string `json:"output,omitempty"`

	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Artifacts   []string     `json:"artifacts,omitempty"`
	Screenshot  string       `json:"screenshot,omitempty"`

	// Notes are things worth knowing that are not the engine's complaints:
	// why no picture was taken, what was skipped.
	Notes []string `json:"notes,omitempty"`

	OK bool `json:"ok"`

	// Problem is why it could not be done at all.
	Problem string `json:"problem,omitempty"`
}

// Errors is how many diagnostics are errors.
func (r Run) Errors() int {
	n := 0

	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			n++
		}
	}

	return n
}

/*
 * Summary is the run in words, for the model and the report: what ran, how it
 * ended, what was complained about, what was made. Exact, because this is what
 * somebody reads to decide whether the work is finished.
 */
func (r Run) Summary() string {
	var b strings.Builder

	state := "passed"

	switch {
	case r.Problem != "":
		state = "could not run: " + r.Problem
	case !r.OK:
		state = "failed"
	}

	fmt.Fprintf(&b, "%s %s: %s", r.Engine, r.What, state)

	if r.Version != "" {
		fmt.Fprintf(&b, " (engine %s)", r.Version)
	}

	for _, argv := range r.Commands {
		fmt.Fprintf(&b, "\n  ran: %s", quoted(argv))
	}

	if len(r.Commands) > 0 {
		fmt.Fprintf(&b, "\n  exit status %d", r.Exit)

		if r.Took != "" {
			fmt.Fprintf(&b, " after %s", r.Took)
		}
	}

	shown := r.Diagnostics
	if len(shown) > 15 {
		shown = shown[:15]
	}

	for _, d := range shown {
		b.WriteString("\n  " + d.String())
	}

	if len(r.Diagnostics) > len(shown) {
		fmt.Fprintf(&b, "\n  … and %d more", len(r.Diagnostics)-len(shown))
	}

	for _, a := range r.Artifacts {
		b.WriteString("\n  made: " + a)
	}

	if r.Screenshot != "" {
		b.WriteString("\n  picture: " + r.Screenshot)
	}

	for _, n := range r.Notes {
		b.WriteString("\n  note: " + n)
	}

	if !r.OK && r.Output != "" && len(r.Diagnostics) == 0 {
		b.WriteString("\n  the engine said:\n" + indent(r.Output))
	}

	return b.String()
}

func indent(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	for i := range lines {
		lines[i] = "    " + lines[i]
	}

	return strings.Join(lines, "\n")
}

// quoted is a command as it would be typed, with anything spaced quoted.
func quoted(argv []string) string {
	out := make([]string, 0, len(argv))

	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			out = append(out, strconv.Quote(a))

			continue
		}

		out = append(out, a)
	}

	return strings.Join(out, " ")
}

// File is one file a new project starts with, relative to its folder.
type File struct {
	Path    string      `json:"path"`
	Content []byte      `json:"-"`
	Mode    fs.FileMode `json:"-"`
}

// Scaffold is what a new project is asked to be.
type Scaffold struct {
	Name string

	// Engine is the installed engine's version, so a new project is made for
	// the engine that will open it.
	Engine string
}

// Doc is somewhere authoritative for one version.
type Doc struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

/*
 * Adapter is one engine.
 *
 * Every question is answered, even by an engine that cannot do the thing: a
 * Run with a Problem saying why is an answer, and "this engine has no headless
 * mode" is worth knowing in exactly those words.
 */
type Adapter interface {
	ID() string
	Title() string

	// Recipe is the provision recipe that installs or finds the engine.
	Recipe() string

	// Licensed is an engine its owner has to install and accept terms for.
	Licensed() bool

	Engine() (Installed, bool)
	Detect(dir string) (Project, bool)
	Docs(version, topic string) []Doc
	Scaffold(s Scaffold) ([]File, error)
	Validate(p Project) []Diagnostic
	Check(ctx context.Context, p Project) Run
	Build(ctx context.Context, p Project, into string) Run
	Smoke(ctx context.Context, p Project, pictures string) Run
}

// Registry is every engine, in the order they are tried.
type Registry struct {
	list []Adapter
}

// NewRegistry holds adapters in the order given.
func NewRegistry(adapters ...Adapter) *Registry {
	return &Registry{list: adapters}
}

// Get is one engine by id.
func (r *Registry) Get(id string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}

	for _, a := range r.list {
		if a.ID() == id {
			return a, true
		}
	}

	return nil, false
}

// Known answers whether an id is an engine, for checking a package.
func (r *Registry) Known(id string) bool {
	_, ok := r.Get(id)

	return ok
}

// All is every engine.
func (r *Registry) All() []Adapter {
	if r == nil {
		return nil
	}

	return append([]Adapter{}, r.list...)
}

// Detect is the engine a folder is a project of, first match wins — which is
// why the more specific engines are tried before the general web one.
func (r *Registry) Detect(dir string) (Adapter, Project, bool) {
	if r == nil {
		return nil, Project{}, false
	}

	for _, a := range r.list {
		if p, ok := a.Detect(dir); ok {
			return a, p, true
		}
	}

	return nil, Project{}, false
}

// skip are folders never looked inside for projects: somebody else's code,
// or an engine's own output.
var skip = map[string]bool{
	"node_modules": true, "vendor": true, "addons": true, "library": true, "temp": true,
	"intermediate": true, "binaries": true, "saved": true, "deriveddatacache": true,
	"target": true, "dist": true, "build": true, "builds": true, "packaged": true,
}

/*
 * Find is the projects under a folder, of any engine, at most most of them.
 *
 * A folder that is a project is not looked inside: a Godot project's addons
 * are projects too, and listing them as somebody's games is the mistake the
 * document scanner once made with node_modules.
 */
func (r *Registry) Find(root string, most int) []Project {
	var found []Project

	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || len(found) >= most {
			if len(found) >= most {
				return fs.SkipAll
			}

			return nil
		}

		name := strings.ToLower(d.Name())

		if path != root && (strings.HasPrefix(name, ".") || skip[name]) {
			return fs.SkipDir
		}

		if _, p, ok := r.Detect(path); ok {
			found = append(found, p)

			return fs.SkipDir
		}

		// Deep enough for ~/Projects/client/game, not a walk of the disk.
		if rel, err := filepath.Rel(root, path); err == nil && strings.Count(rel, string(filepath.Separator)) >= 3 {
			return fs.SkipDir
		}

		return nil
	})

	sort.Slice(found, func(i, j int) bool { return found[i].Dir < found[j].Dir })

	return found
}

// MostOutput is how much of an engine's output is kept on a run.
const MostOutput = 6000

/*
 * run carries out one command: no shell, a time limit, stdin closed, and the
 * output kept whatever happens.
 *
 * A command that is still running at the limit is stopped and reported as
 * that, rather than as a failure of whatever it was doing — "it took too
 * long" and "it broke" are different things to fix.
 */
func run(ctx context.Context, dir string, limit time.Duration, env []string, argv ...string) (exit int, out string, took time.Duration, err error) {
	if len(argv) == 0 {
		return -1, "", 0, errors.New("no command")
	}

	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	/*
	 * Nothing of this program's goes with it, and on a project the kernel
	 * holds it to the project: an engine runs the project's own scripts, and
	 * a game's code writing somebody's home folder is exactly what a check
	 * must not be the way in for.
	 */
	spec := sandbox.Spec{Dir: dir, Env: env}

	if writable, confined := sandbox.WritableFrom(ctx); confined {
		spec.Writable = writable
	}

	cmd, cmdErr := sandbox.Command(ctx, spec, argv...)
	if cmdErr != nil {
		return -1, "", 0, cmdErr
	}

	var buf bytes.Buffer

	cmd.Stdout = &buf
	cmd.Stderr = &buf

	started := time.Now()
	runErr := cmd.Run()
	took = time.Since(started).Round(100 * time.Millisecond)
	out = buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return -1, out, took, fmt.Errorf("still running after %s, and stopped", limit)
	}

	var exitErr *exec.ExitError

	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), out, took, nil
	}

	if runErr != nil {
		return -1, out, took, fmt.Errorf("could not run %s: %w", filepath.Base(argv[0]), runErr)
	}

	return 0, out, took, nil
}

// tail is the end of some output, where engines say what went wrong.
func tail(out string, most int) string {
	out = strings.TrimSpace(out)

	if len(out) <= most {
		return out
	}

	return "…" + out[len(out)-most:]
}

// step runs one command into a Run, adding to what is already there.
func (r *Run) step(ctx context.Context, dir string, limit time.Duration, argv ...string) (string, bool) {
	return r.stepWith(ctx, dir, limit, nil, argv...)
}

// stepWith is step with something added to the program's environment — a
// display to draw on.
func (r *Run) stepWith(ctx context.Context, dir string, limit time.Duration, env []string, argv ...string) (string, bool) {
	r.Commands = append(r.Commands, append([]string{}, argv...))
	r.Dir = dir

	exit, out, took, err := run(ctx, dir, limit, env, argv...)

	r.Exit = exit
	r.Took = took.String()
	r.Output = tail(out, MostOutput)

	if err != nil {
		r.Problem = err.Error()

		return out, false
	}

	return out, exit == 0
}

// exists is whether a path is there.
func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// read is a file's text, or empty.
func read(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(raw)
}

// majorMinor is "4.7" from "4.7.2.stable" and "6000.0" from "6000.0.23f1".
func majorMinor(version string) string {
	m := regexp.MustCompile(`(\d+)\.(\d+)`).FindStringSubmatch(version)
	if m == nil {
		return ""
	}

	return m[1] + "." + m[2]
}

// safeName is a project name as a file name: letters, digits and dashes.
func safeName(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "game"
	}

	return out
}

// className is a project name as a C# or C++ identifier.
func className(name string) string {
	var b strings.Builder

	up := true

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			if up {
				r -= 'a' - 'A'
			}

			b.WriteRune(r)

			up = false
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9' && b.Len() > 0:
			b.WriteRune(r)

			up = false
		default:
			up = true
		}
	}

	if b.Len() == 0 {
		return "Game"
	}

	return b.String()
}

var (
	cSharpLine = regexp.MustCompile(`(?m)^\s*(.+?\.cs)\((\d+),\d+\):\s*(error|warning)\s+(\w+):\s*(.+)$`)
	clangLine  = regexp.MustCompile(`(?m)^(.+?\.(?:cpp|h|hpp|cs)):(\d+):(?:\d+:)?\s*(error|warning):\s*(.+)$`)
	rustError  = regexp.MustCompile(`(?m)^(error|warning)(?:\[\w+\])?:\s*(.+)\n\s*-->\s*(.+?):(\d+):\d+`)
)

// csharpDiagnostics reads the compiler's lines out of a Unity log.
func csharpDiagnostics(out, root string) []Diagnostic {
	var found []Diagnostic

	seen := map[string]bool{}

	for _, m := range cSharpLine.FindAllStringSubmatch(out, -1) {
		line, _ := strconv.Atoi(m[2])
		d := Diagnostic{Severity: m[3], File: relative(root, m[1]), Line: line, Message: m[4] + ": " + m[5]}

		if key := d.String(); !seen[key] {
			seen[key] = true
			found = append(found, d)
		}
	}

	return found
}

// clangDiagnostics reads a C++ compiler's lines.
func clangDiagnostics(out, root string) []Diagnostic {
	var found []Diagnostic

	for _, m := range clangLine.FindAllStringSubmatch(out, -1) {
		line, _ := strconv.Atoi(m[2])
		found = append(found, Diagnostic{Severity: m[3], File: relative(root, m[1]), Line: line, Message: m[4]})
	}

	return found
}

// rustDiagnostics reads cargo's short and long forms.
func rustDiagnostics(out, root string) []Diagnostic {
	var found []Diagnostic

	for _, m := range rustError.FindAllStringSubmatch(out, -1) {
		line, _ := strconv.Atoi(m[4])
		found = append(found, Diagnostic{Severity: m[1], File: relative(root, m[3]), Line: line, Message: m[2]})
	}

	return found
}

func relative(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}

	return path
}

// add keeps diagnostics once each: a check that imports and then runs the
// project hears the same parse error twice, and it is one error.
func (r *Run) add(found ...Diagnostic) {
	seen := map[string]bool{}

	for _, d := range r.Diagnostics {
		seen[d.String()] = true
	}

	for _, d := range found {
		if key := d.String(); !seen[key] {
			seen[key] = true
			r.Diagnostics = append(r.Diagnostics, d)
		}
	}
}

// withErrors marks a run failed when any diagnostic is an error, whatever the
// exit status said — engines that exit zero with errors printed are common.
func (r *Run) withErrors(ok bool) {
	r.OK = ok && r.Problem == "" && r.Errors() == 0
}

// notInstalled is the Run for an engine that is not here.
func notInstalled(a Adapter, what string) Run {
	return Run{What: what, Engine: a.ID(),
		Problem: a.Title() + " is not installed on this machine — check_requirements says what it would take"}
}
