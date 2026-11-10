// Package programname holds one test: nothing a person reads tells them to
// type "brain …" or calls the program "the brain" or "AI Brain".
//
// The program was PN Brain and is PN Scripts Assistant; its command is
// pn-scripts-assistant (and pn-scripts-assistant-cli, pn-scripts-assistant-doctor).
// "The brain" stays the name of the knowledge it keeps, in one folder, which is
// a different thing: "move the brain to another drive" is right, "brain move"
// is a command that does not exist (owner, 10.11.2026).
package programname

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// subcommands are the program's commands, as main.go dispatches them, and the
// two companion programs' names.
const subcommands = `app|serve|rewrite-paths|ingest|mic-test|drives|move|tidy|start-over|promote|copies|places|status|menu|setup|start-again|unlock|read-log|window-check|update|version|help|cli|doctor`

// asCommand is "brain <command>": the old name typed as if it were a program.
// The word before it is captured to tell "the brain status" from a command.
var asCommand = regexp.MustCompile(`(?i)(?:(\S+)\s+)?\S*?\bbrain(-cli|-doctor)? (` + subcommands + `)\b`)

// determiners make "the brain status" a noun phrase, not a command.
var determiners = map[string]bool{
	"the": true, "a": true, "this": true, "that": true, "your": true, "its": true,
	"whole": true, "own": true, "new": true, "empty": true, "old": true, "one": true,
	"each": true, "every": true, "which": true, "another": true,
}

// asName is the program called a brain: by its old product name, or as the
// thing that is or is not running.
var asName = regexp.MustCompile(`(?i)\bAI Brain\b|\bbrain (is|isn't|is not|was not|wasn't) running\b|\bis the brain running\b|\bstart(ing)? the brain\b|\bbuilding the brain\b`)

// repoRoot is the repository, two levels above clients/internal/programname.
func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	return filepath.Join(wd, "..", "..", "..")
}

// skipped are what is history or not read by a user of the program: the
// changelog keeps released versions as they were said, audits and decisions
// are dated records, and AGENTS.md/CLAUDE.md name the workspace's AI Brain,
// which is not this program.
func skipped(rel string) bool {
	switch {
	case rel == "CHANGELOG.md", rel == "AGENTS.md", rel == "CLAUDE.md",
		strings.HasPrefix(rel, "docs/audit/"), strings.HasPrefix(rel, "docs/decisions/"),
		strings.HasPrefix(rel, "docs/RELEASE-NOTES-"),
		strings.HasPrefix(rel, ".git/"), strings.HasPrefix(rel, ".claude/"),
		strings.HasPrefix(rel, "build/"), strings.HasPrefix(rel, "dist/"),
		strings.HasPrefix(rel, "clients/dist/"),
		strings.Contains(rel, "node_modules/"),
		strings.HasSuffix(rel, "_test.go"):
		return true
	}

	return false
}

// readable are the kinds of file whose text reaches a person: pages and their
// scripts, guides, the manual, packaging and the scripts that print to a
// terminal. Go is read separately, by its string literals.
var readable = map[string]bool{
	".js": true, ".html": true, ".md": true, ".1": true, ".sh": true,
	".iss": true, ".json": true, ".desktop": true, ".service": true, ".txt": true,
}

type finding struct{ where, text string }

func check(where, text string, out *[]finding) {
	for _, m := range asCommand.FindAllStringSubmatch(text, -1) {
		// "pn-brain setup" is the old program too; "the brain setup" is not a
		// command.
		before := strings.ToLower(m[1])
		if determiners[before] {
			continue
		}

		*out = append(*out, finding{where, m[0]})
	}

	for _, m := range asName.FindAllString(text, -1) {
		*out = append(*out, finding{where, m})
	}
}

// goStrings is every string literal in a Go file, which is everything in it a
// person can read.
func goStrings(t *testing.T, path string) []string {
	t.Helper()

	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	var out []string

	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		if s, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, s)
		}

		return true
	})

	return out
}

func TestNothingAPersonReadsCallsTheProgramBrain(t *testing.T) {
	root := repoRoot(t)

	var found []finding

	checked := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel != "." && skipped(rel+"/") {
				return filepath.SkipDir
			}

			return nil
		}

		if skipped(rel) {
			return nil
		}

		ext := filepath.Ext(path)

		switch {
		case ext == ".go":
			checked++

			for _, s := range goStrings(t, path) {
				check(rel, s, &found)
			}
		case readable[ext]:
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			checked++
			check(rel, string(b), &found)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A walk that read nothing proves nothing.
	if checked < 200 {
		t.Fatalf("only %d files were read; is the repository where this test thinks it is (%s)?", checked, root)
	}

	for _, f := range found {
		t.Errorf("%s: %q. The program is PN Scripts Assistant and its command is pn-scripts-assistant", f.where, f.text)
	}
}

/*
 * The terminal client and the doctor do not say "brain" at all.
 *
 * Neither keeps any knowledge, so for them the word can only ever mean the
 * program. The client's old address variable and the old folder name are read
 * for compatibility and are not words a person is shown: BRAIN_API_URL,
 * /api/brain and pn-brain do not count.
 */
func TestTheCompanionProgramsNeverSayBrain(t *testing.T) {
	root := repoRoot(t)
	word := regexp.MustCompile(`(?i)(^|[^a-z_/-])brain([^a-z_]|$)`)

	for _, dir := range []string{"clients/cmd/cli", "clients/cmd/doctor"} {
		files, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go files in %s", dir)
		}

		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}

			for _, s := range goStrings(t, path) {
				if word.MatchString(s) {
					t.Errorf("%s says %q", filepath.Base(dir)+"/"+filepath.Base(path), s)
				}
			}
		}
	}
}

// The check itself: it must catch what was there, and leave the knowledge
// called the brain alone.
func TestTheCheckTellsACommandFromTheKnowledge(t *testing.T) {
	flagged := []string{
		"    brain start-again\n",
		"from System or \"brain setup\".",
		"`brain rewrite-paths` repairs them",
		"Connected, but nothing answered. Is the brain running?",
		"AI Brain API base URL",
		"In the meantime the brain is running: open %s",
		"pn-brain status",
	}

	for _, s := range flagged {
		var f []finding
		if check("x", s, &f); len(f) == 0 {
			t.Errorf("not caught: %q", s)
		}
	}

	fine := []string{
		"move the brain to another drive, verifying every byte",
		"where copies of the brain are kept",
		"Everything the brain knows lives in one folder you choose",
		"pn-scripts-assistant start-again",
		"the brain status colour",
		"keep the brain in your account instead",
	}

	for _, s := range fine {
		var f []finding
		if check("x", s, &f); len(f) != 0 {
			t.Errorf("caught by mistake: %q (%v)", s, f)
		}
	}
}
