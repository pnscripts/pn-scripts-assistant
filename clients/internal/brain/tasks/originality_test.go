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
