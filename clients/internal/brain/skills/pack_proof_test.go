package skills

import (
	"io/fs"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/proof"
)

/*
 * The Bulgarian half of the shipped pack reads clean to the assistant's own
 * proofreader: a slip a user can see in a built-in skill is ours, and a false
 * alarm on correct Bulgarian is a rule to fix, not a finding to live with.
 * Examples that show mistakes on purpose are written as code.
 */
func TestTheShippedBulgarianPackHasNoSlips(t *testing.T) {
	_ = fs.WalkDir(packFS, "pack", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".bg.md") {
			return err
		}

		data, _ := packFS.ReadFile(p)
		text := string(data)

		for _, f := range proof.Check(text, "bg") {
			from, to := max(0, f.At-40), min(len(text), f.At+f.Length+40)
			t.Errorf("%s: %s %q in %q", p, f.Rule, f.Found, strings.ToValidUTF8(text[from:to], ""))
		}

		return nil
	})
}
