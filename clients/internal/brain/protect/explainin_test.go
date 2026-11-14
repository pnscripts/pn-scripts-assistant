package protect

import (
	"strings"
	"testing"
	"unicode"
)

func cyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}

	return false
}

// Every built-in rule is explained in Bulgarian on a Bulgarian card, with
// the path as it came and the English card unchanged.
func TestEveryRuleIsExplainedInBulgarian(t *testing.T) {
	for _, r := range append(BuiltIn, Rule{ID: "yours", What: "Something you added", Why: yoursWhy + "taxes"}) {
		bg := ExplainIn("Read /home/you/x", "/home/you/x", r, "bg")

		if !cyrillic(bg) || !strings.Contains(bg, "/home/you/x") || strings.Contains(bg, r.Why) {
			t.Errorf("%s: %q", r.ID, bg)
		}

		if ExplainIn("Read", "/p", r, "en") != Explain("Read", "/p", r) {
			t.Errorf("%s: the English card changed", r.ID)
		}
	}

	if got := ExplainIn("Read", "/p", BuiltIn[0], "bg"); !strings.Contains(got, "(SSH ключове)") {
		t.Errorf("an abbreviation was lowered: %q", got)
	}
}
