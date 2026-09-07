package learning

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Project is one codebase found on disk.
type Project struct {
	Path       string
	Name       string
	Stack      string
	Readme     string
	ModifiedAt string
}

// markers identify a project by a file it must contain, in priority order.
// A map would work but the order decides which stack a polyglot project is
// labelled with, and map iteration is random.
var markers = []struct {
	File  string
	Stack string
}{
	{"composer.json", "PHP/Composer"},
	{"package.json", "Node.js"},
	{"go.mod", "Go"},
	{"project.godot", "Godot"},
	{"pyproject.toml", "Python"},
	{"requirements.txt", "Python"},
	{"*.csproj", "C#/.NET"},
}

// skipDirs are never descended into.
//
// The last three are the ones that were learned the hard way: a WordPress site
// carries hundreds of third-party plugins, each with its own package.json, and
// scanning them turned 62 real projects into 75 with thirteen of somebody
// else's code presented as the owner's work.
var skipDirs = map[string]bool{
	"vendor": true, "node_modules": true, ".git": true, "storage": true,
	"bootstrap": true, ".next": true, "dist": true, "build": true,
	".idea": true, ".vscode": true, "target": true,
	"wp-content": true, "wp-includes": true, "wp-admin": true,

	/*
	 * Everything below this line came out of one measurement.
	 *
	 * Petar's review queue held 9,199 things. Grouped by the folder they came
	 * from, the top ten were all somebody else's: 3,589 from a Unity project's
	 * PackageCache and Bee artifacts, 362 from a vendored copy of HTMLPurifier,
	 * 370 from the uploads folders of two client websites, 218 from mail
	 * templates. None of it is his writing and none of it is about him, and it
	 * had crowded out whatever was.
	 *
	 * The document walk had a skip list already; it was written for PHP and
	 * JavaScript trees and had never met a game engine or a Python one.
	 */
	"packagecache": true, "bee": true, "scriptassemblies": true,
	"bower_components": true, "pods": true, "packages": true,
	"third_party": true, "thirdparty": true, "site-packages": true,
	"venv": true, ".venv": true, "__pycache__": true, ".tox": true,
	"obj": true, "out": true, "bin": true, "coverage": true,
	"logs": true, "log": true, "cache": true, ".cache": true,
	"tmp": true, "temp": true, ".gradle": true, ".m2": true,
}

/*
 * Folders whose name alone is not enough to judge them.
 *
 * "Library" is where Unity keeps a gigabyte of regenerated build cache, and it
 * is also a perfectly ordinary name for a folder of somebody's own documents.
 * The difference is what stands beside it: a Unity project always has Assets
 * next to Library, so that is what is asked rather than the name.
 *
 * "uploads" is the same shape of question. Under a website's public directory
 * it is other people's files; anywhere else it is likely to be your own.
 */
func skipBecauseOfWhereItIs(path, name string) bool {
	switch strings.ToLower(name) {
	case "library":
		if _, err := os.Stat(filepath.Join(filepath.Dir(path), "Assets")); err == nil {
			return true
		}

	case "uploads":
		if strings.EqualFold(filepath.Base(filepath.Dir(path)), "public") {
			return true
		}
	}

	return false
}

// worthDescending is the whole rule for a directory, by name and by place.
func worthDescending(path, name string) bool {
	return !skipDirs[strings.ToLower(name)] &&
		!strings.HasPrefix(name, ".") &&
		!skipBecauseOfWhereItIs(path, name)
}

// skipNames match dated snapshots rather than a distinct current project.
var skipNames = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^backup[_-]`),
	regexp.MustCompile(`(?i)^\.backup`),
}

// MaxScanDepth bounds the walk. Deep trees are almost always something the
// skip list should have caught, and an unbounded walk of a home directory is a
// way to spend an afternoon reading node_modules.
const MaxScanDepth = 6

// ReadmeExcerpt is how much of a README is kept.
//
// Enough to say what a project is, little enough that a hundred of them do not
// overwhelm the store or the prompts built from it.
const ReadmeExcerpt = 400

// ScanProjects walks a tree and describes the projects in it.
//
// Read-only, always. This looks at somebody's entire working life, and the one
// guarantee worth making about that is that looking changes nothing. Descent
// stops as soon as a marker is found, so a project's own dependencies are never
// treated as projects of their own.
func ScanProjects(root string) ([]Project, error) {
	var found []Project

	if err := walkProjects(root, 0, &found); err != nil {
		return nil, err
	}

	return found, nil
}

func walkProjects(dir string, depth int, found *[]Project) error {
	if depth > MaxScanDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory is normal on a real machine and is not a
		// reason to abandon the scan.
		return nil
	}

	names := map[string]bool{}

	for _, e := range entries {
		names[e.Name()] = true
	}

	for _, m := range markers {
		hit := names[m.File]

		if strings.Contains(m.File, "*") {
			matches, _ := filepath.Glob(filepath.Join(dir, m.File))
			hit = len(matches) > 0
		}

		if hit {
			*found = append(*found, describe(dir, m.Stack))

			return nil
		}
	}

	for _, e := range entries {
		if !e.IsDir() || !worthDescending(filepath.Join(dir, e.Name()), e.Name()) {
			continue
		}

		if matchesAny(e.Name(), skipNames) {
			continue
		}

		// A symlink can point back up the tree; following one is how a scan
		// becomes infinite.
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}

		walkProjects(filepath.Join(dir, e.Name()), depth+1, found)
	}

	return nil
}

func describe(dir, stack string) Project {
	p := Project{Path: dir, Name: filepath.Base(dir), Stack: stack}

	for _, name := range []string{"README-AI.md", "README.md", "readme.md"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}

		p.Readme = truncateRunes(string(raw), ReadmeExcerpt)

		break
	}

	if info, err := os.Stat(dir); err == nil {
		p.ModifiedAt = info.ModTime().Format("2006-01-02")
	}

	return p
}

// Sentence renders a project as the fact the brain will remember.
func (p Project) Sentence(owner string) string {
	if owner == "" {
		owner = "The owner"
	}

	s := fmt.Sprintf("%s has a %s project called %q at %s", owner, p.Stack, p.Name, p.Path)

	if p.ModifiedAt != "" {
		s += ", last modified " + p.ModifiedAt
	}

	s += "."

	if p.Readme != "" {
		s += " README excerpt: " + p.Readme
	}

	return s
}

// Source is how the Validator recognises a claim it can check against disk.
func (p Project) Source() string { return "project:" + p.Path }

// Document is one file worth remembering.
type Document struct {
	Path       string
	Name       string
	Kind       string
	ModifiedAt string
}

// documentKinds are the extensions worth recording.
//
// Text is deliberately excluded from extraction here: reading the contents of
// every document would be a far larger promise than noting that they exist, and
// the formats people actually keep notes in need parsers this does not have.
var documentKinds = map[string]string{
	".pdf": "PDF", ".doc": "Word", ".docx": "Word", ".dotx": "Word template",
	".odt": "OpenDocument", ".ods": "spreadsheet", ".xlsx": "spreadsheet",
	".ppt": "presentation", ".pptx": "presentation", ".md": "Markdown", ".txt": "text",
}

// ScanDocuments lists the documents under a directory.
func ScanDocuments(root string) ([]Document, error) {
	var found []Document

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path != root && !worthDescending(path, d.Name()) {
				return filepath.SkipDir
			}

			return nil
		}

		kind, ok := documentKinds[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return nil
		}

		doc := Document{Path: path, Name: d.Name(), Kind: kind}

		if info, err := d.Info(); err == nil {
			doc.ModifiedAt = info.ModTime().Format("2006-01-02")
		}

		found = append(found, doc)

		return nil
	})

	return found, err
}

func (d Document) Sentence(owner string) string {
	if owner == "" {
		owner = "The owner"
	}

	s := fmt.Sprintf("%s has a %s document called %q at %s", owner, d.Kind, d.Name, d.Path)

	if d.ModifiedAt != "" {
		s += ", last modified " + d.ModifiedAt
	}

	return s + "."
}

func (d Document) Source() string { return "document:" + d.Path }

func matchesAny(s string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}

	return false
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

// Observation is a scanned fact and the claim that can be rechecked.
type Observation struct {
	Content string
	Source  string
}

// Observations turns a scan into the claims the pipeline accepts.
type Observations []Observation

// FromProjects renders scanned projects as observations.
func FromProjects(projects []Project, owner string) Observations {
	out := make(Observations, 0, len(projects))

	for _, p := range projects {
		out = append(out, Observation{Content: p.Sentence(owner), Source: p.Source()})
	}

	return out
}

// FromDocuments renders scanned documents as observations.
func FromDocuments(docs []Document, owner string) Observations {
	out := make(Observations, 0, len(docs))

	for _, d := range docs {
		out = append(out, Observation{Content: d.Sentence(owner), Source: d.Source()})
	}

	// And a mark against each one whose insides can be read, so the reading is
	// tracked separately from the listing. See ReadingSource.
	out = append(out, ContentsWanted(docs)...)

	return out
}
