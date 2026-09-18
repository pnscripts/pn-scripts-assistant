package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * The recipes this program ships.
 *
 * Three kinds of install, and nothing else:
 *
 *   pinned   a release fixed by version and checksum, from the project's own
 *            host — Godot, its export templates, Node.js. Downloaded, hashed,
 *            unpacked into the home folder. No password, nothing run.
 *   system   a package from the distribution, by name, through the same
 *            graphical password prompt the machine parts use. The package
 *            manager decides the version; the rule is checked afterwards.
 *   none     Unity, Unreal, Rust, Chrome: each needs an account, a licence
 *            somebody has to accept, or an installer script this program will
 *            not pipe into a shell. Detected, reported with what it costs and
 *            what to do — and never installed or agreed to on anybody's behalf.
 */

// Standard is every recipe this program ships, the machine parts included.
func Standard() *Book {
	recipes := []Recipe{
		godotRecipe(),
		godotTemplates(),
		nodeRecipe(),
		{
			ID: "npm", Title: "npm", Kind: Tool,
			Why:      "installs a JavaScript project's dependencies",
			Commands: []string{"npm"}, Places: []string{"~/.local/bin"},
			VersionArgs: []string{"--version"},
			Source:      "comes with Node.js", Licence: "Artistic 2.0", Cost: "free",
			Manual: "npm comes with Node.js — install that, and npm arrives with it",
		},
		system("git", "Git", Tool, "keeps a project's history, so every change can be seen and undone",
			"git", []string{"git"}, "GPL-2.0", []string{"--version"}),
		system("python3", "Python 3", Runtime, "runs Python programs and tools",
			"python3", []string{"python3"}, "PSF licence", []string{"--version"}),
		system("pandoc", "Pandoc", Tool, "turns a manuscript into EPUB, Word or PDF",
			"pandoc", []string{"pandoc"}, "GPL-2.0", []string{"--version"}),
		system("libreoffice", "LibreOffice", Viewer, "opens and converts documents and spreadsheets",
			"libreoffice", []string{"libreoffice", "soffice"}, "MPL-2.0", []string{"--version"}),
		system("librecad", "LibreCAD", Viewer, "opens 2D drawings (DXF) such as floor and wiring plans",
			"librecad", []string{"librecad"}, "GPL-2.0", nil),
		system("freecad", "FreeCAD", Viewer, "opens 3D and building models (STEP, IFC)",
			"freecad", []string{"freecad", "freecadcmd"}, "LGPL-2.0", []string{"--version"}),
		system("inkscape", "Inkscape", Tool, "draws and converts vector graphics (SVG)",
			"inkscape", []string{"inkscape"}, "GPL-3.0", []string{"--version"}),
		system("imagemagick", "ImageMagick", Tool, "resizes and converts pictures",
			"imagemagick", []string{"convert", "magick"}, "ImageMagick licence (Apache-style)", []string{"-version"}),
		system("blender", "Blender", Tool, "models, textures and renders 3D assets",
			"blender", []string{"blender"}, "GPL-2.0", []string{"--version"}),
		system("xvfb", "Virtual display (Xvfb)", Tool,
			"lets a game be run and photographed without a window appearing on the screen",
			"xvfb", []string{"xvfb-run"}, "MIT/X11", nil),
		{
			ID: "go", Title: "Go", Kind: Runtime,
			Why:      "builds Go programs",
			Commands: []string{"go"}, Places: []string{"/usr/local/go/bin", "~/go/bin", "~/.local/go/bin"},
			VersionArgs: []string{"version"}, VersionPattern: regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`),
			Source: "go.dev/dl", Licence: "BSD-3-Clause", Cost: "free",
			Manual: "Download the archive for Linux from go.dev/dl and unpack it into /usr/local " +
				"(or your distribution's golang package).",
		},
		{
			ID: "cargo", Title: "Rust (cargo)", Kind: Runtime,
			Why:      "builds Rust programs, including Bevy games",
			Commands: []string{"cargo"}, Places: []string{"~/.cargo/bin"},
			VersionArgs: []string{"--version"},
			Source:      "rust-lang.org", Licence: "MIT or Apache-2.0", Cost: "free",
			Manual: "Install Rust with rustup from rust-lang.org. This program does not run its " +
				"installer: the published method pipes a script from the internet into a shell.",
		},
		{
			ID: "chrome", Title: "Chrome or Chromium", Kind: Tool,
			Why:         "tests a web page in a real browser, headless",
			Commands:    []string{"google-chrome", "chromium", "chromium-browser"},
			VersionArgs: []string{"--version"},
			Source:      "google.com/chrome or your distribution", Licence: "Chrome: Google's terms; Chromium: BSD",
			Cost:   "free",
			Manual: "Install Chromium from your distribution, or Chrome from google.com/chrome.",
		},
		unityRecipe(),
		unrealRecipe(),
		{
			ID: "uv", Title: "uv (Python tools)", Kind: Tool,
			Why:      "runs Python-based MCP servers without installing them system-wide",
			Commands: []string{"uvx", "uv"}, Places: []string{"~/.local/bin", "~/.cargo/bin"},
			VersionArgs: []string{"--version"},
			Source:      "astral.sh", Licence: "MIT or Apache-2.0", Cost: "free",
			Manual: "Install uv from your distribution or from its GitHub releases " +
				"(github.com/astral-sh/uv). This program does not run its install script.",
		},
	}

	recipes = append(recipes, parts()...)

	return NewBook(recipes...)
}

// system is a recipe the distribution's package manager installs.
func system(id, title, kind, why, pkg string, commands []string, licence string, versionArgs []string) Recipe {
	req := preflight.Requirement{
		Name:       title,
		NeedsRoot:  true,
		InstallCmd: func() []string { return []string{"sudo", "apt-get", "install", "-y", pkg} },
		ManualHint: "sudo apt install " + pkg,
	}

	r := Recipe{
		ID: id, Title: title, Kind: kind, Why: why,
		Commands: commands, VersionArgs: versionArgs,
		Source:  "your distribution's package " + pkg,
		Licence: licence, Cost: "free",
		Manual: "sudo apt install " + pkg,

		// The argv is fixed here, the same shape the machine parts use, and it
		// asks for the password through the desktop rather than a terminal.
		NeedsRoot: true,
		InstallTo: func() string { return "/usr (through apt)" },
	}

	if _, err := exec.LookPath("apt-get"); err == nil {
		r.Install = func(_ context.Context, w io.Writer) error { return preflight.Install(req, w) }
	}

	return r
}

var godotVersion = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)

func godotRecipe() Recipe {
	r := Recipe{
		ID: "godot", Title: "Godot", Kind: Engine,
		Why: "the game engine: runs, checks and exports Godot projects",
		Find: func() (string, string, bool) {
			engine, ok := godot.Find()
			if !ok {
				return "", "", false
			}

			version := engine.Version

			if m := godotVersion.FindStringSubmatch(version); m != nil {
				version = m[1]
			}

			return engine.Path, version, true
		},
		Source:    "github.com/godotengine/godot, release " + preflight.GodotVersion + ", checked against its SHA-512",
		Licence:   "MIT",
		Cost:      "free",
		Size:      "78MB",
		InstallTo: func() string { return filepath.Join(preflight.LocalBin(), "godot4") },
		Smoke:     []string{"--headless", "--version"},
	}

	// The same installer the machine parts offer, not a second one beside it.
	if part, ok := partNamed("Godot (making games)"); ok {
		r.Install = func(_ context.Context, w io.Writer) error { return preflight.Install(part, w) }

		if part.RemoveFunc != nil {
			r.Remove = part.RemoveFunc
		}
	}

	return r
}

/*
 * The export templates, which must be exactly the engine's version.
 *
 * Godot refuses to export with templates from another release, and says so
 * only when the export is attempted — so the rule here is not a range but the
 * installed engine's own number, and installing refuses to put 4.7.2's
 * templates beside a 4.3 engine.
 */
func templatesFolder(version string) string {
	home, _ := os.UserHomeDir()

	return filepath.Join(home, ".local", "share", "godot", "export_templates", version+".stable")
}

func godotTemplates() Recipe {
	pinned := preflight.GodotTemplates()
	wanted := strings.TrimSuffix(preflight.GodotVersion, "-stable")

	return Recipe{
		ID: "godot-templates", Title: "Godot export templates", Kind: Engine,
		Why: "lets Godot write a finished game for somebody else to run",
		Find: func() (string, string, bool) {
			version := wanted

			if _, found, ok := godotRecipe().Find(); ok && found != "" {
				version = found
			}

			dir := templatesFolder(version)

			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) == 0 {
				return "", "", false
			}

			return dir, version, true
		},
		Source: "github.com/godotengine/godot, release " + preflight.GodotVersion +
			", checked against its SHA-512",
		Licence:   pinned.Licence,
		Cost:      "free",
		Size:      "1.3GB",
		InstallTo: func() string { return templatesFolder(wanted) },
		Install: func(_ context.Context, w io.Writer) error {
			if _, version, ok := godotRecipe().Find(); ok && version != wanted {
				return fmt.Errorf("the templates must match the engine: Godot %s is installed, and "+
					"these are for %s — install Godot %s first", version, wanted, wanted)
			}

			temp, err := os.MkdirTemp("", "pn-scripts-assistant-templates-*")
			if err != nil {
				return err
			}

			defer os.RemoveAll(temp)

			archive := filepath.Join(temp, "templates.tpz")

			if err := preflight.Fetch(pinned, archive, w); err != nil {
				return err
			}

			// The archive holds a single templates/ folder.
			return preflight.UnpackZip(archive, templatesFolder(wanted), 1, w)
		},
		Remove: func(w io.Writer) error {
			fmt.Fprintf(w, "Removing %s\n", templatesFolder(wanted))

			return os.RemoveAll(templatesFolder(wanted))
		},
	}
}

// nodeHome is where the pinned Node.js is unpacked, in this program's own
// folder rather than anywhere another program considers its own.
func nodeHome() string {
	return filepath.Join(paths.MachineFolder(), "node", preflight.NodeVersion)
}

func nodeRecipe() Recipe {
	return Recipe{
		ID: "node", Title: "Node.js", Kind: Runtime,
		Why:      "runs JavaScript tooling: web projects, their tests, and MCP servers written for Node",
		Commands: []string{"node"}, Places: []string{"~/.local/bin"},
		VersionArgs: []string{"--version"},
		Source:      "nodejs.org, release " + preflight.NodeVersion + " (long-term support), checked against its SHA-256",
		Licence:     "MIT",
		Cost:        "free",
		Size:        "58MB",
		InstallTo:   nodeHome,
		Smoke:       []string{"-e", "process.exit(0)"},
		Install: func(_ context.Context, w io.Writer) error {
			pinned, err := preflight.Node()
			if err != nil {
				return err
			}

			temp, err := os.MkdirTemp("", "pn-scripts-assistant-node-*")
			if err != nil {
				return err
			}

			defer os.RemoveAll(temp)

			archive := filepath.Join(temp, "node.tar.gz")

			if err := preflight.Fetch(pinned, archive, w); err != nil {
				return err
			}

			if err := preflight.UnpackTarGz(archive, nodeHome(), 1, w); err != nil {
				return err
			}

			// Linked into ~/.local/bin, but never over something that is not
			// ours: a node another program put there stays where it was.
			for _, name := range []string{"node", "npm", "npx"} {
				link := filepath.Join(preflight.LocalBin(), name)

				if at, err := os.Readlink(link); err == nil && !strings.HasPrefix(at, filepath.Dir(nodeHome())) {
					fmt.Fprintf(w, "Left %s alone: it points at %s\n", link, at)

					continue
				} else if err != nil {
					if _, statErr := os.Lstat(link); statErr == nil {
						fmt.Fprintf(w, "Left %s alone: it is not a link this program made\n", link)

						continue
					}
				}

				os.Remove(link)

				if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
					return err
				}

				if err := os.Symlink(filepath.Join(nodeHome(), "bin", name), link); err != nil {
					return err
				}
			}

			return nil
		},
		Remove: func(w io.Writer) error {
			for _, name := range []string{"node", "npm", "npx"} {
				link := filepath.Join(preflight.LocalBin(), name)

				if at, err := os.Readlink(link); err == nil && strings.HasPrefix(at, filepath.Dir(nodeHome())) {
					os.Remove(link)
				}
			}

			fmt.Fprintf(w, "Removing %s\n", nodeHome())

			return os.RemoveAll(nodeHome())
		},
	}
}

/*
 * Unity: found, never installed.
 *
 * The editor comes through Unity Hub, which needs a Unity account and its
 * owner's acceptance of Unity's terms, and the plan it runs under depends on
 * the revenue of whoever is using it. None of that is this program's to agree
 * to — so it looks for an editor the Hub put here and says what it would take.
 */
func unityRecipe() Recipe {
	return Recipe{
		ID: "unity", Title: "Unity editor", Kind: Engine,
		Why:  "opens, builds and tests Unity projects",
		Find: FindUnity,
		Licensed: func(path string) (string, string) {
			l := UnityLicence(path)

			return l.State, l.Why
		},
		Source:  "unity.com, through Unity Hub",
		Licence: "Unity's own terms, accepted by its owner in Unity Hub",
		Cost: "Unity Personal is free below Unity's revenue threshold; above it a paid plan " +
			"(Pro, Enterprise) is required",
		Size: "5–15GB with one build platform",
		Manual: "Install Unity Hub from unity.com, sign in with your Unity account, accept Unity's " +
			"terms and choose a licence, then install an editor version from the Hub. This program " +
			"will find it afterwards; it will not sign in or accept terms for you.",
	}
}

// UnityEditorsIn is where the Hub puts editors, one folder per version.
func UnityEditorsIn() []string {
	home, _ := os.UserHomeDir()

	return []string{
		filepath.Join(home, "Unity", "Hub", "Editor"),
		"/opt/Unity/Hub/Editor",
		"/opt/unity/editors",
	}
}

// FindUnity is the Unity editor to use: see UnityInstalls.
func FindUnity() (string, string, bool) {
	all := UnityInstalls()

	if len(all) == 0 {
		if at, err := exec.LookPath("unity-editor"); err == nil {
			return at, "", true
		}

		return "", "", false
	}

	return all[0].Path, all[0].Version, true
}

/*
 * Unreal: the same, and larger.
 *
 * Epic's engine needs an Epic Games account and its licence, is built from
 * source or downloaded as a hundred-gigabyte binary on Linux, and carries a
 * royalty above a revenue threshold. Found where people put it, read for its
 * version from the engine's own build file, and never installed from here.
 */
func unrealRecipe() Recipe {
	return Recipe{
		ID: "unreal", Title: "Unreal Engine", Kind: Engine,
		Why:     "opens, builds and packages Unreal projects",
		Find:    FindUnreal,
		Source:  "unrealengine.com / Epic Games' GitHub, with an Epic Games account",
		Licence: "the Unreal Engine EULA, accepted by its owner with their Epic Games account",
		Cost:    "free to use; a royalty applies to a product's gross revenue above Epic's threshold",
		Size:    "about 100GB installed",
		Manual: "Sign in to Epic Games, accept the Unreal Engine EULA, and either download the " +
			"Linux build from unrealengine.com or build it from Epic's GitHub repository. Tell " +
			"this program where it is by putting it in ~/UnrealEngine or /opt/UnrealEngine.",
	}
}

// UnrealRoots are where an engine is looked for.
func UnrealRoots() []string {
	home, _ := os.UserHomeDir()

	return []string{
		filepath.Join(home, "UnrealEngine"),
		filepath.Join(home, "Unreal", "UnrealEngine"),
		"/opt/UnrealEngine",
		"/opt/unreal-engine",
	}
}

// FindUnreal is the Unreal editor and its version, read from the engine.
func FindUnreal() (string, string, bool) {
	for _, root := range UnrealRoots() {
		editor := filepath.Join(root, "Engine", "Binaries", "Linux", "UnrealEditor")

		if _, err := os.Stat(editor); err != nil {
			continue
		}

		return editor, unrealVersion(root), true
	}

	if at, err := exec.LookPath("UnrealEditor"); err == nil {
		root := filepath.Join(filepath.Dir(at), "..", "..", "..")

		return at, unrealVersion(root), true
	}

	return "", "", false
}

// unrealVersion reads Engine/Build/Build.version, which the engine writes
// about itself.
func unrealVersion(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "Engine", "Build", "Build.version"))
	if err != nil {
		return ""
	}

	var v struct {
		MajorVersion, MinorVersion, PatchVersion int
	}

	if json.Unmarshal(raw, &v) != nil || v.MajorVersion == 0 {
		return ""
	}

	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.PatchVersion)
}

// partNamed is one machine part, by the name the Parts panel shows.
func partNamed(name string) (preflight.Requirement, bool) {
	for _, r := range preflight.Requirements() {
		if r.Name == name {
			return r, true
		}
	}

	return preflight.Requirement{}, false
}

// PartID is the recipe id a machine part goes by.
func PartID(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if s := b.String(); s != "" && !strings.HasSuffix(s, "-") {
				b.WriteRune('-')
			}
		}
	}

	return "part." + strings.Trim(b.String(), "-")
}

/*
 * parts are the machine parts, as recipes.
 *
 * The same checks and the same installers the setup screen and the Parts
 * panel use, so there is one answer to "is the voice installed" wherever it
 * is asked. Their version is whatever their check reports.
 */
func parts() []Recipe {
	reqs := preflight.Requirements()
	out := make([]Recipe, 0, len(reqs))

	sort.SliceStable(reqs, func(i, j int) bool { return reqs[i].Name < reqs[j].Name })

	for _, req := range reqs {
		req := req

		r := Recipe{
			ID: PartID(req.Name), Title: req.Name, Kind: Part,
			Why: req.Why, Size: req.Size, NeedsRoot: req.NeedsRoot,
			Manual:  req.ManualHint,
			Source:  "this program's machine parts",
			Licence: "see the Parts panel", Cost: "free",
			Find: func() (string, string, bool) {
				state, detail := req.Check()
				if state != preflight.OK {
					return "", "", false
				}

				return req.Location(), anyVersion.FindString(detail), true
			},
		}

		if req.Where != nil {
			r.InstallTo = req.Where
		}

		if req.Installable() {
			r.Install = func(_ context.Context, w io.Writer) error {
				_, err := preflight.InstallAndVerify(req)

				return err
			}
		}

		if req.RemoveFunc != nil {
			r.Remove = req.RemoveFunc
		}

		out = append(out, r)
	}

	return out
}
