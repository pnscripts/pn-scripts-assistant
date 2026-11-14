package team

import (
	"strings"
	"testing"
	"unicode"

	"pn-scripts-assistant/internal/brain/provision"
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
 * The hire proposal on a Bulgarian approval card: every heading and fixed
 * sentence in Bulgarian, every value (titles, names, the model's lines) as
 * it is; and the English text is the one it always was.
 */
func TestAHireProposalIsSaidInBulgarianToo(t *testing.T) {
	for _, permanence := range []string{Permanent, TaskOnly, ""} {
		p := Proposal{
			Permanence: permanence, Goal: "zz goal",
			Agent: Agent{Name: "zz-dev", Title: "ZZ Developer", Brief: "zz brief"},
			Job:   JobView{ID: "zz-job", Title: "ZZ Job"},
			Seat:  "zz-seat", SeatIn: "zz-unit",
			Packages:         []PackageView{{ID: "zz.pkg", Version: "1.0.0", Title: "ZZ Package", Work: "zz", Needs: []string{"zz need"}}},
			Responsibilities: []string{"zz responsibility"},
			Limits:           []string{"zz limit"},
			Model:            ModelView{Uses: "big", Name: "zz-model", Provider: "ollama"},
			Skills:           []string{"zz skill"},
			Requires: []Needed{
				{Why: "zz why", Status: provision.Status{Title: "ZZ Tool"}},
				{Why: "zz why", Optional: true, Status: provision.Status{Title: "ZZ Other", Present: true, Problem: "zz problem"}},
			},
			Integrations: []WantedIntegration{{ID: "zz-int", Why: "zz why"}},
			Itself:       []string{"zz itself"},
			You:          []string{"zz you"},
			Professional: []string{"zz professional"},
			Risk:         "high",
		}

		if p.TextIn("en") != p.Text() {
			t.Fatalf("%q: the English text changed", permanence)
		}

		bg := p.TextIn("bg")

		for _, line := range strings.Split(bg, "\n") {
			if line == "" || strings.HasPrefix(line, "  - ") {
				continue
			}

			if !hasCyrillic(line) {
				t.Errorf("%q: a line in English on the Bulgarian card: %q", permanence, line)
			}
		}

		if !strings.Contains(bg, "Риск: висок") {
			t.Errorf("%q: the risk is not said in Bulgarian", permanence)
		}

		for _, item := range []string{"zz responsibility", "zz limit", "zz skill", "ZZ Developer", "zz-dev", "zz-model"} {
			if !strings.Contains(bg, item) {
				t.Errorf("%q: %q is not on the Bulgarian card", permanence, item)
			}
		}
	}

	existing := Proposal{Existing: &Agent{Name: "zz", Title: "ZZ"}}
	if !hasCyrillic(existing.TextIn("bg")) || existing.TextIn("en") != existing.Text() {
		t.Errorf("nobody new: %q", existing.TextIn("bg"))
	}
}
