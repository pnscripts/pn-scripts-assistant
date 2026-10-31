/*
 * Package proof is a proofreading pass that does not need a model.
 *
 * A model asked to proofread rewrites: it changes words that were right,
 * smooths a voice that was the writer's own, and cannot say which of its
 * changes were corrections and which were taste. What somebody wants from a
 * proofreader first is the opposite — the slips, each one pointed at, each
 * one with the reason, and their own sentences otherwise left alone.
 *
 * So the rules that can be decided from the text itself are decided here, in
 * English and in Bulgarian: a doubled word, spacing around punctuation, a
 * Latin letter hiding inside a Cyrillic word, "в" and "във", the full article
 * after a preposition, the missing comma before "че", "a" and "an", "could
 * of". Each finding says whether it is sure. Sure ones may be applied in one
 * press; the others are shown and left for a person, because a rule that is
 * usually right is still sometimes wrong, and a proofreader that introduces
 * errors is worse than none.
 *
 * Meaning, tone and style stay with the writer specialist and its model. This
 * package only says what it can prove.
 */
package proof

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Finding is one slip in a text.
type Finding struct {
	// Rule is a short name for the kind of slip, the same in both languages.
	Rule string `json:"rule"`

	// At and Length are the byte span of what was found in the text checked.
	At     int `json:"at"`
	Length int `json:"length"`

	// Found is the text at that span; Suggest is what should replace it, or
	// empty when there is nothing certain to put there.
	Found   string `json:"found"`
	Suggest string `json:"suggest,omitempty"`

	// Why is the reason, in English and in Bulgarian.
	Why   string `json:"why"`
	WhyBG string `json:"why_bg"`

	// Sure is whether the suggestion is safe to apply without reading it.
	Sure bool `json:"sure"`
}

// Language guesses whether text is Bulgarian ("bg") or English ("en"): the
// script most of its letters are written in.
func Language(text string) string {
	cyr, lat := 0, 0

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case unicode.Is(unicode.Latin, r):
			lat++
		}
	}

	if cyr > lat {
		return "bg"
	}

	return "en"
}

// Check finds the slips in text. lang is "en", "bg" or empty to guess.
// Markdown code — fenced blocks and `inline` — is not checked (code.go).
func Check(text, lang string) []Finding {
	code := codeSpans(text)
	text = withoutCode(text, code)

	if lang != "bg" && lang != "en" {
		lang = Language(text)
	}

	var out []Finding

	out = append(out, spacing(text)...)
	out = append(out, repeated(text, lang)...)
	out = append(out, mixedScript(text)...)
	out = append(out, capitals(text)...)
	out = append(out, unbalanced(text)...)

	if lang == "bg" {
		out = append(out, bulgarian(text)...)
	} else {
		out = append(out, english(text)...)
	}

	return tidy(outsideCode(out, code))
}

/*
 * Apply puts every sure suggestion into text, and those named in also
 * (by position in findings) whether sure or not.
 *
 * From the end backwards, so an earlier replacement never moves a later one;
 * and a finding that overlaps one already applied is skipped rather than
 * guessed at.
 */
func Apply(text string, findings []Finding, also ...int) string {
	wanted := map[int]bool{}
	for _, i := range also {
		wanted[i] = true
	}

	type change struct {
		at, end int
		with    string
	}

	var changes []change

	for i, f := range findings {
		if !f.Sure && !wanted[i] {
			continue
		}

		if f.At < 0 || f.At+f.Length > len(text) || text[f.At:f.At+f.Length] != f.Found {
			continue
		}

		// An empty suggestion means "take this out" only for the slips that
		// are taken out; for the others it means nothing is known to put there.
		if f.Suggest == "" && f.Rule != "repeated_word" && f.Rule != "space_before_punctuation" {
			continue
		}

		changes = append(changes, change{f.At, f.At + f.Length, f.Suggest})
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].at > changes[j].at })

	limit := len(text) + 1

	for _, c := range changes {
		if c.end > limit {
			continue
		}

		text = text[:c.at] + c.with + text[c.end:]
		limit = c.at
	}

	return text
}

// tidy sorts findings by position and drops one that repeats another's span
// and suggestion.
func tidy(in []Finding) []Finding {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].At != in[j].At {
			return in[i].At < in[j].At
		}

		return in[i].Length < in[j].Length
	})

	out := in[:0]
	seen := map[string]bool{}

	for _, f := range in {
		key := fmt.Sprintf("%d:%d:%s:%s", f.At, f.Length, f.Rule, f.Suggest)
		if seen[key] {
			continue
		}

		seen[key] = true
		out = append(out, f)
	}

	return out
}

// word is one word of a text and where it is.
type word struct {
	text    string
	at, end int
}

var wordPattern = regexp.MustCompile(`[\p{L}\p{M}][\p{L}\p{M}'’]*`)

func words(text string) []word {
	spans := wordPattern.FindAllStringIndex(text, -1)

	out := make([]word, 0, len(spans))

	for _, s := range spans {
		out = append(out, word{text: text[s[0]:s[1]], at: s[0], end: s[1]})
	}

	return out
}

// between is the text separating two words.
func between(text string, a, b word) string { return text[a.end:b.at] }

// onlySpace reports whether s is spaces and tabs, and at least one.
func onlySpace(s string) bool {
	return s != "" && strings.Trim(s, " \t ") == ""
}

/*
 * spacing is the commonest slip in anything typed: two spaces, a space
 * before a comma, none after one.
 */
var (
	doubleSpace  = regexp.MustCompile(`[^\s] {2,}[^\s]`)
	spaceBefore  = regexp.MustCompile(`[^\s][ \t]+[,.;:!?)\]]`)
	missingAfter = regexp.MustCompile(`[,;!?]\p{L}`)
	colonAfter   = regexp.MustCompile(`[\p{Ll}]:[\p{Lu}]`)
	stopAfter    = regexp.MustCompile(`[\p{Ll}]{2}\.[\p{Lu}][\p{Ll}]`)
)

func spacing(text string) []Finding {
	var out []Finding

	for _, m := range doubleSpace.FindAllStringIndex(text, -1) {
		// The match includes a character either side; the spaces are between.
		start := m[0] + firstRuneLen(text[m[0]:])
		end := m[1] - lastRuneLen(text[:m[1]])

		out = append(out, Finding{
			Rule: "double_space", At: start, Length: end - start, Found: text[start:end], Suggest: " ",
			Why: "Two or more spaces between words.", WhyBG: "Два или повече интервала между думите.",
			Sure: true,
		})
	}

	for _, m := range spaceBefore.FindAllStringIndex(text, -1) {
		start := m[0] + firstRuneLen(text[m[0]:])
		end := m[1] - 1

		// "...": an ellipsis may stand after a space at the start of a
		// quotation, and a lone dot after a space is usually a file name.
		if text[end] == '.' && end+1 < len(text) && text[end+1] != ' ' && text[end+1] != '\n' {
			continue
		}

		// "[ ]" is a Markdown checkbox, not a stray space: taking the space
		// out would turn an unticked item into a pair of brackets.
		if text[end] == ']' && text[m[0]] == '[' && start == m[0]+1 && end == start+1 {
			continue
		}

		// A smiley is not punctuation.
		if text[end] == ':' || text[end] == ')' {
			if end+1 < len(text) && strings.ContainsRune("-)(DPp", rune(text[end+1])) {
				continue
			}

			if text[end] == ')' && end > 0 && strings.ContainsRune(":;", rune(text[end-1])) {
				continue
			}
		}

		out = append(out, Finding{
			Rule: "space_before_punctuation", At: start, Length: end - start, Found: text[start:end], Suggest: "",
			Why:   "No space goes before this punctuation mark.",
			WhyBG: "Пред този препинателен знак не се оставя интервал.",
			Sure:  true,
		})
	}

	for _, m := range missingAfter.FindAllStringIndex(text, -1) {
		mark := m[0]

		if inAddress(text, mark) {
			continue
		}

		out = append(out, Finding{
			Rule: "missing_space", At: mark, Length: 1, Found: text[mark : mark+1], Suggest: text[mark:mark+1] + " ",
			Why:   "A space goes after this punctuation mark.",
			WhyBG: "След този препинателен знак се оставя интервал.",
			Sure:  true,
		})
	}

	for _, re := range []*regexp.Regexp{colonAfter, stopAfter} {
		for _, m := range re.FindAllStringIndex(text, -1) {
			// The mark follows one lower-case letter, or two for a full stop.
			mark := m[0] + firstRuneLen(text[m[0]:])
			if re == stopAfter {
				mark += firstRuneLen(text[mark:])
			}

			// An address or a file name: no spaces round it, a dot or slash
			// in it, or "www".
			if inAddress(text, mark) {
				continue
			}

			out = append(out, Finding{
				Rule: "missing_space", At: mark, Length: 1, Found: text[mark : mark+1], Suggest: text[mark:mark+1] + " ",
				Why:   "A space goes after the end of a sentence or a colon.",
				WhyBG: "След края на изречението или двоеточие се оставя интервал.",
				Sure:  false,
			})
		}
	}

	return out
}

// inAddress reports whether position i sits inside something like a web
// address, an e-mail address or a file name.
func inAddress(text string, i int) bool {
	start, end := i, i

	for start > 0 && !isSpace(text[start-1]) {
		start--
	}

	for end < len(text) && !isSpace(text[end]) {
		end++
	}

	token := strings.ToLower(text[start:end])

	return strings.Contains(token, "/") || strings.Contains(token, "@") ||
		strings.HasPrefix(token, "www") || strings.Count(token, ".") > 1 ||
		strings.Contains(token, "://") || strings.ContainsAny(token, "=&")
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' }

func firstRuneLen(s string) int {
	_, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return 1
	}

	return n
}

func lastRuneLen(s string) int {
	_, n := utf8.DecodeLastRuneInString(s)
	if n == 0 {
		return 1
	}

	return n
}

// Words that are correct said twice.
var twiceIsRight = map[string]bool{
	"had": true, "that": true, "bye": true, "no": true, "yes": true, "very": true,
	"да": true, "не": true, "ха": true, "хо": true, "много": true, "тук": true,
	"бавно": true, "скоро": true,
}

func repeated(text, lang string) []Finding {
	var out []Finding

	list := words(text)

	for i := 1; i < len(list); i++ {
		a, b := list[i-1], list[i]

		if !onlySpace(between(text, a, b)) || !strings.EqualFold(a.text, b.text) {
			continue
		}

		if twiceIsRight[strings.ToLower(b.text)] {
			continue
		}

		out = append(out, Finding{
			Rule: "repeated_word", At: a.end, Length: b.end - a.end, Found: text[a.end:b.end], Suggest: "",
			Why:   "The word \"" + b.text + "\" is written twice.",
			WhyBG: "Думата „" + b.text + "“ е написана два пъти.",
			Sure:  true,
		})
	}

	return out
}

/*
 * A Latin letter inside a Cyrillic word, or the other way round.
 *
 * They look identical — а and a, о and o, р and p — so nobody reading sees
 * it, and every search, spell checker and screen reader does: the word is not
 * the word. It happens whenever a keyboard layout is switched mid-word.
 */
var latinToCyrillic = map[rune]rune{
	'a': 'а', 'e': 'е', 'o': 'о', 'p': 'р', 'c': 'с', 'x': 'х', 'y': 'у', 'k': 'к', 'm': 'м', 't': 'т',
	'A': 'А', 'B': 'В', 'E': 'Е', 'K': 'К', 'M': 'М', 'H': 'Н', 'O': 'О', 'P': 'Р', 'C': 'С', 'T': 'Т',
	'X': 'Х', 'Y': 'У',
}

var cyrillicToLatin = func() map[rune]rune {
	out := map[rune]rune{}

	for l, c := range latinToCyrillic {
		out[c] = l
	}

	return out
}()

func mixedScript(text string) []Finding {
	var out []Finding

	for _, w := range words(text) {
		cyr, lat := 0, 0

		for _, r := range w.text {
			switch {
			case unicode.Is(unicode.Cyrillic, r):
				cyr++
			case unicode.Is(unicode.Latin, r):
				lat++
			}
		}

		if cyr == 0 || lat == 0 {
			continue
		}

		toCyrillic := cyr >= lat
		fixed, ok := convert(w.text, toCyrillic)

		f := Finding{
			Rule: "mixed_letters", At: w.at, Length: w.end - w.at, Found: w.text,
			Why:   "This word mixes Latin and Cyrillic letters that look alike, so searches and spell checks miss it.",
			WhyBG: "Тази дума смесва еднакво изглеждащи латински и кирилски букви, затова търсенето и проверката на правописа не я разпознават.",
		}

		if ok {
			f.Suggest, f.Sure = fixed, true
		}

		out = append(out, f)
	}

	return out
}

// convert swaps look-alike letters into one script; ok is false when a
// letter has no look-alike and the word stays mixed.
func convert(s string, toCyrillic bool) (string, bool) {
	table := cyrillicToLatin
	foreign := unicode.Cyrillic

	if toCyrillic {
		table, foreign = latinToCyrillic, unicode.Latin
	}

	var b strings.Builder

	ok := true

	for _, r := range s {
		if unicode.Is(foreign, r) {
			if to, found := table[r]; found {
				b.WriteRune(to)

				continue
			}

			ok = false
		}

		b.WriteRune(r)
	}

	return b.String(), ok
}

// Abbreviations a sentence does not end at.
var abbreviations = map[string]bool{
	"e.g": true, "i.e": true, "etc": true, "vs": true, "mr": true, "mrs": true, "ms": true, "dr": true,
	"no": true, "approx": true, "fig": true, "p": true, "pp": true, "st": true, "jr": true, "sr": true,
	"т": true, "т.е": true, "т.н": true, "напр": true, "г": true, "гр": true, "ул": true, "бул": true,
	"св": true, "др": true, "вкл": true, "мин": true, "стр": true, "проф": true, "доц": true, "с": true,
	"лв": true, "хил": true, "млн": true, "млрд": true, "чл": true, "ал": true, "т.нар": true,
	"вж": true, "изд": true, "пр": true, "сл": true, "хр": true, "англ": true, "лат": true, "бел": true, "прев": true, "год": true, "сек": true, "акад": true, "инж": true, "кв": true, "ж.к": true, "р": true, "ч": true,
}

var sentenceStart = regexp.MustCompile(`([\p{L}\p{N}.]+)([.!?])[ \t]+(\p{Ll})`)

func capitals(text string) []Finding {
	var out []Finding

	for _, m := range sentenceStart.FindAllStringSubmatchIndex(text, -1) {
		before := strings.ToLower(strings.TrimSuffix(text[m[2]:m[3]], "."))
		mark := text[m[4]:m[5]]

		if mark == "." && (abbreviations[before] || strings.Contains(before, ".") ||
			utf8.RuneCountInString(before) == 1 || isNumber(before)) {
			continue
		}

		at := m[6]
		r, size := utf8.DecodeRuneInString(text[at:])

		// "iPhone", "eBay": a lower-case letter followed by a capital is a
		// name written that way.
		if next, _ := utf8.DecodeRuneInString(text[at+size:]); unicode.IsUpper(next) {
			continue
		}

		out = append(out, Finding{
			Rule: "capital_letter", At: at, Length: size, Found: text[at : at+size],
			Suggest: string(unicode.ToUpper(r)),
			Why:     "A sentence starts with a capital letter.",
			WhyBG:   "Изречението започва с главна буква.",
			Sure:    true,
		})
	}

	return out
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' && r != ',' {
			return false
		}
	}

	return true
}

/*
 * unbalanced is a bracket or a quotation mark opened and never closed, or the
 * other way round, within one paragraph. Nothing is suggested: where the
 * missing one belongs is the writer's to say.
 */
func unbalanced(text string) []Finding {
	var out []Finding

	at := 0

	for _, para := range strings.Split(text, "\n") {
		pairs := [][2]rune{{'(', ')'}, {'„', '“'}, {'«', '»'}}

		for _, p := range pairs {
			open, closed := strings.Count(para, string(p[0])), strings.Count(para, string(p[1]))

			// “ also opens English quotations; only Bulgarian „ counts it as
			// a closing mark.
			if p[0] == '„' && open == 0 {
				continue
			}

			if open == closed {
				continue
			}

			// A list numbered "1)" or "a)" closes what was never opened.
			if p[0] == '(' && open < closed && listNumbered(para) {
				continue
			}

			mark := p[0]
			if closed > open {
				mark = p[1]
			}

			i := strings.IndexRune(para, mark)

			out = append(out, Finding{
				Rule: "unbalanced", At: at + i, Length: utf8.RuneLen(mark), Found: string(mark),
				Why:   "A bracket or quotation mark here has no partner in the same paragraph.",
				WhyBG: "Скоба или кавичка тук няма своята двойка в същия абзац.",
			})
		}

		if strings.Count(para, `"`)%2 == 1 {
			i := strings.Index(para, `"`)

			out = append(out, Finding{
				Rule: "unbalanced", At: at + i, Length: 1, Found: `"`,
				Why:   "A quotation mark here has no partner in the same paragraph.",
				WhyBG: "Кавичка тук няма своята двойка в същия абзац.",
			})
		}

		at += len(para) + 1
	}

	return out
}

var numberedItem = regexp.MustCompile(`(^|\s)(\d{1,2}|[a-zа-я])\)`)

func listNumbered(para string) bool { return numberedItem.MatchString(para) }

// keepCase writes replacement in the case of the original's first letter.
func keepCase(original, replacement string) string {
	r, _ := utf8.DecodeRuneInString(original)
	if !unicode.IsUpper(r) {
		return replacement
	}

	first, size := utf8.DecodeRuneInString(replacement)

	return string(unicode.ToUpper(first)) + replacement[size:]
}
