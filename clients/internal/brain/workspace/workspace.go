/*
 * Package workspace is the folder a piece of work happens in, and the rules
 * that come with it.
 *
 * The file tools could always write anywhere their owner approved. That is the
 * right rule for a conversation — its owner is reading every approval — and
 * the wrong one for an agent building a game for half an hour: a build that
 * writes next to the project instead of into it, or a command run one folder
 * up, is an approval somebody gave for something else. So a project says, in a
 * file that lives in it and travels with it, which folders are its own and
 * where its builds go, and work on the project is held to exactly that.
 *
 * The file is .pn-assistant/project.json, written for people as much as for
 * the program: it can be read, edited and committed with the project. What
 * the program knows about a project — its engine, its commands, what finished
 * means, what was learned working on it — is kept there, apart from what the
 * assistant knows about its owner, which never goes into somebody's project.
 */
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Folder is the project's own settings folder, and ConfigFile the settings.
const (
	Folder     = ".pn-assistant"
	ConfigFile = "project.json"
	MemoryFile = "memory.md"

	// Evidence is where pictures and reports of this project's checks go,
	// inside the settings folder so they never mix with the project's files.
	Evidence = ".pn-assistant/evidence"
)

// Version is the settings format, for the day it changes.
const Version = 1

// Config is a project's own settings.
type Config struct {
	Version int    `json:"version"`
	Name    string `json:"name"`

	// Kind is what the project is: game, web, api, book, plan or other.
	Kind string `json:"kind"`

	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`

	Packages []string `json:"packages,omitempty"`

	/*
	 * Roots are the folders work on this project may write in, relative to
	 * the project; "." is the project itself. Outputs are where builds and
	 * exports may go. Both are checked on every write and every command.
	 */
	Roots   []string `json:"roots"`
	Outputs []string `json:"outputs"`

	// Commands are the project's approved commands, by what they are for:
	// build, test, run. Exactly the argv, never a shell line.
	Commands map[string][]string `json:"commands,omitempty"`

	Conventions []string `json:"conventions,omitempty"`

	// Acceptance is what finished means for this project, a line each.
	Acceptance []string `json:"acceptance,omitempty"`

	// Integrations are the MCP servers work on this project may use.
	Integrations []string `json:"integrations,omitempty"`

	Agents []string `json:"agents,omitempty"`

	/*
	 * Resources is how this project's work may be done — local only, which
	 * services it may and may not use, paths that never leave this machine —
	 * as the orchestrator reads it. It only ever narrows its owner's policy.
	 */
	Resources json.RawMessage `json:"resources,omitempty"`

	Created string `json:"created,omitempty"`
}

// ErrNoConfig is a folder with no .pn-assistant settings in it.
var ErrNoConfig = errors.New("this folder has no project settings")

// Path is where a project's settings live.
func Path(dir string) string { return filepath.Join(dir, Folder, ConfigFile) }

// Load reads a project's settings.
func Load(dir string) (Config, error) {
	raw, err := os.ReadFile(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, ErrNoConfig
	}

	if err != nil {
		return Config{}, err
	}

	var c Config

	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s could not be read: %w", Path(dir), err)
	}

	if err := c.Check(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", Path(dir), err)
	}

	return c, nil
}

/*
 * Check refuses settings that would widen what work here may touch.
 *
 * A root or an output that climbs out of the project, or is absolute, is not
 * a setting for this project — it is a way for a file in somebody's project to
 * say "and also my home folder". Refused whole.
 */
func (c Config) Check() error {
	for _, list := range [][]string{c.Roots, c.Outputs} {
		for _, rel := range list {
			if filepath.IsAbs(rel) {
				return fmt.Errorf("%q is outside the project: folders here are relative to it", rel)
			}

			clean := filepath.Clean(rel)

			if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%q climbs out of the project", rel)
			}
		}
	}

	for purpose, argv := range c.Commands {
		if len(argv) == 0 {
			return fmt.Errorf("the %s command is empty", purpose)
		}
	}

	return nil
}

/*
 * Save writes a project's settings, and a short note beside them saying what
 * the folder is — somebody finding .pn-assistant in their repository should
 * not have to guess.
 */
func Save(dir string, c Config) error {
	if err := c.Check(); err != nil {
		return err
	}

	if c.Version == 0 {
		c.Version = Version
	}

	if c.Created == "" {
		c.Created = time.Now().UTC().Format(time.RFC3339)
	}

	if len(c.Roots) == 0 {
		c.Roots = []string{"."}
	}

	sort.Strings(c.Outputs)

	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	folder := filepath.Join(dir, Folder)

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}

	// Pictures of checks are worth keeping on the machine and not in the
	// repository; a .gitignore beside them says so to git.
	ignore := filepath.Join(folder, ".gitignore")

	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(ignore, []byte("evidence/\n"), 0o644)
	}

	readme := filepath.Join(folder, "README.md")

	if _, err := os.Stat(readme); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(readme, []byte(note), 0o644)
	}

	path := Path(dir)

	if err := os.WriteFile(path+".new", append(raw, '\n'), 0o644); err != nil {
		return err
	}

	return os.Rename(path+".new", path)
}

const note = `# .pn-assistant

This project's settings for PN Scripts Assistant.

- project.json — which folders work on this project may write in, where builds
  go, its approved commands, and what finished means for it. Edit it by hand if
  you like; the assistant reads it before every step.
- memory.md — what was learned working on this project, a line each. It is
  this project's, and never mixed with anything the assistant knows about you.
- skills/ — this project's own know-how, a Markdown file a piece. Every step
  of work on the project is told it.
- evidence/ — pictures and reports from checks and builds. Safe to delete, and
  usually not worth committing.
`

// Scope is where work on a project may write, as absolute paths.
type Scope struct {
	Dir     string   `json:"dir"`
	Roots   []string `json:"roots"`
	Outputs []string `json:"outputs"`
}

// ScopeOf is a project's settings as folders on this machine.
func ScopeOf(dir string, c Config) Scope {
	dir = filepath.Clean(dir)

	s := Scope{Dir: dir}

	roots := c.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}

	for _, rel := range roots {
		s.Roots = append(s.Roots, filepath.Join(dir, rel))
	}

	for _, rel := range c.Outputs {
		s.Outputs = append(s.Outputs, filepath.Join(dir, rel))
	}

	s.Outputs = append(s.Outputs, filepath.Join(dir, Evidence))

	return s
}

/*
 * Allows is whether a path is inside the project's roots.
 *
 * Judged on where the path really is, not how it is spelled: "..", a symbolic
 * link out of the project and a relative path are all resolved first, so a
 * link called assets that points at the home folder is the home folder.
 */
func (s Scope) Allows(path string) bool {
	resolved := real(path)

	return within(resolved, s.realRoots(s.Roots)) && !Protected(resolved)
}

// Writable is the project's own folders, as they really are: what the kernel
// is told work on it may write in.
func (s Scope) Writable() []string {
	return s.realRoots(append(append([]string{}, s.Roots...), s.Outputs...))
}

/*
 * Protected is a path in a project's settings folder that its own work may
 * not change: project.json — which folders are the project's, its commands,
 * its integrations, what finished means — and the files the program keeps
 * there. An agent that could edit project.json could add an integration to
 * the list that is meant to be the second key, or rewrite the test it is
 * judged by. Its evidence/ and skills/ folders stay open: pictures of checks,
 * and what was learned about the project.
 *
 * Any project's, not only this one's: a project inside another is still a
 * project, and its settings are its own.
 */
func Protected(path string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(filepath.ToSlash(clean), "/")

	for i, part := range parts {
		if part != Folder {
			continue
		}

		if i == len(parts)-1 {
			return true
		}

		switch parts[i+1] {
		case "evidence", "skills":
			continue
		}

		return true
	}

	return false
}

// Output is whether a path is inside one of the project's output folders.
func (s Scope) Output(path string) bool {
	return within(real(path), s.realRoots(s.Outputs))
}

func (s Scope) realRoots(list []string) []string {
	out := make([]string, 0, len(list))

	for _, r := range list {
		out = append(out, real(r))
	}

	return out
}

func within(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

/*
 * real is where a path actually is: made absolute, cleaned, and with every
 * link in the part of it that exists followed. The part that does not exist
 * yet — a file about to be written — is added back as written.
 */
func real(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	rest := ""
	at := abs

	for {
		if resolved, err := filepath.EvalSymlinks(at); err == nil {
			return filepath.Join(resolved, rest)
		}

		parent := filepath.Dir(at)
		if parent == at {
			return abs
		}

		rest = filepath.Join(filepath.Base(at), rest)
		at = parent
	}
}

// Remember adds a line to what was learned working on this project.
func Remember(dir, line string) error {
	line = strings.TrimSpace(strings.ReplaceAll(line, "\n", " "))
	if line == "" {
		return nil
	}

	folder := filepath.Join(dir, Folder)

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(folder, MemoryFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = fmt.Fprintf(f, "- %s — %s\n", time.Now().Format("2006-01-02"), line)

	return err
}

/*
 * Skills is the project's own know-how: .pn-assistant/skills/*.md, a file a
 * piece, written by whoever works on it — "the level files are generated, edit
 * the generator". Read in name order, each cut short, a few at most: they go
 * into every step's brief, and a brief that is mostly skills is a brief the
 * step itself gets lost in.
 */
func Skills(dir string, most int) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, Folder, "skills", "*.md"))

	sort.Strings(matches)

	var out []string

	for _, path := range matches {
		if len(out) == most {
			break
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		text := strings.TrimSpace(string(raw))
		if len(text) > 1500 {
			text = text[:1500] + "…"
		}

		if text != "" {
			out = append(out, strings.TrimSuffix(filepath.Base(path), ".md")+": "+text)
		}
	}

	return out
}

// Memory is the last lines learned working on this project, oldest first.
func Memory(dir string, most int) []string {
	raw, err := os.ReadFile(filepath.Join(dir, Folder, MemoryFile))
	if err != nil {
		return nil
	}

	var lines []string

	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "- ") {
			lines = append(lines, strings.TrimPrefix(line, "- "))
		}
	}

	if len(lines) > most {
		lines = lines[len(lines)-most:]
	}

	return lines
}

/*
 * Widened is how now lets work do more than before did, in words, or empty
 * when it does not: a folder, an output, an integration or a command that
 * was not there, or a command that now runs something else.
 */
func Widened(before, now Config) string {
	var more []string

	added := func(what string, old, current []string) {
		have := map[string]bool{}

		for _, v := range old {
			have[filepath.Clean(v)] = true
		}

		for _, v := range current {
			if !have[filepath.Clean(v)] {
				more = append(more, what+" "+v)
			}
		}
	}

	roots := before.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}

	nowRoots := now.Roots
	if len(nowRoots) == 0 {
		nowRoots = []string{"."}
	}

	added("folder", roots, nowRoots)
	added("output", before.Outputs, now.Outputs)
	added("integration", before.Integrations, now.Integrations)

	if strings.TrimSpace(string(now.Resources)) != strings.TrimSpace(string(before.Resources)) &&
		len(now.Resources) > 0 {
		more = append(more, "how its work may be done")
	}

	for purpose, argv := range now.Commands {
		if old, ok := before.Commands[purpose]; !ok || strings.Join(old, "\x00") != strings.Join(argv, "\x00") {
			more = append(more, "the "+purpose+" command")
		}
	}

	sort.Strings(more)

	return strings.Join(more, ", ")
}
