package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
)

/*
 * Engines that are a library in a web page: three.js, Phaser, Babylon.js,
 * PlayCanvas.
 *
 * One adapter with the library as a parameter, because what can be checked is
 * the same for all of them: the page and its modules are there and name each
 * other correctly, the scripts parse, and — the check that matters — the page
 * actually runs in a real browser engine without throwing, draws something,
 * and looks like something. That last part uses this program's own WebKit, so
 * nothing needs installing to check a web game.
 */

// PageShot is what running a page found. See browse.Snapshot.
type PageShot struct {
	Title   string
	Errors  []string
	Canvas  int
	Picture string
	Trouble string
}

// Snapper runs a page for some seconds and photographs it into png.
type Snapper func(ctx context.Context, url string, seconds int, png string) (PageShot, error)

// Web is one library-in-a-page engine.
type Web struct {
	Name     string
	Label    string
	Packages []string // package.json dependencies that mean this library
	Import   string   // what the import map calls it

	// Vendored is the library's own module, when this program carries a copy
	// a new project can start from, and Revision which release that is.
	Vendored func() []byte
	Revision string

	// DocsAt is the documentation for a version, most specific first.
	DocsAt func(version, topic string) []Doc

	Snap Snapper

	// Node finds node, for the syntax check. Nil means look on PATH.
	Node func() (string, bool)
}

// HowLongAWebBuildMayTake bounds a package's own build script.
const HowLongAWebBuildMayTake = 10 * time.Minute

func (w Web) ID() string     { return w.Name }
func (w Web) Title() string  { return w.Label }
func (w Web) Licensed() bool { return false }

// Recipe: nothing to install for the library itself, which lives in each
// project. Node is optional and checked where it is used.
func (w Web) Recipe() string { return "" }

func (w Web) Engine() (Installed, bool) {
	if w.Vendored != nil {
		return Installed{Version: w.Revision, Where: "a copy ships with this program; each project carries its own"}, true
	}

	return Installed{Where: "in each project"}, true
}

var threeRevision = regexp.MustCompile(`(?:REVISION\s*=\s*['"]|const t=")(\d+)`)

// Detect: a package.json naming the library, or a page whose import map or
// scripts do.
func (w Web) Detect(dir string) (Project, bool) {
	p := Project{Engine: w.Name, Dir: dir, Name: filepath.Base(dir), Details: map[string]string{}}

	if pkg := read(filepath.Join(dir, "package.json")); pkg != "" {
		var manifest struct {
			Name            string            `json:"name"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
			Scripts         map[string]string `json:"scripts"`
		}

		if json.Unmarshal([]byte(pkg), &manifest) == nil {
			for _, name := range w.Packages {
				version := manifest.Dependencies[name]
				if version == "" {
					version = manifest.DevDependencies[name]
				}

				if version != "" {
					p.Targets = strings.TrimLeft(version, "^~=v ")
					p.Details["package"] = name

					if manifest.Name != "" {
						p.Name = manifest.Name
					}

					if manifest.Scripts["build"] != "" {
						p.Details["build_script"] = "yes"
					}

					if w.Name == "threejs" {
						// three's npm version is 0.169.0 for r169.
						if m := regexp.MustCompile(`0\.(\d+)`).FindStringSubmatch(p.Targets); m != nil {
							p.Targets = m[1]
						}
					}

					return p, true
				}
			}
		}
	}

	page := read(filepath.Join(dir, "index.html"))
	if page == "" {
		return Project{}, false
	}

	lower := strings.ToLower(page)
	named := false

	for _, name := range append([]string{w.Import}, w.Packages...) {
		if name != "" && (strings.Contains(lower, `"`+strings.ToLower(name)+`"`) ||
			strings.Contains(lower, "/"+strings.ToLower(name))) {
			named = true
		}
	}

	if !named {
		return Project{}, false
	}

	if m := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(page); m != nil {
		p.Name = strings.TrimSpace(m[1])
	}

	if w.Name == "threejs" {
		for _, candidate := range []string{"vendor/three.module.js", "lib/three.module.js", "three.module.js"} {
			if head := read(filepath.Join(dir, candidate)); head != "" {
				if m := threeRevision.FindStringSubmatch(head[:min(len(head), 4000)]); m != nil {
					p.Targets = m[1]
				}

				p.Details["vendored"] = candidate
			}
		}
	}

	return p, true
}

func (w Web) Docs(version, topic string) []Doc {
	if w.DocsAt == nil {
		return nil
	}

	return w.DocsAt(version, topic)
}

// Scaffold is a page, a game module with the rules kept apart, and the
// library vendored beside them — no build step, no network, no npm.
func (w Web) Scaffold(s Scaffold) ([]File, error) {
	if w.Vendored == nil {
		return nil, fmt.Errorf("there is no %s template here: this program carries no copy of %s to start from",
			w.Label, w.Label)
	}

	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = "Game"
	}

	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<link rel="stylesheet" href="./style.css">
<script type="importmap">{"imports": {"three": "./vendor/three.module.js"}}</script>
</head>
<body>
<script type="module" src="./src/main.js"></script>
</body>
</html>
`, name)

	main := `import * as THREE from 'three';
import { Game } from './game.js';

// Drawing lives here; the rules live in game.js, where they can be checked
// without a screen.
const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
renderer.setSize(innerWidth, innerHeight);
document.body.appendChild(renderer.domElement);

const scene = new THREE.Scene();
scene.background = new THREE.Color(0x10141c);

const camera = new THREE.PerspectiveCamera(50, innerWidth / innerHeight, 0.1, 200);
camera.position.set(0, 0, 12);

scene.add(new THREE.HemisphereLight(0xffffff, 0x223344, 2));

const game = new Game();

addEventListener('keydown', (event) => game.press(event.key));

addEventListener('resize', () => {
  camera.aspect = innerWidth / innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(innerWidth, innerHeight);
});

let last = performance.now();

renderer.setAnimationLoop((now) => {
  game.step((now - last) / 1000);
  last = now;
  renderer.render(scene, camera);
});
`

	game := `// The rules of the game, with nothing about drawing in them.
export class Game {
  constructor() {
    this.score = 0;
    this.over = false;
  }

  press(key) {}

  step(seconds) {}
}
`

	style := "html, body { margin: 0; height: 100%; overflow: hidden; background: #10141c; }\ncanvas { display: block; }\n"

	return []File{
		{Path: "index.html", Content: []byte(page)},
		{Path: "src/main.js", Content: []byte(main)},
		{Path: "src/game.js", Content: []byte(game)},
		{Path: "style.css", Content: []byte(style)},
		{Path: "vendor/three.module.js", Content: w.Vendored()},
		{Path: ".gitignore", Content: []byte("dist/\n")},
	}, nil
}

var (
	relativeImport = regexp.MustCompile(`(?m)(?:import|export)[^'"]*?from\s*['"](\.{1,2}/[^'"]+)['"]|import\s*\(\s*['"](\.{1,2}/[^'"]+)['"]`)
	moduleScript   = regexp.MustCompile(`<script[^>]+src=["']([^"']+)["']`)
	importMap      = regexp.MustCompile(`(?s)<script type="importmap">(.*?)</script>`)
)

// Validate reads the page and its modules without running anything.
func (w Web) Validate(p Project) []Diagnostic {
	var found []Diagnostic

	page := read(filepath.Join(p.Dir, "index.html"))

	if page == "" {
		if p.Details["package"] != "" {
			return nil
		}

		return []Diagnostic{{Severity: "error", File: "index.html", Message: "there is no index.html to open"}}
	}

	if m := importMap.FindStringSubmatch(page); m != nil {
		var mapped struct {
			Imports map[string]string `json:"imports"`
		}

		if err := json.Unmarshal([]byte(m[1]), &mapped); err != nil {
			found = append(found, Diagnostic{Severity: "error", File: "index.html",
				Message: "the import map is not valid JSON: " + err.Error()})
		}

		for name, to := range mapped.Imports {
			switch {
			case strings.HasPrefix(to, "http://") || strings.HasPrefix(to, "https://"):
				found = append(found, Diagnostic{Severity: "warning", File: "index.html",
					Message: name + " is fetched from " + to + ": the game will not run offline, " +
						"and the version can change underneath it"})
			case !exists(filepath.Join(p.Dir, to)):
				found = append(found, Diagnostic{Severity: "error", File: "index.html",
					Message: "the import map points " + name + " at " + to + ", which does not exist"})
			}
		}
	}

	for _, m := range moduleScript.FindAllStringSubmatch(page, -1) {
		src := m[1]

		if strings.Contains(src, "://") || strings.HasPrefix(src, "//") {
			continue
		}

		if !exists(filepath.Join(p.Dir, src)) {
			found = append(found, Diagnostic{Severity: "error", File: "index.html",
				Message: "the page loads " + src + ", which does not exist"})
		}
	}

	for _, script := range scripts(p.Dir) {
		text := read(script)

		for _, m := range relativeImport.FindAllStringSubmatch(text, -1) {
			target := m[1]
			if target == "" {
				target = m[2]
			}

			if !exists(filepath.Join(filepath.Dir(script), target)) {
				message := "it imports " + target + ", which does not exist"

				// The one a model gets wrong every time: a path to three.js from
				// wherever it happens to be, where the page already maps a name.
				if name := mappedTo(p.Dir, target); name != "" {
					message += "; import it as '" + name + "', which index.html maps to it"
				}

				found = append(found, Diagnostic{Severity: "error", File: relative(p.Dir, script), Message: message})
			}
		}
	}

	return found
}

// mappedTo is the name index.html's import map gives a file of the same name
// as target, when it gives one: "three" for ./vendor/three.module.js.
func mappedTo(dir, target string) string {
	m := importMap.FindStringSubmatch(read(filepath.Join(dir, "index.html")))
	if m == nil {
		return ""
	}

	var imports struct {
		Imports map[string]string `json:"imports"`
	}

	if json.Unmarshal([]byte(m[1]), &imports) != nil {
		return ""
	}

	for name, to := range imports.Imports {
		if filepath.Base(to) == filepath.Base(target) {
			return name
		}
	}

	return ""
}

// scripts is the project's own JavaScript: not the vendored library, not
// dependencies, not a build.
func scripts(dir string) []string {
	var out []string

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()

			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" ||
				name == "dist" || name == "build" || name == "lib") {
				return filepath.SkipDir
			}

			return nil
		}

		if ext := filepath.Ext(path); ext == ".js" || ext == ".mjs" {
			out = append(out, path)
		}

		return nil
	})

	return out
}

var nodeSyntax = regexp.MustCompile(`(?m)^\[stdin\]:(\d+)`)

/*
 * Check is everything short of running the page: its files name each other
 * correctly, and every module parses.
 *
 * Parsing is node's, fed each module on its standard input as an ES module —
 * "node --check file.js" reads a file with import in it as the old kind of
 * script and calls every one of them broken.
 */
func (w Web) Check(ctx context.Context, p Project) Run {
	r := Run{What: "check", Engine: w.Name, Version: p.Targets}
	r.Diagnostics = w.Validate(p)

	node, ok := w.node()
	if !ok {
		r.Notes = append(r.Notes, "the scripts were not parsed: that needs Node.js (the node recipe)")
		r.withErrors(true)

		return r
	}

	for _, script := range scripts(p.Dir) {
		argv := []string{node, "--input-type=module", "--check"}

		r.Commands = append(r.Commands, append(append([]string{}, argv...), "<", relative(p.Dir, script)))

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)

		// Parsing runs nothing, and still goes the way every program here
		// goes: nothing of this program's in its environment.
		spec := sandbox.Spec{Dir: p.Dir}
		if writable, confined := sandbox.WritableFrom(ctx); confined {
			spec.Writable = writable
		}

		cmd, err := sandbox.Command(ctx, spec, argv...)
		if err != nil {
			cancel()

			continue
		}

		source, err := os.Open(script)
		if err != nil {
			cancel()

			continue
		}

		cmd.Stdin = source

		out, runErr := cmd.CombinedOutput()

		source.Close()
		cancel()

		if runErr == nil {
			continue
		}

		d := Diagnostic{Severity: "error", File: relative(p.Dir, script), Message: syntaxMessage(string(out))}

		if m := nodeSyntax.FindStringSubmatch(string(out)); m != nil {
			d.Line, _ = strconv.Atoi(m[1])
		}

		r.Diagnostics = append(r.Diagnostics, d)
		r.Exit = 1
	}

	r.withErrors(true)

	return r
}

func syntaxMessage(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Error:") {
			return strings.TrimSpace(line)
		}
	}

	return "it does not parse"
}

func (w Web) node() (string, bool) {
	if w.Node != nil {
		return w.Node()
	}

	path, err := exec.LookPath("node")

	return path, err == nil
}

/*
 * Build is the project's own build script when it has one, and otherwise a
 * copy of what the page needs into the folder given — a finished web game is
 * exactly the files a browser loads, and nothing else.
 */
func (w Web) Build(ctx context.Context, p Project, into string) Run {
	r := Run{What: "build", Engine: w.Name, Version: p.Targets}

	if p.Details["build_script"] == "yes" {
		npm, err := exec.LookPath("npm")
		if err != nil {
			r.Problem = "the project builds with npm, which is not installed (the node recipe brings it)"

			return r
		}

		_, exited := r.step(ctx, p.Dir, HowLongAWebBuildMayTake, npm, "run", "build")
		r.Diagnostics = clangDiagnostics(r.Output, p.Dir)

		for _, dist := range []string{"dist", "build"} {
			if exists(filepath.Join(p.Dir, dist, "index.html")) {
				r.Artifacts = append(r.Artifacts, filepath.Join(p.Dir, dist))
			}
		}

		r.withErrors(exited)

		return r
	}

	if err := os.MkdirAll(into, 0o755); err != nil {
		r.Problem = err.Error()

		return r
	}

	copied := 0

	for _, part := range []string{"index.html", "style.css", "src", "vendor", "assets", "lib"} {
		from := filepath.Join(p.Dir, part)

		if !exists(from) {
			continue
		}

		n, err := copyTree(from, filepath.Join(into, part))
		if err != nil {
			r.Problem = err.Error()

			return r
		}

		copied += n
	}

	r.Commands = append(r.Commands, []string{"copy", "index.html style.css src vendor assets lib", "->", into})
	r.Notes = append(r.Notes, fmt.Sprintf("%d files copied; there is no build step", copied))

	if exists(filepath.Join(into, "index.html")) {
		r.Artifacts = append(r.Artifacts, into)
		r.OK = true
	} else {
		r.Problem = "there was no index.html to build from"
	}

	return r
}

// copyTree copies a file or a folder, and says how many files.
func copyTree(from, to string) (int, error) {
	info, err := os.Stat(from)
	if err != nil {
		return 0, err
	}

	if !info.IsDir() {
		return 1, copyFile(from, to)
	}

	count := 0

	err = filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(to, strings.TrimPrefix(path, from))

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		count++

		return copyFile(path, target)
	})

	return count, err
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	in, err := os.Open(from)
	if err != nil {
		return err
	}

	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()

		return err
	}

	return out.Close()
}

// HowLongAPageRuns is how long a web game runs before it is photographed.
const HowLongAPageRuns = 3

/*
 * Smoke serves the project on this machine only, runs its page in this
 * program's own browser engine for a few seconds, and keeps what it
 * complained about and a picture of it.
 *
 * Served rather than opened as a file: browsers refuse modules from file://,
 * which would make every web game look broken. The server listens on the
 * loopback address, for this one run, and serves only the project folder.
 */
func (w Web) Smoke(ctx context.Context, p Project, pictures string) Run {
	r := Run{What: "smoke", Engine: w.Name, Version: p.Targets}

	if w.Snap == nil {
		r.Problem = "this build cannot run a page: it was built without WebKit"

		return r
	}

	root := p.Dir

	// A built project is run as built.
	if p.Details["package"] != "" && !exists(filepath.Join(root, "index.html")) {
		for _, dist := range []string{"dist", "build"} {
			if exists(filepath.Join(p.Dir, dist, "index.html")) {
				root = filepath.Join(p.Dir, dist)
			}
		}
	}

	url, stop, err := serve(root)
	if err != nil {
		r.Problem = err.Error()

		return r
	}

	defer stop()

	if err := os.MkdirAll(pictures, 0o755); err != nil {
		r.Problem = err.Error()

		return r
	}

	png := filepath.Join(pictures, w.Name+".png")

	r.Commands = append(r.Commands, []string{"run the page", url, "for", strconv.Itoa(HowLongAPageRuns) + "s"})

	started := time.Now()

	shot, err := w.Snap(ctx, url, HowLongAPageRuns, png)

	r.Took = time.Since(started).Round(100 * time.Millisecond).String()

	if err != nil {
		r.Problem = err.Error()

		return r
	}

	for _, e := range shot.Errors {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Severity: "error", Message: e})
	}

	if shot.Canvas == 0 {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Severity: "error",
			Message: "the page drew nothing: there is no canvas on it"})
	}

	if shot.Picture != "" {
		r.Screenshot = shot.Picture

		if d, flat := flatPicture(shot.Picture); flat {
			r.Diagnostics = append(r.Diagnostics, d)
		}
	} else if shot.Trouble != "" {
		r.Notes = append(r.Notes, "no picture: "+shot.Trouble)
	}

	r.Notes = append(r.Notes, fmt.Sprintf("the page is titled %q", shot.Title))
	r.withErrors(true)

	return r
}

// serve puts a folder on a loopback port until stopped.
func serve(dir string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("could not serve the page: %w", err)
	}

	files := http.FileServer(http.Dir(dir))

	server := &http.Server{
		// Nothing hidden: .git, .env and the project's own settings are not
		// part of any page, and a page has no business reading them.
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, part := range strings.Split(r.URL.Path, "/") {
				if strings.HasPrefix(part, ".") {
					http.NotFound(w, r)

					return
				}
			}

			files.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go server.Serve(listener)

	return "http://" + listener.Addr().String() + "/", func() { server.Close() }, nil
}

// ThreeJS is the three.js adapter, with the copy this program carries.
func ThreeJS(vendored func() []byte, revision string, snap Snapper) Web {
	return Web{
		Name: "threejs", Label: "three.js", Packages: []string{"three"}, Import: "three",
		Vendored: vendored, Revision: revision, Snap: snap,
		DocsAt: func(version, topic string) []Doc {
			if version == "" {
				version = revision
			}

			out := []Doc{}

			if topic != "" {
				out = append(out, Doc{Title: topic + " in three.js r" + version,
					URL: "https://raw.githubusercontent.com/mrdoob/three.js/r" + version + "/docs/list.json#" + topic})
			}

			return append(out,
				Doc{Title: "three.js r" + version + " documentation", URL: "https://github.com/mrdoob/three.js/tree/r" + version + "/docs"},
				Doc{Title: "three.js r" + version + " examples", URL: "https://github.com/mrdoob/three.js/tree/r" + version + "/examples"})
		},
	}
}

// Phaser, Babylon.js and PlayCanvas: found, checked, built and run; no copy
// ships to start a new one from.
func Phaser(snap Snapper) Web {
	return Web{Name: "phaser", Label: "Phaser", Packages: []string{"phaser"}, Import: "phaser", Snap: snap,
		DocsAt: func(version, topic string) []Doc {
			return []Doc{{Title: "Phaser API documentation", URL: "https://docs.phaser.io/api-documentation/api-documentation"}}
		}}
}

func Babylon(snap Snapper) Web {
	return Web{Name: "babylonjs", Label: "Babylon.js", Packages: []string{"@babylonjs/core", "babylonjs"},
		Import: "@babylonjs/core", Snap: snap,
		DocsAt: func(version, topic string) []Doc {
			return []Doc{{Title: "Babylon.js documentation", URL: "https://doc.babylonjs.com/typedoc/modules/BABYLON"}}
		}}
}

func PlayCanvas(snap Snapper) Web {
	return Web{Name: "playcanvas", Label: "PlayCanvas", Packages: []string{"playcanvas"}, Import: "playcanvas",
		Snap: snap,
		DocsAt: func(version, topic string) []Doc {
			return []Doc{{Title: "PlayCanvas engine API", URL: "https://api.playcanvas.com/engine/"}}
		}}
}
