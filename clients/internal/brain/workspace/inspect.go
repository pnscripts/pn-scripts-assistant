package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/undo"
)

/*
 * Looking at a folder before anything is written in it.
 *
 * The one thing this must never do is guess. A project is created where its
 * owner said and nowhere else — and before that, the folder they named is
 * looked at: is it there, is it empty, is it already a project and of what,
 * does it have changes nobody has committed, is it somewhere no project should
 * be at all. All of that is said before anything is proposed, because every
 * one of them changes what the proposal should be.
 */

// Report is what was found.
type Report struct {
	Dir     string `json:"dir"`
	Exists  bool   `json:"exists"`
	Empty   bool   `json:"empty"`
	Entries int    `json:"entries"`

	// Refused is why no project may be made or changed here at all.
	Refused string `json:"refused,omitempty"`

	Writable bool `json:"writable"`

	// Engine and Project are the game engine it is a project of, if any.
	Engine  string           `json:"engine,omitempty"`
	Project *engines.Project `json:"project,omitempty"`

	// Stacks are what else it is built with: node, go, python and so on.
	Stacks []string `json:"stacks,omitempty"`

	Config *Config `json:"config,omitempty"`

	Git    bool   `json:"git"`
	Branch string `json:"branch,omitempty"`
	Dirty  int    `json:"dirty,omitempty"`

	Conventions []string `json:"conventions,omitempty"`

	// Commands are the project's own build and test commands, as found.
	Commands map[string][]string `json:"commands,omitempty"`

	Risks []string `json:"risks,omitempty"`
}

// Forbidden are places no project is made in or changed, whoever asks.
var forbiddenRoots = []string{"/", "/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64",
	"/opt", "/proc", "/root", "/run", "/sbin", "/snap", "/srv", "/sys", "/usr", "/var"}

/*
 * refused is why a folder is no place for a project.
 *
 * The system's own folders; the home folder itself, whose every file would
 * become part of the project; hidden folders directly in it, which are
 * programs' settings; and this program's own folders, which a project written
 * into could corrupt the brain that is writing it.
 */
func refused(dir string, own []string) string {
	clean := real(dir)

	for _, root := range forbiddenRoots {
		if clean == root || (root != "/" && strings.HasPrefix(clean, root+"/")) {
			return clean + " belongs to the system, not to a project"
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		home = real(home)

		if clean == home {
			return "that is your whole home folder — a project needs a folder of its own inside it"
		}

		if rel, err := filepath.Rel(home, clean); err == nil && strings.HasPrefix(rel, ".") && !strings.HasPrefix(rel, "..") {
			return clean + " is a hidden folder where programs keep their settings"
		}
	}

	for _, mine := range own {
		if mine == "" {
			continue
		}

		mine = real(mine)

		if clean == mine || strings.HasPrefix(clean, mine+"/") || strings.HasPrefix(mine, clean+"/") {
			return clean + " holds this assistant's own memory, and a project there could damage it"
		}
	}

	return ""
}

// stacks are what a folder is built with, by the file that says so.
var stacks = []struct{ file, stack string }{
	{"package.json", "node"}, {"go.mod", "go"}, {"pyproject.toml", "python"},
	{"requirements.txt", "python"}, {"composer.json", "php"}, {"Cargo.toml", "rust"},
	{"pom.xml", "java"}, {"build.gradle", "java"}, {"Gemfile", "ruby"},
	{"index.html", "web page"}, {"project.godot", "godot"}, {"CMakeLists.txt", "c++"},
}

/*
 * Inspect looks at a folder and writes nothing.
 *
 * own is this program's own folders — the brain, its machine folder — which
 * no project may be put in or around.
 */
func Inspect(dir string, reg *engines.Registry, own ...string) Report {
	r := Report{Dir: filepath.Clean(dir)}

	if !filepath.IsAbs(dir) {
		r.Refused = "give the whole path to the folder, starting from /"

		return r
	}

	r.Refused = refused(dir, own)

	info, err := os.Stat(dir)

	switch {
	case err == nil && !info.IsDir():
		r.Exists = true
		r.Refused = orSaid(r.Refused, dir+" is a file, not a folder")

		return r
	case err == nil:
		r.Exists = true
	}

	r.Writable = writable(dir)

	if !r.Exists {
		r.Empty = true

		if !r.Writable {
			r.Risks = append(r.Risks, "the folder it would be created in cannot be written to")
		}

		return r
	}

	entries, _ := os.ReadDir(dir)
	r.Entries = len(entries)
	r.Empty = len(entries) == 0

	if !r.Writable {
		r.Risks = append(r.Risks, "this folder cannot be written to")
	}

	if adapter, project, ok := reg.Detect(dir); ok {
		r.Engine = adapter.ID()
		r.Project = &project
	}

	for _, s := range stacks {
		if _, err := os.Stat(filepath.Join(dir, s.file)); err == nil && !contains(r.Stacks, s.stack) {
			r.Stacks = append(r.Stacks, s.stack)
		}
	}

	if c, err := Load(dir); err == nil {
		r.Config = &c
	} else if err != ErrNoConfig {
		r.Risks = append(r.Risks, "its project settings could not be read: "+err.Error())
	}

	r.conventions(dir)
	r.git(dir)
	r.commands(dir)

	if !r.Empty && r.Engine == "" && len(r.Stacks) == 0 && r.Config == nil {
		r.Risks = append(r.Risks, fmt.Sprintf("it already holds %d things that are not a project — a new "+
			"project would go into a folder of its own inside it", r.Entries))
	}

	if r.Dirty > 0 {
		r.Risks = append(r.Risks, fmt.Sprintf("%d files have changes nobody has committed, and new changes "+
			"would be mixed in with them", r.Dirty))
	}

	if r.Engine != "" && r.Project != nil && r.Project.Targets != "" {
		if adapter, ok := reg.Get(r.Engine); ok {
			if engine, found := adapter.Engine(); !found && adapter.Recipe() != "" {
				r.Risks = append(r.Risks, adapter.Title()+" is not installed here, and this project needs it")
			} else if found && engine.Version != "" && majorMinor(engine.Version) != majorMinor(r.Project.Targets) {
				r.Risks = append(r.Risks, fmt.Sprintf("it was made for %s %s, and %s is installed",
					adapter.Title(), r.Project.Targets, engine.Version))
			}
		}
	}

	return r
}

func orSaid(a, b string) string {
	if a != "" {
		return a
	}

	return b
}

func contains(list []string, s string) bool {
	for _, one := range list {
		if one == s {
			return true
		}
	}

	return false
}

// writable is whether the folder, or the nearest one that exists above it,
// can be written to.
func writable(dir string) bool {
	for at := dir; ; at = filepath.Dir(at) {
		if _, err := os.Stat(at); err == nil {
			probe, err := os.CreateTemp(at, ".pn-scripts-assistant-probe-*")
			if err != nil {
				return false
			}

			probe.Close()
			os.Remove(probe.Name())

			return true
		}

		if filepath.Dir(at) == at {
			return false
		}
	}
}

func (r *Report) conventions(dir string) {
	for _, pattern := range []string{"README*", ".editorconfig", ".eslintrc*", "eslint.config.*",
		".prettierrc*", "Makefile", "tsconfig.json", ".gitignore", "CONTRIBUTING*"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))

		for _, m := range matches {
			r.Conventions = append(r.Conventions, filepath.Base(m))
		}
	}

	for _, tests := range []string{"test", "tests", "__tests__", "spec"} {
		if info, err := os.Stat(filepath.Join(dir, tests)); err == nil && info.IsDir() {
			r.Conventions = append(r.Conventions, tests+"/")
		}
	}

	sort.Strings(r.Conventions)
}

func (r *Report) git(dir string) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return
	}

	r.Git = true

	git, err := exec.LookPath("git")
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if out, err := exec.CommandContext(ctx, git, "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		r.Branch = strings.TrimSpace(string(out))
	}

	if out, err := exec.CommandContext(ctx, git, "-C", dir, "status", "--porcelain").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				r.Dirty++
			}
		}
	}
}

// commands are the project's own ways of building and testing, as found in
// its files — offered, never invented.
func (r *Report) commands(dir string) {
	found := map[string][]string{}

	if raw, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}

		if json.Unmarshal(raw, &pkg) == nil {
			if pkg.Scripts["build"] != "" {
				found["build"] = []string{"npm", "run", "build"}
			}

			if pkg.Scripts["test"] != "" {
				found["test"] = []string{"npm", "test"}
			}
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		found["build"] = []string{"go", "build", "./..."}
		found["test"] = []string{"go", "test", "./..."}
	}

	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		found["build"] = []string{"cargo", "build"}
		found["test"] = []string{"cargo", "test"}
	}

	if contains(r.Stacks, "python") {
		for _, tests := range []string{"tests", "test"} {
			if _, err := os.Stat(filepath.Join(dir, tests)); err == nil {
				found["test"] = []string{"python3", "-m", "pytest"}
			}
		}
	}

	if len(found) > 0 {
		r.Commands = found
	}
}

func majorMinor(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)

	if len(parts) < 2 {
		return version
	}

	return parts[0] + "." + strings.TrimRightFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' })
}

// Text is the report in words, for the model and for a person.
func (r Report) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Folder: %s\n", r.Dir)

	if r.Refused != "" {
		fmt.Fprintf(&b, "No project can be made or changed here: %s\n", r.Refused)

		return strings.TrimSpace(b.String())
	}

	switch {
	case !r.Exists:
		b.WriteString("It does not exist yet.\n")
	case r.Empty:
		b.WriteString("It exists and is empty.\n")
	default:
		fmt.Fprintf(&b, "It exists and holds %d things.\n", r.Entries)
	}

	if r.Project != nil {
		fmt.Fprintf(&b, "It is a %s project: %s", r.Engine, r.Project.Name)

		if r.Project.Targets != "" {
			fmt.Fprintf(&b, " (made for %s)", r.Project.Targets)
		}

		b.WriteString("\n")
	}

	if len(r.Stacks) > 0 {
		fmt.Fprintf(&b, "Built with: %s\n", strings.Join(r.Stacks, ", "))
	}

	if r.Config != nil {
		fmt.Fprintf(&b, "It has project settings: a %s project", r.Config.Kind)

		if len(r.Config.Packages) > 0 {
			fmt.Fprintf(&b, " under %s", strings.Join(r.Config.Packages, ", "))
		}

		b.WriteString("\n")
	}

	if r.Git {
		fmt.Fprintf(&b, "Git: branch %s, %d uncommitted changes\n", orSaid(r.Branch, "unknown"), r.Dirty)
	}

	if len(r.Conventions) > 0 {
		fmt.Fprintf(&b, "Conventions found: %s\n", strings.Join(r.Conventions, ", "))
	}

	for purpose, argv := range r.Commands {
		fmt.Fprintf(&b, "Its %s command: %s\n", purpose, strings.Join(argv, " "))
	}

	for _, risk := range r.Risks {
		fmt.Fprintf(&b, "Worth knowing: %s\n", risk)
	}

	return strings.TrimSpace(b.String())
}

/*
 * Lay writes a new project's files and its settings, all or nothing.
 *
 * Nothing that exists is overwritten, ever: every file is checked before the
 * first is written, and one that is already there stops the whole thing with
 * nothing changed. Each file written is recorded where "put it back" can find
 * it, so a project laid by mistake can be taken away again file by file.
 */
func Lay(dir string, files []engines.File, c Config, undoRoot string) ([]string, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("the project folder must be a whole path")
	}

	var clashes []string

	for _, f := range files {
		clean := filepath.Clean(f.Path)

		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%s is outside the project", f.Path)
		}

		if _, err := os.Stat(filepath.Join(dir, clean)); err == nil {
			clashes = append(clashes, clean)
		}
	}

	if _, err := os.Stat(Path(dir)); err == nil && len(files) > 0 {
		clashes = append(clashes, filepath.Join(Folder, ConfigFile))
	}

	if len(clashes) > 0 {
		return nil, fmt.Errorf("nothing was written: these are already there and would have been "+
			"overwritten — %s", strings.Join(clashes, ", "))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("making %s: %w", dir, err)
	}

	var written []string

	for _, f := range files {
		path := filepath.Join(dir, filepath.Clean(f.Path))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}

		undo.Keep(undoRoot, path, "created for a new project")

		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}

		if err := os.WriteFile(path, f.Content, mode); err != nil {
			return written, fmt.Errorf("writing %s: %w", f.Path, err)
		}

		written = append(written, path)
	}

	if _, err := os.Stat(Path(dir)); err != nil {
		undo.Keep(undoRoot, Path(dir), "created for a new project")

		if err := Save(dir, c); err != nil {
			return written, err
		}

		written = append(written, Path(dir))
	}

	return written, nil
}
