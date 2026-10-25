package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/glossary"
	"pn-scripts-assistant/internal/brain/translate"
)

/*
 * Translation with the owner's glossary.
 *
 * Four tools, split by what they change. Translating a text and reading the
 * glossary change nothing. Translating a file writes a new file beside it —
 * never over anything — and changing the glossary changes how every later
 * translation is made, so both are Mutating and are shown before they happen.
 */

// Translating is what the translation tools need from the brain: a
// translator with the model and the glossary as they are now, and the
// brain's folder, where the glossary lives.
type Translating struct {
	Translator func() translate.Translator
	Root       string
}

var translationCues = []string{
	"translat", "glossary", "terminolog", "term ", "terms", "in english", "in bulgarian", "into ",
	"превод", "преведи", "речник", "термин", "на английски", "на български", "на немски",
}

// Translate translates a text.
type Translate struct{ With Translating }

func (Translate) Name() string   { return "translate" }
func (Translate) Risk() Risk     { return Safe }
func (Translate) Cues() []string { return translationCues }
func (Translate) Description() string {
	return "Translate a text into another language with the model, using the owner's glossary " +
		"(agreed translations of terms) and checking afterwards that every glossary term, number, " +
		"address and link survived. Give the text and the target language code (en, bg, de, fr…). " +
		"For a whole file use translate_document instead."
}

func (Translate) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The text to translate"},
			"to": {"type": "string", "description": "Target language code: en, bg, de, fr, es, it, ru, uk…"},
			"from": {"type": "string", "description": "Source language code, or auto"}
		},
		"required": ["text", "to"]
	}`)
}

type translateArgs struct {
	Text string `json:"text"`
	Path string `json:"path"`
	Out  string `json:"out"`
	To   string `json:"to"`
	From string `json:"from"`
}

func (Translate) Summarize(raw json.RawMessage) string {
	var a translateArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Translate a text of %d words into %s", len(strings.Fields(a.Text)), translate.Name(a.To))
}

func (t Translate) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a translateArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if strings.TrimSpace(a.Text) == "" {
		return "", fmt.Errorf("there is no text to translate")
	}

	if t.With.Translator == nil {
		return "", translate.ErrNoEngine
	}

	res, err := t.With.Translator().Text(ctx, a.Text, strings.ToLower(a.From), strings.ToLower(a.To))
	if err != nil {
		return "", err
	}

	return res.Text + translationNotes(res), nil
}

// TranslateDocument translates a file into a new file with the same layout.
type TranslateDocument struct{ With Translating }

func (TranslateDocument) Name() string { return "translate_document" }
func (TranslateDocument) Risk() Risk   { return Mutating }
func (TranslateDocument) Cues() []string {
	return append([]string{"document", "file", "subtitle", "документ", "файл", "субтитр"}, translationCues...)
}

func (TranslateDocument) Description() string {
	return "Translate a file into a new file beside it, keeping its layout: plain text (.txt), Markdown " +
		"(.md, code left alone), subtitles (.srt, .vtt, timings kept), Word (.docx) and OpenDocument (.odt) " +
		"with their styles, tables, headers and pictures. Uses the owner's glossary and reports any term, " +
		"number or link the translation did not keep. Never replaces a file: the new one is named like " +
		"report.bg.docx unless out is given. A long document takes minutes; offer do_in_background."
}

func (TranslateDocument) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path of the file to translate"},
			"to": {"type": "string", "description": "Target language code: en, bg, de, fr…"},
			"from": {"type": "string", "description": "Source language code, or auto"},
			"out": {"type": "string", "description": "Optional absolute path for the translation; must not exist"}
		},
		"required": ["path", "to"]
	}`)
}

func (TranslateDocument) Summarize(raw json.RawMessage) string {
	var a translateArgs

	_ = json.Unmarshal(raw, &a)

	out := a.Out
	if out == "" && a.Path != "" {
		out = translate.Beside(a.Path, strings.ToLower(a.To))
	}

	return fmt.Sprintf("Translate %s into %s and write the translation to %s (a new file; the original is not changed)",
		a.Path, translate.Name(strings.ToLower(a.To)), out)
}

func (t TranslateDocument) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a translateArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if !filepath.IsAbs(a.Path) || (a.Out != "" && !filepath.IsAbs(a.Out)) {
		return "", fmt.Errorf("give absolute paths")
	}

	if err := GuardSensitive(ctx, a.Path); err != nil {
		return "", err
	}

	if t.With.Translator == nil {
		return "", translate.ErrNoEngine
	}

	res, err := t.With.Translator().File(ctx, a.Path, a.Out, strings.ToLower(a.From), strings.ToLower(a.To))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Translated %d parts of %s into %s and wrote %s.%s",
		res.Segments, filepath.Base(a.Path), translate.Name(res.To), res.Text, translationNotes(res)), nil
}

// translationNotes is what a translation used and missed, for the model to
// pass on.
func translationNotes(res translate.Result) string {
	var b strings.Builder

	if len(res.Terms) > 0 {
		names := make([]string, 0, len(res.Terms))
		for _, t := range res.Terms {
			names = append(names, t.Source+" → "+t.Target)
		}

		fmt.Fprintf(&b, "\n\nGlossary terms used: %s.", strings.Join(names, "; "))
	}

	for _, m := range res.Missed {
		switch m.Kind {
		case "term":
			fmt.Fprintf(&b, "\nCheck: %q should be translated as %q and that word is not in the translation.", m.Source, m.Want)
		default:
			fmt.Fprintf(&b, "\nCheck: %q is in the original and not in the translation.", m.Source)
		}
	}

	return b.String()
}

// Glossary shows the glossary and checks a translation against it.
type Glossary struct{ With Translating }

func (Glossary) Name() string   { return "glossary" }
func (Glossary) Risk() Risk     { return Safe }
func (Glossary) Cues() []string { return translationCues }
func (Glossary) Description() string {
	return "Show the owner's glossary — the agreed translation of each term — for one target language or " +
		"all; or, given an original and its translation, check that every glossary term, number, address " +
		"and link in the original was kept."
}

func (Glossary) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"to": {"type": "string", "description": "Only terms for this target language code"},
			"original": {"type": "string", "description": "To check a translation: the original text"},
			"translation": {"type": "string", "description": "To check a translation: the translated text"}
		}
	}`)
}

type glossaryArgs struct {
	To          string `json:"to"`
	From        string `json:"from"`
	Original    string `json:"original"`
	Translation string `json:"translation"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	Note        string `json:"note"`
	Remove      bool   `json:"remove"`
}

func (Glossary) Summarize(raw json.RawMessage) string {
	var a glossaryArgs

	_ = json.Unmarshal(raw, &a)

	if a.Original != "" {
		return "Check a translation against the glossary"
	}

	return "Show the glossary"
}

func (g Glossary) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a glossaryArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	terms, err := glossary.Load(g.With.Root)
	if err != nil {
		return "", err
	}

	to := strings.ToLower(a.To)

	if a.Original != "" || a.Translation != "" {
		if to == "" {
			return "", fmt.Errorf("say which language the translation is in (to)")
		}

		misses := glossary.Check(a.Original, a.Translation, glossary.For(terms, strings.ToLower(a.From), to))
		if len(misses) == 0 {
			return "Every glossary term, number, address and link in the original is in the translation.", nil
		}

		return strings.TrimSpace(translationNotes(translate.Result{Missed: misses})), nil
	}

	var b strings.Builder

	n := 0

	for _, t := range terms {
		if to != "" && t.To != to {
			continue
		}

		n++

		from := t.From
		if from == "" {
			from = "any"
		}

		fmt.Fprintf(&b, "- %s → %s (%s→%s)", t.Source, t.Target, from, t.To)

		if t.Note != "" {
			fmt.Fprintf(&b, " — %s", t.Note)
		}

		b.WriteString("\n")
	}

	if n == 0 {
		return "The glossary has no terms yet" + map[bool]string{true: " for " + translate.Name(to), false: ""}[to != ""] +
			". Terms are added with change_glossary or on the Translation page.", nil
	}

	return fmt.Sprintf("%d terms:\n%s", n, b.String()), nil
}

// ChangeGlossary adds a term to the glossary or removes one.
type ChangeGlossary struct{ With Translating }

func (ChangeGlossary) Name() string   { return "change_glossary" }
func (ChangeGlossary) Risk() Risk     { return Mutating }
func (ChangeGlossary) Cues() []string { return translationCues }
func (ChangeGlossary) Description() string {
	return "Add a term to the owner's glossary — how a word or phrase is always translated into a " +
		"language — or remove one with remove=true. A target equal to the source means the term is never " +
		"translated (a name, a brand). Use when the owner says how something should be translated."
}

func (ChangeGlossary) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"source": {"type": "string", "description": "The word or phrase in the original language"},
			"target": {"type": "string", "description": "How it is always translated"},
			"to": {"type": "string", "description": "Target language code: en, bg, de…"},
			"from": {"type": "string", "description": "Source language code, or empty for any"},
			"note": {"type": "string", "description": "Optional note: where it applies, why"},
			"remove": {"type": "boolean", "description": "True to remove the term instead"}
		},
		"required": ["source", "to"]
	}`)
}

func (ChangeGlossary) Summarize(raw json.RawMessage) string {
	var a glossaryArgs

	_ = json.Unmarshal(raw, &a)

	if a.Remove {
		return fmt.Sprintf("Remove %q from the glossary for %s", a.Source, translate.Name(strings.ToLower(a.To)))
	}

	return fmt.Sprintf("Add to the glossary: %q is always translated into %s as %q",
		a.Source, translate.Name(strings.ToLower(a.To)), a.Target)
}

func (c ChangeGlossary) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a glossaryArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	term := glossary.Term{Source: a.Source, Target: a.Target, From: a.From, To: a.To, Note: a.Note}

	if a.Remove {
		found, err := glossary.Remove(c.With.Root, term)
		if err != nil {
			return "", err
		}

		if !found {
			return fmt.Sprintf("%q was not in the glossary for %s.", a.Source, translate.Name(strings.ToLower(a.To))), nil
		}

		return fmt.Sprintf("Removed %q from the glossary.", a.Source), nil
	}

	saved, replaced, err := glossary.Add(c.With.Root, term)
	if err != nil {
		return "", err
	}

	verb := "Added"
	if replaced {
		verb = "Changed"
	}

	return fmt.Sprintf("%s: %q → %q for %s.", verb, saved.Source, saved.Target, translate.Name(saved.To)), nil
}
