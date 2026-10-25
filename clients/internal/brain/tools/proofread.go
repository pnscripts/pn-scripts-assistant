package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/proof"
)

/*
 * Proofreading and comparing, in English and Bulgarian.
 *
 * Both only read. The slips are found by the proof package — rules decided
 * from the text, not a model's opinion — so what comes back is a list
 * somebody can check line by line, and the corrected text has only the sure
 * corrections in it. Putting the corrections into a file is edit_document's
 * job and goes through its approval like any other change.
 */

// Proofread finds the slips in a text or a document.
type Proofread struct{}

func (Proofread) Name() string { return "proofread" }
func (Proofread) Risk() Risk   { return Safe }

func (Proofread) Cues() []string {
	return []string{
		"proofread", "proof-read", "spelling", "grammar", "typo", "mistake", "punctuation", "correct",
		"check my", "check this", "tone", "formal", "polite", "concise", "shorter", "rewrite", "edit",
		"правопис", "граматик", "пунктуац", "грешк", "провери", "коригир", "поправ", "редактир",
		"тон", "официал", "учтив", "по-кратк", "пренапиш",
	}
}

func (Proofread) Description() string {
	return "Proofread English or Bulgarian text: finds spelling, punctuation and grammar slips that " +
		"rules can prove (doubled words, spacing, Latin letters inside Cyrillic words, в/във and с/със, " +
		"the full article after a preposition, a missing comma before че/защото/който, a/an, " +
		"could of) and returns each with a reason, plus the text with only the sure corrections made. " +
		"With a tone (formal, friendly, concise, persuasive, simple, polite) it also checks sentence " +
		"length and words that do not fit that tone; with max_words, the length. Give text, or the " +
		"absolute path of a .txt, .md, .docx, .odt or .pdf file. It changes nothing: to put the " +
		"corrections into a file use edit_document."
}

func (Proofread) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The text to proofread"},
			"path": {"type": "string", "description": "Or the absolute path of a document to proofread"},
			"language": {"type": "string", "enum": ["auto", "en", "bg"], "description": "Language of the text; auto guesses"},
			"tone": {"type": "string", "enum": ["", "formal", "friendly", "concise", "persuasive", "simple", "polite"], "description": "Optional tone to check against"},
			"max_words": {"type": "integer", "description": "Optional most words the text should have"}
		}
	}`)
}

type proofArgs struct {
	Text     string `json:"text"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Tone     string `json:"tone"`
	MaxWords int    `json:"max_words"`
}

func (Proofread) Summarize(raw json.RawMessage) string {
	var a proofArgs

	_ = json.Unmarshal(raw, &a)

	if a.Path != "" {
		return "Proofread " + a.Path
	}

	return fmt.Sprintf("Proofread a text of %d words", len(strings.Fields(a.Text)))
}

func (Proofread) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a proofArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	text, err := textToCheck(ctx, a.Text, a.Path)
	if err != nil {
		return "", err
	}

	lang := a.Language
	if lang != "en" && lang != "bg" {
		lang = proof.Language(text)
	}

	report := ProofReport(text, lang, a.Tone, a.MaxWords)

	var b strings.Builder

	fmt.Fprintf(&b, "Language: %s. %d words, %d sentences (longest %d words), about %d min to read.\n",
		map[string]string{"en": "English", "bg": "Bulgarian"}[lang],
		report.Stats.Words, report.Stats.Sentences, report.Stats.Longest, report.Stats.Minutes)

	if len(report.Findings) == 0 && len(report.Tone) == 0 {
		b.WriteString("\nNo slips found by the rules. Meaning, style and facts are not checked by this.")

		return b.String(), nil
	}

	write := func(title string, list []proof.Finding) {
		if len(list) == 0 {
			return
		}

		fmt.Fprintf(&b, "\n%s:\n", title)

		for i, f := range list {
			why := f.Why
			if lang == "bg" {
				why = f.WhyBG
			}

			line := fmt.Sprintf("%d. %q", i+1, first(flat(f.Found), 80))
			if f.Suggest != "" || f.Rule == "repeated_word" || f.Rule == "space_before_punctuation" {
				line += fmt.Sprintf(" → %q", f.Suggest)
			}

			// A slip that is only space is shown with the words round it.
			if strings.TrimSpace(f.Found) == "" && f.Length > 0 {
				line += " in \"" + around(text, f.At, f.Length) + "\""
			}

			sure := ""
			if !f.Sure {
				sure = " (check it)"
			}

			fmt.Fprintf(&b, "%s — %s%s\n", line, why, sure)
		}
	}

	write("Slips", report.Findings)
	write("Tone and length", report.Tone)

	if report.Corrected != text {
		fmt.Fprintf(&b, "\nThe text with the sure corrections made:\n%s\n", first(report.Corrected, MaxReadBytes/2))
	}

	return b.String(), nil
}

// Report is everything a proofreading pass found.
type Report struct {
	Language  string          `json:"language"`
	Findings  []proof.Finding `json:"findings"`
	Tone      []proof.Finding `json:"tone"`
	Stats     proof.Stats     `json:"stats"`
	Corrected string          `json:"corrected"`
}

// ProofReport proofreads text, and checks it against a tone and a length
// when they are given. Shared by the tool and the Writing page.
func ProofReport(text, lang, tone string, most int) Report {
	if lang != "en" && lang != "bg" {
		lang = proof.Language(text)
	}

	r := Report{Language: lang, Findings: proof.Check(text, lang), Stats: proof.Measure(text)}

	if r.Findings == nil {
		r.Findings = []proof.Finding{}
	}

	r.Corrected = proof.Apply(text, r.Findings)

	preset, ok := proof.ToneNamed(tone)
	if ok || most > 0 {
		r.Tone = proof.CheckTone(text, lang, preset, most)
	}

	if r.Tone == nil {
		r.Tone = []proof.Finding{}
	}

	return r
}

// CompareVersions shows what changed between two versions of a text.
type CompareVersions struct{}

func (CompareVersions) Name() string { return "compare_versions" }
func (CompareVersions) Risk() Risk   { return Safe }

func (CompareVersions) Cues() []string {
	return []string{
		"compare", "what changed", "what did you change", "difference", "diff", "versions", "before and after",
		"сравни", "какво промен", "какво си промен", "разлик", "версии", "compare_versions",
	}
}

func (CompareVersions) Description() string {
	return "Compare two versions of a text word by word and show exactly what was taken out " +
		"[-like this-] and put in {+like this+}, with counts. Give the two texts, or the absolute " +
		"paths of two files (.txt, .md, .docx, .odt, .pdf). Use it to answer \"what did you change\" " +
		"after a rewrite instead of describing the changes from memory."
}

func (CompareVersions) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"before": {"type": "string", "description": "The earlier version"},
			"after": {"type": "string", "description": "The later version"},
			"before_path": {"type": "string", "description": "Or the absolute path of the earlier file"},
			"after_path": {"type": "string", "description": "Or the absolute path of the later file"}
		}
	}`)
}

type compareArgs struct {
	Before     string `json:"before"`
	After      string `json:"after"`
	BeforePath string `json:"before_path"`
	AfterPath  string `json:"after_path"`
}

func (CompareVersions) Summarize(raw json.RawMessage) string {
	var a compareArgs

	_ = json.Unmarshal(raw, &a)

	if a.BeforePath != "" || a.AfterPath != "" {
		return "Compare " + a.BeforePath + " with " + a.AfterPath
	}

	return "Compare two versions of a text"
}

func (CompareVersions) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a compareArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	before, err := textToCheck(ctx, a.Before, a.BeforePath)
	if err != nil {
		return "", fmt.Errorf("the earlier version: %w", err)
	}

	after, err := textToCheck(ctx, a.After, a.AfterPath)
	if err != nil {
		return "", fmt.Errorf("the later version: %w", err)
	}

	d := proof.Compare(before, after)

	if !d.Changed() {
		return "The two versions are the same.", nil
	}

	return fmt.Sprintf("%d words taken out, %d put in, %d unchanged.\n"+
		"Taken out is marked [-like this-], put in {+like this+}:\n\n%s",
		d.Removed, d.Added, d.Same, first(d.Marked(), MaxReadBytes)), nil
}

/*
 * textToCheck is the text given, or the text of the file at path: plain text
 * read as it is, anything else through read_document, which knows the
 * formats.
 */
func textToCheck(ctx context.Context, text, path string) (string, error) {
	if path == "" {
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("give the text, or the path of a file")
		}

		return text, nil
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("give an absolute path, not %q", path)
	}

	if err := GuardSensitive(ctx, path); err != nil {
		return "", err
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".srt", ".vtt", ".html", ".htm", ".csv", "":
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}

		if info.Size() > 4*MaxReadBytes {
			return "", fmt.Errorf("%s is %d KB, too long to check in one go", filepath.Base(path), info.Size()>>10)
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}

		return string(body), nil
	}

	args, _ := json.Marshal(map[string]string{"path": path})

	return ReadDocument{}.Execute(ctx, args)
}

func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

// around is the word either side of a span, on one line.
func around(text string, at, length int) string {
	start, end := at, at+length

	for start > 0 && text[start-1] == ' ' {
		start--
	}

	for start > 0 && !isBreak(text[start-1]) {
		start--
	}

	for end < len(text) && text[end] == ' ' {
		end++
	}

	for end < len(text) && !isBreak(text[end]) {
		end++
	}

	return flat(text[start:end])
}

func isBreak(b byte) bool { return b == ' ' || b == '\n' || b == '\t' }
