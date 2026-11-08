package proof

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The Bulgarian user guides read clean to the proofreader the program ships:
 * a guide that tells people how to install it is the first Bulgarian they
 * read from us (owner 26.10: correct Bulgarian, no slips).
 */
func TestTheBulgarianGuidesHaveNoSlips(t *testing.T) {
	guides, _ := filepath.Glob("../../../../docs/guide/*.bg.md")
	if len(guides) < 4 {
		t.Fatalf("found %d Bulgarian guides, want installing, configuring, troubleshooting and known limits", len(guides))
	}

	for _, path := range guides {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		text := string(raw)

		for _, f := range Check(text, "bg") {
			from, to := max(0, f.At-40), min(len(text), f.At+f.Length+40)
			t.Errorf("%s: %s %q in %q", filepath.Base(path), f.Rule, f.Found, strings.ToValidUTF8(text[from:to], ""))
		}
	}
}
