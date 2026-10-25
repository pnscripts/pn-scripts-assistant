/*
 * Package translate translates text and documents with a model, keeping the
 * document's layout and its owner's glossary.
 *
 * A model asked to "translate this file" is handed text and gives back text,
 * and the Word document with a letterhead, a table and three styles comes
 * back as paragraphs in a .txt. What people mean is the same document in
 * another language. So the file is taken apart here into the pieces that are
 * words — a paragraph, a subtitle, a line of Markdown — the pieces are
 * translated, and each is put back exactly where it came from; everything
 * that is not words (styles, timings, code, markup) never goes near the
 * model.
 *
 * The model is given the glossary terms the text uses, and the result is
 * checked against them: a term not translated as agreed, or a number, an
 * address or a link that did not survive, is reported with the file rather
 * than discovered by its reader.
 */
package translate

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/glossary"
)

// Engine asks a model: system is the instruction, user the text. It is the
// brain's own, so the call goes through the router and its privacy rules.
type Engine func(ctx context.Context, system, user string) (string, error)

// Languages are the ones named in instructions and offered on the page.
// Anything else is passed to the model by its code.
var Languages = []struct {
	Code, Name, NameBG string
}{
	{"bg", "Bulgarian", "български"}, {"en", "English", "английски"}, {"de", "German", "немски"},
	{"fr", "French", "френски"}, {"es", "Spanish", "испански"}, {"it", "Italian", "италиански"},
	{"pt", "Portuguese", "португалски"}, {"nl", "Dutch", "нидерландски"}, {"pl", "Polish", "полски"},
	{"ro", "Romanian", "румънски"}, {"el", "Greek", "гръцки"}, {"tr", "Turkish", "турски"},
	{"ru", "Russian", "руски"}, {"uk", "Ukrainian", "украински"}, {"sr", "Serbian", "сръбски"},
	{"mk", "Macedonian", "македонски"}, {"cs", "Czech", "чешки"},
}

// Name is the English name of a language code, or the code itself.
func Name(code string) string {
	for _, l := range Languages {
		if l.Code == code {
			return l.Name
		}
	}

	return code
}

// Result is a translation and what it missed.
type Result struct {
	Text     string          `json:"text"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Segments int             `json:"segments"`
	Terms    []glossary.Term `json:"terms_used"`
	Missed   []glossary.Miss `json:"missed"`
}

// Translator translates pieces of text with an engine and a glossary.
type Translator struct {
	Engine   Engine
	Glossary []glossary.Term

	// Batch is roughly how many characters go to the model at once. Zero is
	// 1,500: enough context to translate well, little enough that a small
	// model keeps its numbering straight.
	Batch int

	// Progress, if set, is told how many pieces of how many are done.
	Progress func(done, of int)
}

// ErrNoEngine is a translation asked for with no model to do it.
var ErrNoEngine = errors.New("no language model is available to translate with")

// Text translates one text: its paragraphs are pieces, so a long text is
// sent in parts and still comes back as one.
func (tr Translator) Text(ctx context.Context, text, from, to string) (Result, error) {
	pieces := strings.Split(text, "\n\n")

	out, res, err := tr.Pieces(ctx, pieces, from, to)
	if err != nil {
		return res, err
	}

	res.Text = strings.Join(out, "\n\n")

	return res, nil
}

/*
 * Pieces translates each piece and returns them in order. Pieces with no
 * letters in them — numbers, blank lines, a row of dashes — are returned as
 * they were without asking anybody.
 */
func (tr Translator) Pieces(ctx context.Context, pieces []string, from, to string) ([]string, Result, error) {
	res := Result{From: from, To: to}

	if tr.Engine == nil {
		return nil, res, ErrNoEngine
	}

	if !glossary.Code(to) {
		return nil, res, fmt.Errorf("%q is not a language code like en or bg", to)
	}

	if from == "auto" {
		from = ""
	}

	if from != "" && from == to {
		return nil, res, errors.New("the text is already in that language")
	}

	terms := glossary.For(tr.Glossary, from, to)

	out := make([]string, len(pieces))
	copy(out, pieces)

	var todo []int

	for i, p := range pieces {
		if hasLetters(p) {
			todo = append(todo, i)
		}
	}

	res.Segments = len(todo)

	size := tr.Batch
	if size <= 0 {
		size = 1500
	}

	done := 0

	for start := 0; start < len(todo); {
		end, chars := start, 0

		for end < len(todo) && (end == start || chars+len(pieces[todo[end]]) <= size) {
			chars += len(pieces[todo[end]])
			end++
		}

		batch := todo[start:end]

		texts := make([]string, len(batch))
		for k, i := range batch {
			texts[k] = pieces[i]
		}

		got, err := tr.batch(ctx, texts, from, to, terms)
		if err != nil {
			return nil, res, err
		}

		for k, i := range batch {
			out[i] = keepEdges(pieces[i], got[k])
		}

		done += len(batch)
		if tr.Progress != nil {
			tr.Progress(done, len(todo))
		}

		start = end
	}

	whole, translated := strings.Join(pieces, "\n"), strings.Join(out, "\n")

	res.Terms = glossary.Used(terms, whole)
	res.Missed = glossary.Check(whole, translated, terms)

	if res.Terms == nil {
		res.Terms = []glossary.Term{}
	}

	if res.Missed == nil {
		res.Missed = []glossary.Miss{}
	}

	return out, res, nil
}

/*
 * batch sends several pieces at once, each behind a marker the model is told
 * to keep. When what comes back does not have every marker exactly once —
 * small models merge and drop them — each piece is sent again on its own,
 * which costs time and never loses a paragraph.
 */
func (tr Translator) batch(ctx context.Context, texts []string, from, to string, terms []glossary.Term) ([]string, error) {
	if len(texts) == 1 {
		got, err := tr.one(ctx, texts[0], from, to, terms)

		return []string{got}, err
	}

	var b strings.Builder

	for i, t := range texts {
		fmt.Fprintf(&b, "⟦%d⟧\n%s\n", i+1, t)
	}

	system := instruction(from, to, glossary.Used(terms, strings.Join(texts, "\n")), true)

	reply, err := tr.Engine(ctx, system, b.String())
	if err != nil {
		return nil, err
	}

	if got, ok := split(reply, len(texts)); ok {
		return got, nil
	}

	out := make([]string, len(texts))

	for i, t := range texts {
		if out[i], err = tr.one(ctx, t, from, to, terms); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func (tr Translator) one(ctx context.Context, text, from, to string, terms []glossary.Term) (string, error) {
	reply, err := tr.Engine(ctx, instruction(from, to, glossary.Used(terms, text), false), text)
	if err != nil {
		return "", err
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", errors.New("the model gave back no translation")
	}

	// A marker the model added on its own is not part of the text.
	if got, ok := split(reply, 1); ok {
		return got[0], nil
	}

	return reply, nil
}

var marker = regexp.MustCompile(`(?m)^\s*⟦(\d+)⟧\s*\n?`)

// split cuts a marked reply into n pieces; ok is false unless every marker
// from 1 to n is there exactly once and in order.
func split(reply string, n int) ([]string, bool) {
	at := marker.FindAllStringSubmatchIndex(reply, -1)
	if len(at) != n {
		return nil, false
	}

	out := make([]string, n)

	for k, m := range at {
		number, _ := strconv.Atoi(reply[m[2]:m[3]])
		if number != k+1 {
			return nil, false
		}

		end := len(reply)
		if k+1 < len(at) {
			end = at[k+1][0]
		}

		out[k] = strings.TrimSpace(reply[m[1]:end])

		if out[k] == "" {
			return nil, false
		}
	}

	return out, true
}

// instruction is what the model is told, in English, which every model
// follows best; the terms are given in their own languages.
func instruction(from, to string, terms []glossary.Term, marked bool) string {
	var b strings.Builder

	source := "the source language"
	if from != "" {
		source = Name(from)
	}

	fmt.Fprintf(&b, "You are a professional translator. Translate from %s into %s.\n", source, Name(to))
	b.WriteString("Translate the meaning naturally, as a native speaker would write it, in the same register and tone.\n")
	b.WriteString("Keep names, numbers, dates, amounts, e-mail addresses, links, code, placeholders such as {name} or %s, and Markdown or HTML markup exactly as they are.\n")

	if to == "bg" {
		b.WriteString("Follow Bulgarian spelling and punctuation: в/във, с/със, full and short article, commas before че, който, защото, но; quotation marks „ “.\n")
	}

	if len(terms) > 0 {
		b.WriteString("Always use these agreed translations (the owner's glossary); a term mapped to itself is never translated:\n")

		for _, t := range terms {
			fmt.Fprintf(&b, "- %s → %s", t.Source, t.Target)

			if t.Note != "" {
				fmt.Fprintf(&b, " (%s)", t.Note)
			}

			b.WriteString("\n")
		}
	}

	if marked {
		b.WriteString("The text is in numbered parts, each starting with a marker like ⟦1⟧ on its own line. " +
			"Translate each part and keep every marker, in the same order, on its own line before its translation.\n")
	}

	b.WriteString("Reply with the translation only: no notes, no explanations, no quotation marks round it.")

	return b.String()
}

func hasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}

	return false
}

// keepEdges gives a translation the leading and trailing space of the
// piece it replaces, which is layout rather than words.
func keepEdges(original, translated string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}

	lead := original[:strings.Index(original, trimmed)]
	tail := original[len(lead)+len(trimmed):]

	return lead + strings.TrimSpace(translated) + tail
}
