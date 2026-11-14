package bootstrap

import (
	"strings"
	"testing"
	"unicode"

	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/workspace"
)

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}

	return false
}

/*
 * The project proposal on a Bulgarian approval card: every heading and
 * fixed sentence in Bulgarian, every value as it is; and the English text
 * is the one it always was.
 */
func TestAProjectProposalIsSaidInBulgarianToo(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		p := Proposal{
			Name: "zz-game", Dir: "/zz/dir", New: fresh,
			Report:  workspace.Report{Exists: !fresh, Entries: 3, Git: true, Branch: "main", Dirty: 2},
			Package: &team.PackageView{ID: "zz.pkg", Version: "1.0.0", Title: "ZZ Package", Work: "game"},
			Engine:  "godot", EngineVersion: "4.7.1",
			Files: []string{"zz/file.txt"},
			Hire:  &team.Proposal{Agent: team.Agent{Name: "zz-dev", Title: "ZZ Developer"}, Tools: []string{"write_file"}},
			Requires: []team.Needed{
				{Why: "zz why", Status: provision.Status{Title: "ZZ Tool", Installable: true, Source: "zz source", Licence: "MIT", Cost: "free"}},
				{Why: "zz why", Optional: true, Status: provision.Status{Title: "ZZ Optional"}},
				{Why: "zz why", Status: provision.Status{Title: "ZZ Manual", Manual: "zz manual"}},
			},
			Chosen:  []string{"zz chosen"},
			Stack:   []string{"zz stack"},
			Minutes: 30, MinutesWhy: "zz minutes",
			Steps:        []store.TaskStep{{Instruction: "zz step"}},
			Done:         []string{"zz done"},
			Limits:       []string{"zz limit"},
			Professional: []string{"zz professional"},
			Blockers:     []string{"zz blocker"},
		}
		p.Config.Outputs = []string{"zz/build"}

		if p.TextIn("en") != p.Text() {
			t.Fatalf("new %v: the English text changed", fresh)
		}

		bg := p.TextIn("bg")

		for _, line := range strings.Split(bg, "\n") {
			if line == "" {
				continue
			}

			// Listed values are data, except the fixed words said around them.
			if strings.HasPrefix(line, "  - ") && !strings.Contains(line, "ZZ Tool") && !strings.Contains(line, "ZZ Optional") &&
				!strings.Contains(line, "ZZ Manual") && !strings.Contains(line, "ZZ Developer") && !strings.Contains(line, "/zz/dir") &&
				!strings.Contains(line, "zz/build") && !strings.Contains(line, "main") && !strings.Contains(line, "write_file") {
				continue
			}

			if !hasCyrillic(line) {
				t.Errorf("new %v: a line in English on the Bulgarian card: %q", fresh, line)
			}
		}

		if fresh && !strings.Contains(bg, "zz-game") {
			t.Errorf("the new project's name is not on the Bulgarian card")
		}

		for _, item := range []string{"zz chosen", "zz step", "/zz/dir", "zz blocker", "4.7.1"} {
			if !strings.Contains(bg, item) {
				t.Errorf("new %v: %q is not on the Bulgarian card", fresh, item)
			}
		}
	}
}
