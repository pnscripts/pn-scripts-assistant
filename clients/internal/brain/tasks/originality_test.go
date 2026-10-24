package tasks

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

func checkers(p Plan) []string {
	var out []string

	for _, s := range p.Steps {
		if strings.HasPrefix(s.Instruction, OriginalityMarker) {
			out = append(out, s.Assignee)
		}
	}

	return out
}

// Each result is checked by the specialists of the fields it touches, side
// by side, and only those — with the reasons written on the first check.
func TestEachResultIsCheckedByTheSpecialistsItNeedsOnly(t *testing.T) {
	base := Plan{Steps: []store.TaskStep{{Instruction: "do it"}, {Instruction: "finish it"}}}

	cases := map[string][]string{
		"Make me a great space strategy game":            {"visual_reviewer", "game_design_reviewer", "copy_reviewer", "business_strategist"},
		"Направи ми лого за пекарна":                     {"ip_specialist", "visual_reviewer"},
		"compose a jingle for the radio":                 {"sound_reviewer"},
		"Make a business strategy for a bakery in Sofia": {"copy_reviewer", "business_strategist"},
		"Направи бюджет и план за ДДС на фирмата":        {"copy_reviewer", "finance_analyst", "accountant"},
		"write a draft logo and slogan for myself":       {"ip_specialist"},
		"sort my downloads folder":                       nil,
	}

	for request, want := range cases {
		got := withOriginality(request, base)

		if strings.Join(checkers(got), ",") != strings.Join(want, ",") {
			t.Errorf("%q checked by %v, want %v", request, checkers(got), want)

			continue
		}

		if len(want) == 0 {
			continue
		}

		first := got.Steps[2]
		if !strings.Contains(first.Instruction, "not needed") && len(want) < len(aspects) {
			t.Errorf("%q: the reasons for the checks not used are not recorded: %s", request, first.Instruction)
		}

		for i, s := range got.Steps[2:] {
			if s.Kind != store.StepCheck || s.Together != (i > 0) {
				t.Errorf("%q: check %d is not a parallel check step: %+v", request, i, s)
			}
		}

		if again := withOriginality(request, got); len(again.Steps) != len(got.Steps) {
			t.Errorf("%q: checks added twice", request)
		}
	}
}

// The specialists' findings are one section of the final report, under their
// own heading, not more bullets among the answers.
func TestTheChecksHaveTheirOwnSectionInTheReport(t *testing.T) {
	plan := withOriginality("Направи ми лого за пекарна", Plan{Steps: []store.TaskStep{{Instruction: "draw the logo"}}})

	steps := plan.Steps
	steps[0].State, steps[0].Answer, steps[0].Verdict = store.StepDone, "logo.svg drawn", store.Verified
	steps[1].State, steps[1].Answer = store.StepDone, "no registered mark named Zorna in TMview"
	steps[2].State = store.StepRunning

	report := (&Conductor{}).report(&store.Task{Name: "logo"}, steps, store.TaskDone, "done")

	at := strings.Index(report, OriginalitySection)
	if at < 0 {
		t.Fatalf("no originality section:\n%s", report)
	}

	answers, section := report[:at], report[at:]

	if !strings.Contains(answers, "logo.svg drawn") || strings.Contains(answers, "TMview") {
		t.Errorf("the checks are mixed with the answers:\n%s", report)
	}

	for _, want := range []string{"• name and trademarks: no registered mark named Zorna", "• look: not done"} {
		if !strings.Contains(section, want) {
			t.Errorf("section lacks %q:\n%s", want, section)
		}
	}

	if plain := (&Conductor{}).report(&store.Task{Name: "sort"}, steps[:1], store.TaskDone, "done"); strings.Contains(plain, OriginalitySection) {
		t.Errorf("a result without checks got the section:\n%s", plain)
	}
}
