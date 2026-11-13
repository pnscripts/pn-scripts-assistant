package terms

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/proof"
)

// A brain nobody has agreed to is not agreed to, and says so without an error.
func TestANewBrainHasNotAcceptedTheTerms(t *testing.T) {
	root := t.TempDir()

	if Accepted(root) {
		t.Fatal("a brain with no record counts as accepted")
	}

	r, err := Load(root)
	if err != nil || r.Version != 0 {
		t.Fatalf("Load of nothing = %+v, %v; want an empty record and no error", r, err)
	}
}

// Accepting writes a record that the next start reads back.
func TestAcceptingIsKeptBesideTheBrain(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 13, 9, 30, 15, 500, time.FixedZone("Sofia", 2*3600))

	r, err := Accept(root, Version, "bg-BG", now)
	if err != nil {
		t.Fatal(err)
	}

	if r.Version != Version || r.Language != "bg" || !r.AcceptedAt.Equal(now.UTC().Truncate(time.Second)) {
		t.Fatalf("record = %+v", r)
	}

	if !Accepted(root) {
		t.Fatal("accepted, and still not accepted")
	}

	back, err := Load(root)
	if err != nil || back != r {
		t.Fatalf("read back %+v, %v; want %+v", back, err, r)
	}

	// Nothing half-written is left behind.
	left, _ := filepath.Glob(filepath.Join(root, ".terms-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// Only the terms in force can be accepted: a page left open across an update
// showed other words.
func TestOnlyTheTermsInForceCanBeAccepted(t *testing.T) {
	root := t.TempDir()

	for _, v := range []int{0, Version - 1, Version + 1} {
		if _, err := Accept(root, v, "en", time.Now()); !errors.Is(err, ErrOtherVersion) {
			t.Errorf("accepting version %d: %v, want ErrOtherVersion", v, err)
		}
	}

	if Accepted(root) {
		t.Fatal("a refused acceptance was kept")
	}
}

// When the terms change, an acceptance of the older ones asks again.
func TestOlderTermsAreAskedAgain(t *testing.T) {
	root := t.TempDir()

	old := `{"version": ` + strconv.Itoa(Version-1) + `, "accepted_at": "2026-01-01T00:00:00Z", "language": "en"}`
	if err := os.WriteFile(filepath.Join(root, AcceptedFile), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	if Accepted(root) {
		t.Fatal("an acceptance of older terms counts for these")
	}
}

// A damaged record asks again rather than letting it through.
func TestADamagedRecordAsksAgain(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, AcceptedFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if Accepted(root) {
		t.Fatal("a damaged record counts as accepted")
	}

	if _, err := Load(root); err == nil {
		t.Fatal("a damaged record read without an error")
	}
}

// A folder that cannot be written to says so and keeps nothing.
func TestAnUnwritableFolderIsAnError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes anywhere")
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { os.Chmod(root, 0o700) })

	if _, err := Accept(root, Version, "en", time.Now()); err == nil {
		t.Fatal("accepting into a read-only folder said nothing")
	}

	if Accepted(root) {
		t.Fatal("accepted with nothing written")
	}
}

// The terms say what article 50 asks, and what the program does, in both
// languages, with the same number of parts.
func TestTheTermsSayItIsAnAIAndWhatIsMarked(t *testing.T) {
	en, bg := In("en"), In("bg")

	if len(en.Sections) != len(bg.Sections) || len(en.Sections) < 5 {
		t.Fatalf("%d English parts, %d Bulgarian", len(en.Sections), len(bg.Sections))
	}

	all := func(t Text) string {
		var b strings.Builder

		b.WriteString(t.Title + "\n" + t.Lead + "\n" + t.Version + "\n")

		for _, s := range t.Sections {
			b.WriteString(s.Heading + "\n" + s.Body + "\n")
		}

		return b.String()
	}

	for _, want := range []string{"artificial intelligence (AI) system", "language model", "can be wrong",
		"encrypted", "hosted model", "private mode", "without asking first", "AI-Disclosure: mode=ai-originated",
		"AI-generated", "MIT licence", "asks you again", "docs/guide/terms.md", "Version 1, 13 November 2026"} {
		if !strings.Contains(all(en), want) {
			t.Errorf("the English terms do not say %q", want)
		}
	}

	for _, want := range []string{"система с изкуствен интелект", "езиков модел", "грешни", "шифровано",
		"модел в облака", "поверителен режим", "без да пита", "AI-Disclosure: mode=ai-originated",
		"AI-generated", "лиценз MIT", "асистентът ще ви попита", "docs/guide/terms.bg.md", "Версия 1, 13.11.2026"} {
		if !strings.Contains(all(bg), want) {
			t.Errorf("the Bulgarian terms do not say %q", want)
		}
	}

	for _, text := range []string{all(en), all(bg)} {
		if strings.ContainsAny(text, "–—") {
			t.Errorf("a dash in the terms: %q", text)
		}
	}

	if In("BG_bg").Title != bg.Title || In("").Title != en.Title || In("de").Title != en.Title {
		t.Error("the language is not chosen the way the rest of the program chooses it")
	}

	text := all(bg)
	for _, f := range proof.Check(text, "bg") {
		from, to := max(0, f.At-40), min(len(text), f.At+f.Length+40)
		t.Errorf("Bulgarian terms: %s %q in %q", f.Rule, f.Found, strings.ToValidUTF8(text[from:to], ""))
	}
}

// The documentation is the same terms, sentence for sentence, so what a person
// reads before installing is what they accept.
func TestTheDocumentationHoldsTheSameTerms(t *testing.T) {
	for _, c := range []struct{ file, lang string }{
		{"../../../../docs/guide/terms.md", "en"},
		{"../../../../docs/guide/terms.bg.md", "bg"},
	} {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}

		doc := flat(string(raw))
		want := In(c.lang)

		for _, s := range append([]string{want.Title, want.Lead, want.Version}, pieces(want)...) {
			if !strings.Contains(doc, flat(s)) {
				t.Errorf("%s does not say %q", filepath.Base(c.file), s)
			}
		}
	}
}

func pieces(t Text) []string {
	var out []string

	for _, s := range t.Sections {
		out = append(out, s.Heading, s.Body)
	}

	return out
}

// flat is text with Markdown's line breaks, emphasis and code marks taken out.
func flat(s string) string {
	s = strings.NewReplacer("`", "", "**", "").Replace(s)

	return strings.Join(strings.Fields(s), " ")
}
