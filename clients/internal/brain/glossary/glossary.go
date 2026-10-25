/*
 * Package glossary is its owner's own list of how words are translated.
 *
 * Every translator who works for one client keeps one: the product is called
 * this, "invoice" is "фактура" and never "сметка", the company name is never
 * translated at all. A model has no such list and decides afresh each time,
 * so the same document comes back with the same word three ways. Here the
 * list is the owner's, kept in the brain's folder as plain JSON that travels
 * with it, given to the model with every translation, and checked afterwards:
 * a term the original uses whose agreed translation is not in the result is
 * reported, not assumed.
 */
package glossary

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// FileName is the glossary's file inside the brain's folder.
const FileName = "glossary.json"

// Term is one agreed translation.
type Term struct {
	// Source is the word or phrase in the original language; Target is how
	// it is always translated. A Target equal to Source means "never
	// translate this" — a name, a brand.
	Source string `json:"source"`
	Target string `json:"target"`

	// From and To are language codes ("en", "bg"). Empty From matches any
	// source language.
	From string `json:"from,omitempty"`
	To   string `json:"to"`

	Note  string    `json:"note,omitempty"`
	Added time.Time `json:"added"`
}

// Same reports whether two terms are the same entry: one source, one pair.
func (t Term) Same(o Term) bool {
	return strings.EqualFold(strings.TrimSpace(t.Source), strings.TrimSpace(o.Source)) &&
		t.From == o.From && t.To == o.To
}

var mu sync.Mutex

func path(root string) string { return filepath.Join(root, FileName) }

// Load reads the glossary. A brain without one has an empty glossary.
func Load(root string) ([]Term, error) {
	mu.Lock()
	defer mu.Unlock()

	return load(root)
}

func load(root string) ([]Term, error) {
	raw, err := os.ReadFile(path(root))
	if errors.Is(err, os.ErrNotExist) {
		return []Term{}, nil
	}

	if err != nil {
		return nil, err
	}

	var file struct {
		Terms []Term `json:"terms"`
	}

	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("the glossary file could not be read: %w", err)
	}

	if file.Terms == nil {
		file.Terms = []Term{}
	}

	return file.Terms, nil
}

func save(root string, terms []Term) error {
	sort.SliceStable(terms, func(i, j int) bool {
		if terms[i].To != terms[j].To {
			return terms[i].To < terms[j].To
		}

		return strings.ToLower(terms[i].Source) < strings.ToLower(terms[j].Source)
	})

	raw, err := json.MarshalIndent(map[string]any{"terms": terms}, "", "  ")
	if err != nil {
		return err
	}

	tmp := path(root) + ".tmp"

	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path(root))
}

// Clean checks a term and tidies its fields; the error says what is wrong.
func Clean(t Term) (Term, error) {
	t.Source = strings.Join(strings.Fields(t.Source), " ")
	t.Target = strings.Join(strings.Fields(t.Target), " ")
	t.Note = strings.TrimSpace(t.Note)
	t.From = strings.ToLower(strings.TrimSpace(t.From))
	t.To = strings.ToLower(strings.TrimSpace(t.To))

	if t.From == "auto" {
		t.From = ""
	}

	switch {
	case t.Source == "":
		return t, errors.New("the term to translate is empty")
	case t.Target == "":
		return t, errors.New("the translation is empty")
	case !Code(t.To):
		return t, fmt.Errorf("%q is not a language code like en or bg", t.To)
	case t.From != "" && !Code(t.From):
		return t, fmt.Errorf("%q is not a language code like en or bg", t.From)
	case t.From != "" && t.From == t.To:
		return t, errors.New("the two languages are the same")
	case utf8.RuneCountInString(t.Source) > 200 || utf8.RuneCountInString(t.Target) > 200:
		return t, errors.New("a glossary term is a word or a phrase, not a paragraph")
	}

	return t, nil
}

var codePattern = regexp.MustCompile(`^[a-z]{2,3}$`)

// Code reports whether s looks like a language code.
func Code(s string) bool { return codePattern.MatchString(s) }

// Add puts a term in, replacing one with the same source and languages.
func Add(root string, t Term) (Term, bool, error) {
	t, err := Clean(t)
	if err != nil {
		return t, false, err
	}

	mu.Lock()
	defer mu.Unlock()

	terms, err := load(root)
	if err != nil {
		return t, false, err
	}

	if t.Added.IsZero() {
		t.Added = time.Now().UTC().Truncate(time.Second)
	}

	replaced := false

	for i := range terms {
		if terms[i].Same(t) {
			terms[i], replaced = t, true

			break
		}
	}

	if !replaced {
		terms = append(terms, t)
	}

	return t, replaced, save(root, terms)
}

// Remove takes a term out; found is false when there was no such term.
func Remove(root string, t Term) (bool, error) {
	t.Source = strings.Join(strings.Fields(t.Source), " ")
	t.From = strings.ToLower(strings.TrimSpace(t.From))
	t.To = strings.ToLower(strings.TrimSpace(t.To))

	if t.From == "auto" {
		t.From = ""
	}

	mu.Lock()
	defer mu.Unlock()

	terms, err := load(root)
	if err != nil {
		return false, err
	}

	out := terms[:0]
	found := false

	for _, have := range terms {
		if have.Same(t) || (t.To == "" && strings.EqualFold(have.Source, t.Source)) {
			found = true

			continue
		}

		out = append(out, have)
	}

	if !found {
		return false, nil
	}

	return true, save(root, out)
}

// For is the terms that apply to translating from one language to another.
// from may be empty when the source language is not known.
func For(terms []Term, from, to string) []Term {
	var out []Term

	for _, t := range terms {
		if t.To != to {
			continue
		}

		if t.From != "" && from != "" && t.From != from {
			continue
		}

		out = append(out, t)
	}

	return out
}

// Used is the terms whose source appears in text, as whole words.
func Used(terms []Term, text string) []Term {
	var out []Term

	lower := strings.ToLower(text)

	for _, t := range terms {
		if contains(lower, strings.ToLower(t.Source), false) {
			out = append(out, t)
		}
	}

	return out
}

// Miss is something a translation should have kept and did not.
type Miss struct {
	// Kind is "term" (a glossary term not translated as agreed) or "kept"
	// (a number, an address or a link missing from the translation).
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Want   string `json:"want"`
}

/*
 * Check compares an original with its translation: every glossary term the
 * original uses must appear as agreed — allowing a Bulgarian or English
 * ending on it, since "договор" is still the agreed word in "договора" — and
 * every number, e-mail address and link must survive unchanged.
 */
func Check(original, translation string, terms []Term) []Miss {
	var out []Miss

	lower := strings.ToLower(translation)

	for _, t := range Used(terms, original) {
		if !contains(lower, strings.ToLower(t.Target), true) {
			out = append(out, Miss{Kind: "term", Source: t.Source, Want: t.Target})
		}
	}

	for _, kept := range keptAsIs(original) {
		if !strings.Contains(translation, kept) && !strings.Contains(translation, strings.ReplaceAll(kept, ".", ",")) &&
			!strings.Contains(translation, strings.ReplaceAll(kept, ",", ".")) {
			out = append(out, Miss{Kind: "kept", Source: kept, Want: kept})
		}
	}

	return out
}

var keptPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+|[\w.+-]+@[\w-]+\.[\w.-]+|\d+(?:[.,:/]\d+)*`)

// keptAsIs is what a translation must carry over exactly, each once.
func keptAsIs(text string) []string {
	seen := map[string]bool{}

	var out []string

	for _, m := range keptPattern.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,;:")
		if m == "" || seen[m] {
			continue
		}

		seen[m] = true
		out = append(out, m)
	}

	return out
}

/*
 * contains reports whether phrase is in text starting at a word boundary,
 * and ending at one — or, with endings, followed only by a few more letters
 * (a case or article ending).
 */
func contains(text, phrase string, endings bool) bool {
	if phrase == "" {
		return false
	}

	for at := 0; at <= len(text); {
		i := strings.Index(text[at:], phrase)
		if i < 0 {
			return false
		}

		start, end := at+i, at+i+len(phrase)
		at = start + 1

		if start > 0 {
			r, _ := utf8.DecodeLastRuneInString(text[:start])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}

		tail := 0

		for rest := text[end:]; rest != ""; {
			r, size := utf8.DecodeRuneInString(rest)
			if !unicode.IsLetter(r) {
				break
			}

			tail++
			rest = rest[size:]
		}

		if tail == 0 || (endings && tail <= 4) {
			return true
		}
	}

	return false
}

// CSV writes terms as a spreadsheet: source, target, from, to, note.
func CSV(terms []Term) string {
	var b strings.Builder

	w := csv.NewWriter(&b)
	_ = w.Write([]string{"source", "target", "from", "to", "note"})

	for _, t := range terms {
		_ = w.Write([]string{t.Source, t.Target, t.From, t.To, t.Note})
	}

	w.Flush()

	return b.String()
}

/*
 * FromCSV reads terms written by CSV or typed by hand: source, target, and
 * optionally from, to and note. A header line is recognised and skipped; a
 * line with no "to" takes defaultTo. Tabs and semicolons work as well as
 * commas, which is what a spreadsheet in Bulgarian saves.
 */
func FromCSV(text, defaultFrom, defaultTo string) ([]Term, []string) {
	sep := ','
	first, _, _ := strings.Cut(text, "\n")

	switch {
	case strings.Contains(first, "\t"):
		sep = '\t'
	case strings.Count(first, ";") > strings.Count(first, ","):
		sep = ';'
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, []string{err.Error()}
	}

	var (
		out      []Term
		problems []string
	)

	for i, row := range rows {
		if i == 0 && len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), "source") {
			continue
		}

		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}

		t := Term{From: defaultFrom, To: defaultTo}

		cell := func(n int) string {
			if n < len(row) {
				return strings.TrimSpace(row[n])
			}

			return ""
		}

		t.Source, t.Target = cell(0), cell(1)

		if v := cell(2); v != "" {
			t.From = v
		}

		if v := cell(3); v != "" {
			t.To = v
		}

		t.Note = cell(4)

		clean, err := Clean(t)
		if err != nil {
			problems = append(problems, fmt.Sprintf("line %d: %v", i+1, err))

			continue
		}

		out = append(out, clean)
	}

	return out, problems
}
