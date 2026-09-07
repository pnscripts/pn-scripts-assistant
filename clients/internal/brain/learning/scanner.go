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

/*
 * What marks a directory as a distributable package.
 *
 * A name list can only ever know the folder names somebody thought of, and the
 * two that got through here were called "tools/htmlpurifier" and
 * "public/js/summernote" — nothing a list would have guessed. But both carried
 * the thing every distributed package carries, in every language and every
 * decade: a manifest saying what it is, or a licence saying who owns it.
 *
 * That is the signal worth using, because it is not a convention this program
 * has noticed — it is the definition. A folder with a licence in it belongs to
 * whoever wrote the licence.
 */
var packageMarks = map[string]bool{
	// What it is, in every ecosystem that has a word for it.
	"composer.json": true, "package.json": true, "go.mod": true,
	"pyproject.toml": true, "setup.py": true, "requirements.txt": true,
	"cargo.toml": true, "gemfile": true, "pom.xml": true,
	"build.gradle": true, "bower.json": true, "podspec": true,
	"pubspec.yaml": true, "project.godot": true,

	// And what it depends on, which is the same statement written down twice.
	// A lock file is the surest of all of these: nothing writes one but a
	// package manager, and it writes one only for a package.
	"composer.lock": true, "package-lock.json": true, "yarn.lock": true,
	"gemfile.lock": true, "cargo.lock": true, "poetry.lock": true,
	"pnpm-lock.yaml": true, "go.sum": true,

	// Who owns it. Plurals included because they are just as common on disk
	// and a rule that misses "LICENSES" misses the whole tree beneath it —
	// which is exactly how a vendored copy of HTMLPurifier got through.
	"license": true, "license.txt": true, "license.md": true,
	"licenses": true, "licenses.txt": true,
	"licence": true, "licence.txt": true, "licence.md": true,
	"licences": true, "licences.txt": true,
	"copying": true, "copying.txt": true, "copyright": true,
	"notice": true, "notice.txt": true,
	"license-mit": true, "license-apache": true,
}

// carriesAPackageMark reports whether a directory declares itself a package.
func carriesAPackageMark(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		if packageMarks[strings.ToLower(e.Name())] {
			return true
		}
	}

	return false
}

/*
 * IsSomebodyElsesWriting reports whether a file is boilerplate that came with
 * software rather than something a person wrote.
 *
 * A licence is the clearest case there is: it is the same text in millions of
 * copies, it says nothing about the person whose disk it is on, and it is
 * legally required to be there. 176 of Petar's review queue were files named
 * "summernote-ar-AR.min.js.LICENSE.txt" — one per language of a text editor he
 * did not write, in four copies of the same site.
 */
func IsSomebodyElsesWriting(name string) bool {
	lower := strings.ToLower(name)

	if packageMarks[lower] {
		return true
	}

	/*
	 * Only the licence words, and only these, as part of a longer name.
	 *
	 * "summernote-ar-AR.min.js.LICENSE.txt" has to be caught, so the check
	 * cannot be on the whole name — but every word added here is a word
	 * somebody might have put in a file of their own. "notice" and "changelog"
	 * were on this list for an hour and caught "EmailNotice.pdf" in Petar's own
	 * documents. They are still recognised as a whole filename, which is how
	 * they appear when they are boilerplate.
	 */
	for _, mark := range []string{"license", "licence"} {
		if strings.Contains(lower, mark) {
			return true
		}
	}

	return false
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

	/*
	 * Films and recordings, which are listed and never opened.
	 *
	 * "What films do I have" is a question about its owner and the brain could
	 * not answer it, because a drive of 866 films was invisible to a scanner
	 * that only knew about paperwork. The name and the path are the whole of
	 * what is worth keeping: nothing here can watch a film, and the subtitles
	 * beside it are a story rather than anything about him. See CanRead, which
	 * is what stops these being opened, and subtitles.go for the rest.
	 */
	".mkv": "film", ".mp4": "film", ".avi": "film", ".mov": "film",
	".m4v": "film", ".webm": "film", ".wmv": "film", ".mpg": "film",
	".mpeg": "film",
}

/*
 * IsAFilm reports whether a path is something to watch rather than to read.
 *
 * The rule stated once, where the list of extensions is, so the two cannot
 * disagree. Nothing in this package needs to ask — CanRead already refuses
 * video, which is what keeps a film from being opened — and the question is
 * worth being able to ask by name rather than by comparing a map lookup to a
 * string at each call site.
 */
func IsAFilm(path string) bool {
	return documentKinds[strings.ToLower(filepath.Ext(path))] == "film"
}

/*
 * ScanDocuments lists the documents under a directory.
 *
 * Two things it will not do, and both are about whose writing a file is.
 *
 * A package inside a package is a dependency. The walk keeps track of whether
 * anything above it declared itself a package — a manifest, a licence — and a
 * directory that declares itself one again, inside that, is a copy of somebody
 * else's work vendored into somebody's project. That is how every package
 * manager on earth arranges things, so it holds without knowing the name of a
 * single tool: "tools/htmlpurifier" and "public/js/summernote" are caught by
 * the same rule as node_modules, without either name appearing anywhere.
 *
 * And a licence is not writing. See IsSomebodyElsesWriting.
 */
func ScanDocuments(root string) ([]Document, error) {
	var found []Document

	/*
	 * Which directories sit inside something that called itself a package.
	 *
	 * The root is never one of them, whatever is lying in it. It is where the
	 * person pointed — "read my projects" — so the outermost package below it
	 * is theirs, and only what is nested inside that is a dependency.
	 *
	 * Seeding it as a package instead was one stray file away from disaster:
	 * somebody had run composer once in their home folder, so ~/composer.lock
	 * existed, and every project on the machine became a package inside a
	 * package. 242 of Petar's 573 memories were his own Laravel projects'
	 * README and documentation, disowned by a lock file he wrote by accident
	 * years ago.
	 *
	 * WalkDir visits a parent before its children, so this is always filled in
	 * by the time it is asked for.
	 */
	inside := map[string]bool{root: false}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path == root {
				return nil
			}

			if !worthDescending(path, d.Name()) {
				return filepath.SkipDir
			}

			under := inside[filepath.Dir(path)]
			mine := carriesAPackageMark(path)

			// A package declared inside a package is a dependency, and its
			// documentation is its author's rather than this machine's owner's.
			if under && mine {
				return filepath.SkipDir
			}

			inside[path] = under || mine

			return nil
		}

		if IsSomebodyElsesWriting(d.Name()) {
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

/*
 * UnderASkippedFolder reports whether a path lies inside a folder the walk
 * would now refuse to enter.
 *
 * For undoing what an earlier, looser rule let in. Petar's memory held 1,220
 * facts and 746 of them were listings of files inside a Unity package cache, a
 * vendored PHP library and two client sites' upload folders — remembered
 * before those folders were on the skip list, and no less useless for having
 * arrived first.
 *
 * Asked of the same functions the walk uses rather than of a list written out
 * again here. A second list is a second thing to keep up to date, and the one
 * that is not being read by the scanner every day is the one that goes stale.
 */
func UnderASkippedFolder(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}

	// A licence is not writing, wherever it sits.
	if IsSomebodyElsesWriting(filepath.Base(path)) {
		return true
	}

	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))

	// The last part is the file itself; only the directories above it decide.
	at := string(filepath.Separator)

	var insideAPackage bool

	for _, name := range parts[1 : len(parts)-1] {
		at = filepath.Join(at, name)

		/*
		 * Nothing above the home folder is judged at all.
		 *
		 * Not for being a package — a home directory is not one, whatever
		 * somebody once ran in it, and treating one as a package makes every
		 * project on the machine a dependency — and not by name either, since
		 * the folders on the way to somebody's home are not their doing.
		 */
		if aPlaceSomebodyLives(at) {
			continue
		}

		if !worthDescending(at, name) {
			return true
		}

		/*
		 * The same nesting rule the walk uses, asked of a path.
		 *
		 * A package declared inside a package is a dependency. Asked here as
		 * well so that "forget what the rules would no longer read" means the
		 * current rules in full, rather than the half of them that happens to
		 * be about folder names.
		 */
		if carriesAPackageMark(at) {
			if insideAPackage {
				return true
			}

			insideAPackage = true
		}
	}

	return false
}

/*
 * aPlaceSomebodyLives reports whether a directory is a home folder or above one.
 *
 * These are never package boundaries. A stray composer.lock in a home folder —
 * from running the thing once, years ago, in the wrong directory — is the whole
 * distance between "read my documents" and "read nothing at all".
 */
func aPlaceSomebodyLives(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir == "/"
	}

	return dir == home || strings.HasPrefix(home, dir+string(filepath.Separator)) || dir == "/"
}

/*
 * PathIn pulls the file out of a sentence the brain wrote about it.
 *
 * Both forms end the path the same way, with a comma or a full stop:
 *
 *   Petar has a text document called "x.txt" at /path/x.txt, last modified …
 *   x.pdf, a PDF document at /path/x.pdf, says: …
 *
 * Returns empty when there is no path in it, which is most facts — a sentence
 * about somebody's preferences is not about a file at all.
 */
func PathIn(sentence string) string {
	i := strings.Index(sentence, " at "+string(filepath.Separator))
	if i < 0 {
		return ""
	}

	rest := sentence[i+len(" at "):]

	for _, end := range []string{", last modified", ", says:", " and there is nothing in it"} {
		if j := strings.Index(rest, end); j >= 0 {
			rest = rest[:j]
		}
	}

	return strings.TrimRight(strings.TrimSpace(rest), ".,")
}
