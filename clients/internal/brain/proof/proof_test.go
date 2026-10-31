package proof

import (
	"strings"
	"testing"
)

// fixed is text with every sure suggestion applied.
func fixed(text, lang string) string { return Apply(text, Check(text, lang)) }

func rules(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Rule)
	}

	return out
}

func TestTheLanguageIsTheScriptMostLettersAreIn(t *testing.T) {
	if got := Language("Здравейте, как сте? OK"); got != "bg" {
		t.Errorf("Bulgarian text read as %q", got)
	}

	if got := Language("Hello, how are you? Добре"); got != "en" {
		t.Errorf("English text read as %q", got)
	}
}

/*
 * Each Bulgarian rule, on a sentence where it is wrong, comes back corrected;
 * and on the same sentence written correctly, finds nothing.
 */
func TestBulgarianSlipsAreCorrected(t *testing.T) {
	cases := []struct{ wrong, right string }{
		{"Живея в Варна.", "Живея във Варна."},
		{"Отивам във София.", "Отивам в София."},
		{"Говорих с сестра си.", "Говорих със сестра си."},
		{"Дойде със брат си.", "Дойде с брат си."},
		{"Дадох й книгата.", "Дадох ѝ книгата."},
		{"Аз незнам какво стана.", "Аз не знам какво стана."},
		{"Това е по добре и най важно.", "Това е по-добре и най-важно."},
		{"Седнах на столът до прозореца.", "Седнах на стола до прозореца."},
		{"Мисля че е прав.", "Мисля, че е прав."},
		{"Дойдох защото ме покани.", "Дойдох, защото ме покани."},
		{"Това е човекът който ми помогна.", "Това е човекът, който ми помогна."},
		{"Искам да дойда но не мога.", "Искам да дойда, но не мога."},
		{"Тя каза \"добре\" и излезе.", "Тя каза „добре“ и излезе."},
		{"Пишем  с два интервала .", "Пишем с два интервала."},
		{"Купих мляко,хляб и сирене.", "Купих мляко, хляб и сирене."},
		{"Това е е грешка.", "Това е грешка."},
		{"Добре дошли. заповядайте.", "Добре дошли. Заповядайте."},
	}

	for _, c := range cases {
		if got := fixed(c.wrong, "bg"); got != c.right {
			t.Errorf("%q\n got %q\nwant %q\nfound %v", c.wrong, got, c.right, rules(Check(c.wrong, "bg")))
		}
	}
}

func TestCorrectBulgarianIsLeftAlone(t *testing.T) {
	for _, text := range []string{
		"Живея във Варна и работя в София.",
		"Говорих със сестра си и с брат си.",
		"Дадох ѝ книгата, защото я поиска.",
		"Столът е до прозореца, а аз седнах на стола.",
		"Мисля, че е прав, така че ще дойда.",
		"Това е човекът, на който помогнах.",
		"По пътя видях целия свят.",
		"Тръгнах, въпреки че валеше.",
		"Не че не искам, но не мога.",
		"Всички деца пеят и се смеят.",
		"Т.е. това е напр. т.н. списък.",
		"„Добре“, каза тя.",
		"Пак по малко от всичко.",
		"Да, да, разбрах.",
	} {
		if fs := Check(text, "bg"); len(fs) > 0 {
			t.Errorf("%q: found %v (%q)", text, rules(fs), fs[0].Found)
		}
	}
}

func TestEnglishSlipsAreCorrected(t *testing.T) {
	cases := []struct{ wrong, right string }{
		{"It was a honest mistake.", "It was an honest mistake."},
		{"She has an university degree.", "She has a university degree."},
		{"He ate a apple and an pear.", "He ate an apple and a pear."},
		{"We need a MBA and a URL.", "We need an MBA and a URL."},
		{"I could of gone.", "I could have gone."},
		{"This is better then that.", "This is better than that."},
		{"Then i left.", "Then I left."},
		{"We have alot of time.", "We have a lot of time."},
		{"It is is done.", "It is done."},
		{"Hello ,world.", "Hello, world."},
		{"Done. next one.", "Done. Next one."},
	}

	for _, c := range cases {
		if got := fixed(c.wrong, "en"); got != c.right {
			t.Errorf("%q\n got %q\nwant %q\nfound %v", c.wrong, got, c.right, rules(Check(c.wrong, "en")))
		}
	}
}

func TestCorrectEnglishIsLeftAlone(t *testing.T) {
	for _, text := range []string{
		"An hour, a user, a one-off, an unimportant note and an FBI agent.",
		"I said that that was fine, and he had had enough.",
		"See e.g. the notes, i.e. the file at www.example.com/a.b and mail me@example.com.",
		"First this, then that.",
		"The iPhone and eBay are names.",
		"Version 3.5 costs 10,000 a year at 10:30.",
		"Lists: 1) one 2) two.",
	} {
		if fs := Check(text, "en"); len(fs) > 0 {
			t.Errorf("%q: found %v (%q)", text, rules(fs), fs[0].Found)
		}
	}
}

/*
 * A Latin letter typed inside a Cyrillic word is found and turned into its
 * Cyrillic twin; a word with a Latin letter that has no twin is pointed at
 * and left alone.
 */
func TestLettersFromTheWrongAlphabetAreFound(t *testing.T) {
	text := "Това е пpобa."
	fs := Check(text, "bg")

	if len(fs) != 1 || fs[0].Rule != "mixed_letters" || !fs[0].Sure {
		t.Fatalf("found %+v", fs)
	}

	if got := Apply(text, fs); got != "Това е проба." {
		t.Errorf("fixed to %q", got)
	}

	if strings.ContainsAny(Apply(text, fs), "pa") {
		t.Error("a Latin letter survived")
	}

	fs = Check("Думаz", "bg")
	if len(fs) != 1 || fs[0].Sure {
		t.Errorf("a letter with no twin should be pointed at, not changed: %+v", fs)
	}
}

func TestOnlySureFindingsApplyUnlessChosen(t *testing.T) {
	text := "Това (е тест."
	fs := Check(text, "bg")

	if len(fs) != 1 || fs[0].Rule != "unbalanced" || fs[0].Sure {
		t.Fatalf("found %+v", fs)
	}

	if Apply(text, fs) != text || Apply(text, fs, 0) != text {
		t.Error("a finding with nothing to suggest changed the text")
	}

	text = "Done:Next"
	fs = Check(text, "en")

	if len(fs) != 1 || fs[0].Sure {
		t.Fatalf("found %+v", fs)
	}

	if Apply(text, fs) != text {
		t.Error("a finding that is not sure was applied without being chosen")
	}

	if got := Apply(text, fs, 0); got != "Done: Next" {
		t.Errorf("chosen finding applied as %q", got)
	}
}

func TestStaleFindingsAreNotApplied(t *testing.T) {
	fs := Check("Живея в Варна.", "bg")

	if got := Apply("Нещо съвсем друго тук.", fs); got != "Нещо съвсем друго тук." {
		t.Errorf("findings from another text changed this one: %q", got)
	}
}

func TestToneChecksCountWhatCanBeCounted(t *testing.T) {
	formal, _ := ToneNamed("formal")

	fs := CheckTone("Hey guys, we can't do this ASAP!", "en", formal, 0)
	got := strings.Join(rules(fs), " ")

	for _, want := range []string{"tone_word", "contraction", "exclamation", "capitals"} {
		if !strings.Contains(got, want) {
			t.Errorf("formal check missed %s: %s", want, got)
		}
	}

	concise, ok := ToneNamed("Кратък")
	if !ok {
		t.Fatal("a tone is not found by its Bulgarian name")
	}

	long := strings.Repeat("дума ", 30) + "край."
	fs = CheckTone("Всъщност това е готово. "+long, "bg", concise, 10)
	got = strings.Join(rules(fs), " ")

	for _, want := range []string{"tone_word", "long_sentence", "too_long"} {
		if !strings.Contains(got, want) {
			t.Errorf("concise check missed %s: %s", want, got)
		}
	}

	for _, f := range fs {
		if f.WhyBG == "" || f.Why == "" {
			t.Errorf("%s has no reason in both languages", f.Rule)
		}
	}

	friendly, _ := ToneNamed("friendly")
	if fs := CheckTone("Thanks so much, see you soon.", "en", friendly, 0); len(fs) != 0 {
		t.Errorf("a friendly text found %v", rules(fs))
	}
}

func TestEveryToneHasBothLanguages(t *testing.T) {
	for _, tone := range Tones {
		if tone.Label == "" || tone.LabelBG == "" || tone.Brief == "" || tone.BriefBG == "" {
			t.Errorf("%s is missing a label or a brief", tone.Name)
		}
	}
}

func TestMeasure(t *testing.T) {
	s := Measure("One two three. Four five.\n\nSix seven eight nine?")

	if s.Words != 9 || s.Sentences != 3 || s.Paragraphs != 2 || s.Longest != 4 || s.Minutes != 1 {
		t.Errorf("%+v", s)
	}

	if Measure("").Minutes != 0 {
		t.Error("an empty text takes time to read")
	}
}

/*
 * A comparison puts back either version exactly: the parts the two share
 * with what was removed are the first, with what was added the second.
 */
func TestAComparisonRebuildsBothVersions(t *testing.T) {
	pairs := [][2]string{
		{"The quick brown fox jumps.", "The quick red fox leaps."},
		{"Първи ред.\nВтори ред.\nТрети ред.\n", "Първи ред.\nВтори, променен ред.\nТрети ред.\nЧетвърти.\n"},
		{"", "Something new."},
		{"Gone entirely.", ""},
		{"Same.", "Same."},
		{"Hello world", "Hello, world"},
	}

	for _, p := range pairs {
		d := Compare(p[0], p[1])

		var before, after strings.Builder

		for _, part := range d.Parts {
			if part.Kind != "added" {
				before.WriteString(part.Text)
			}

			if part.Kind != "removed" {
				after.WriteString(part.Text)
			}
		}

		if before.String() != p[0] || after.String() != p[1] {
			t.Errorf("%q → %q rebuilt as %q → %q", p[0], p[1], before.String(), after.String())
		}

		if d.Changed() != (p[0] != p[1]) {
			t.Errorf("%q → %q: changed = %v", p[0], p[1], d.Changed())
		}
	}

	d := Compare("The quick brown fox jumps.", "The quick red fox leaps.")
	if got := d.Marked(); got != "The quick [-brown-]{+red+} fox [-jumps-]{+leaps+}." {
		t.Errorf("marked as %q", got)
	}

	if d.Removed != 2 || d.Added != 2 || d.Same != 3 {
		t.Errorf("counted %+v", d)
	}
}

func TestAHugeComparisonStillFinishes(t *testing.T) {
	a := strings.Repeat("alpha beta gamma delta ", 5000)
	b := strings.Repeat("alpha beta gamma epsilon ", 5000)

	d := Compare(a, b)
	if !d.Changed() {
		t.Error("two different long texts compared as the same")
	}
}

/*
 * Markdown code is left alone: what is wrong in prose is often right in a
 * command, a selector or a query, and the same slip outside the code is still
 * found.
 */
func TestCodeIsNotProofread(t *testing.T) {
	text := "Сложете правилото `color: red !important;` във файла.\n\n" +
		"```sql\nSELECT name FROM people WHERE (age > 18;\nSELECT \"име\" , 'x' FROM t;\n```\n\n" +
		"~~~\nthe the code\n~~~\n" +
		"Тук има грешка  в текста и и още една."

	found := Check(text, "bg")

	// The prose after the code is still checked, at the right place.
	prose := strings.LastIndex(text, "Тук")

	var after []string

	for _, f := range found {
		if f.At < prose {
			t.Errorf("a finding in or before the code: %+v (%q)", f, f.Found)
		}

		if text[f.At:f.At+f.Length] != f.Found {
			t.Errorf("a finding points at %q but says %q", text[f.At:f.At+f.Length], f.Found)
		}

		after = append(after, f.Rule)
	}

	if !strings.Contains(strings.Join(after, " "), "repeated_word") {
		t.Errorf("the doubled word after the code was missed: %v", after)
	}

	if !strings.Contains(strings.Join(after, " "), "double_space") {
		t.Errorf("the double space after the code was missed: %v", after)
	}

	// Applying the sure findings changes nothing inside the code.
	out := Apply(text, found)
	for _, keep := range []string{"`color: red !important;`", "WHERE (age > 18;", "the the code"} {
		if !strings.Contains(out, keep) {
			t.Errorf("applying the findings changed the code %q:\n%s", keep, out)
		}
	}
}

func TestWhatCountsAsCode(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"a `b` c", []string{"`b`"}},
		{"a ``b ` c`` d", []string{"``b ` c``"}},
		{"an unclosed ` stays text", nil},
		{"one `para\n\nnext` para", nil},
		{"x\n```\ncode\n```\ny", []string{"```\ncode\n```\n"}},
		{"x\n~~~~\n~~~\n~~~~\ny", []string{"~~~~\n~~~\n~~~~\n"}},
		{"x\n```\nnever closed", []string{"```\nnever closed"}},
		{"x\n    ```\nnot a fence (four spaces)", nil},
	}

	for _, c := range cases {
		var got []string
		for _, s := range codeSpans(c.text) {
			got = append(got, c.text[s.at:s.end])
		}

		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q: code %q, want %q", c.text, got, c.want)
		}
	}
}

// A Markdown checklist keeps its unticked boxes: "[ ]" is not a stray space.
func TestUntickedBoxesAreNotSlips(t *testing.T) {
	text := "- [ ] Номер на фактурата\n- [x] Дата\n- [ ] Сума\n"

	for _, f := range Check(text, "bg") {
		t.Errorf("a checklist was proofread: %+v", f)
	}

	if got := fixed("Done [x ] and ( a ]", "en"); got != "Done [x] and ( a]" {
		t.Errorf("a real space before a bracket was not fixed: %q", got)
	}
}
