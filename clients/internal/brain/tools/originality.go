package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/originality"
)

/*
 * What the originality specialists check with (knowledge/guides/
 * originality.md): a name against the registers and stores, a game's one
 * sentence against the closest titles, pictures against references, melodies
 * against well-known themes, and a report of it all as a file to keep.
 *
 * All of them only look. The two that search say so (LeavesTheMachine), so
 * they disappear while privacy keeps everything on this machine; the audit
 * then still runs and says which checks could not be made.
 */

func searcher(ctx context.Context, s WebSearch) originality.Search {
	return func(q string) ([]originality.Hit, error) {
		found, err := s.Find(ctx, q)
		if err != nil {
			return nil, err
		}

		out := make([]originality.Hit, len(found))
		for i, r := range found {
			out[i] = originality.Hit{Title: r.Title, URL: r.URL, Snippet: r.Snippet}
		}

		return out, nil
	}
}

func findingsText(f []originality.Finding, most int) string {
	var b strings.Builder

	for i, x := range f {
		if i == most {
			fmt.Fprintf(&b, "… and %d more in the report.\n", len(f)-most)

			break
		}

		fmt.Fprintf(&b, "- [%s] %s · %s: %s", x.Risk, x.Item, x.Aspect, x.Finding)

		if x.Action != "" {
			b.WriteString(" → " + x.Action)
		}

		b.WriteString("\n")
	}

	return b.String()
}

// CheckName searches a name in trademark registers, stores and the web.
type CheckName struct{ Search WebSearch }

func (CheckName) Name() string           { return "check_name" }
func (CheckName) Risk() Risk             { return Safe }
func (CheckName) LeavesTheMachine() bool { return true }
func (CheckName) Cues() []string {
	return []string{"name", "brand", "trademark", "title", "logo", "име", "марка", "заглавие"}
}

func (CheckName) Description() string {
	return "Check whether a name (product, game, brand, company) is already used or registered: searches " +
		"trademark registers, app and game stores and the web, and returns the close names with a risk " +
		"(high, medium, low) and links to confirm in the official registers."
}

func (CheckName) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"The name to check"}},"required":["name"]}`)
}

func (CheckName) Summarize(raw json.RawMessage) string {
	var a struct{ Name string }
	_ = argsOf(raw, &a)

	return "Search registers and stores for the name: " + a.Name
}

func (t CheckName) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct{ Name string }
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("no name was given")
	}

	return findingsText(originality.CheckName(a.Name, searcher(ctx, t.Search)), 10), nil
}

// ClosestTitles compares a game's one sentence with the nearest known titles.
type ClosestTitles struct{ Search WebSearch }

func (ClosestTitles) Name() string           { return "closest_titles" }
func (ClosestTitles) Risk() Risk             { return Safe }
func (ClosestTitles) LeavesTheMachine() bool { return true }
func (ClosestTitles) Cues() []string {
	return []string{"game", "игра", "rule", "правило", "unique", "уникал"}
}

func (ClosestTitles) Description() string {
	return "Game design uniqueness check: tests the one sentence saying what only this game does, " +
		"and finds the three closest known titles on the web with how much of the idea they share."
}

func (ClosestTitles) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
		"name":{"type":"string","description":"The game's name"},
		"sentence":{"type":"string","description":"One sentence: what only this game does"},
		"genre":{"type":"string","description":"Its genre, e.g. puzzle, tower defence"}},
		"required":["name","sentence"]}`)
}

func (ClosestTitles) Summarize(raw json.RawMessage) string {
	var a struct{ Name, Sentence string }
	_ = argsOf(raw, &a)

	return "Search the web for the games closest to " + a.Name + ": " + a.Sentence
}

func (t ClosestTitles) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct{ Name, Sentence, Genre string }
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	f := originality.CheckGame(a.Name, a.Sentence, a.Genre, searcher(ctx, t.Search))
	if len(f) == 0 {
		return "The sentence says what only this game does and no close title was found.", nil
	}

	return findingsText(f, 10), nil
}

// ComparePictures compares pictures with references by perceptual hash.
type ComparePictures struct{}

func (ComparePictures) Name() string { return "compare_pictures" }
func (ComparePictures) Risk() Risk   { return Safe }
func (ComparePictures) Cues() []string {
	return []string{"picture", "image", "logo", "art", "character", "картин", "изображ", "лого", "персонаж"}
}

func (ComparePictures) Description() string {
	return "Compare pictures (PNG, JPEG, GIF; files or folders) with reference pictures they must not look like, " +
		"by perceptual hash: finds resized, recompressed, recoloured and mirrored copies. Works offline. " +
		"Also says how to search the main picture by image on the web."
}

func (ComparePictures) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
		"pictures":{"type":"array","items":{"type":"string"},"description":"Our pictures or folders"},
		"references":{"type":"array","items":{"type":"string"},"description":"Reference pictures or folders"}},
		"required":["pictures","references"]}`)
}

func (ComparePictures) Summarize(raw json.RawMessage) string {
	var a struct{ Pictures, References []string }
	_ = argsOf(raw, &a)

	return fmt.Sprintf("Compare %s with %s", strings.Join(a.Pictures, ", "), strings.Join(a.References, ", "))
}

func (ComparePictures) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct{ Pictures, References []string }
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	ours, refs := originality.Pictures(a.Pictures...), originality.Pictures(a.References...)
	if len(ours) == 0 || len(refs) == 0 {
		return "", fmt.Errorf("found %d pictures and %d references to compare (PNG, JPEG or GIF)", len(ours), len(refs))
	}

	matches, problems := originality.ComparePictures(ours, refs)

	var b strings.Builder

	for _, m := range matches {
		how := ""
		if m.Mirrored {
			how = ", mirrored"
		}

		fmt.Fprintf(&b, "- [%s] %s looks like %s (%d of 64 differ%s)\n", m.Risk, m.Ours, m.Reference, m.Distance, how)
	}

	if len(matches) == 0 {
		fmt.Fprintf(&b, "%d pictures compared with %d references: none alike.\n", len(ours), len(refs))
	}

	for _, p := range problems {
		b.WriteString("- not read: " + p + "\n")
	}

	b.WriteString(originality.ReverseSearch(ours[0]) + "\n")

	return b.String(), nil
}

// CompareMelody looks for well-known themes in MIDI files and code.
type CompareMelody struct{}

func (CompareMelody) Name() string { return "compare_melody" }
func (CompareMelody) Risk() Risk   { return Safe }
func (CompareMelody) Cues() []string {
	return []string{"melody", "music", "tune", "jingle", "song", "midi", "мелоди", "музик", "песен", "джингъл"}
}

func (CompareMelody) Description() string {
	return "Check melodies for well-known tunes: reads MIDI files and the notes written in code " +
		"(note names, MIDI numbers or frequencies) and compares their intervals, in any key, with a bundled " +
		"set of famous public-domain themes. Works offline."
}

func (CompareMelody) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
		"files":{"type":"array","items":{"type":"string"},"description":"MIDI files, code files or folders"}},
		"required":["files"]}`)
}

func (CompareMelody) Summarize(raw json.RawMessage) string {
	var a struct{ Files []string }
	_ = argsOf(raw, &a)

	return "Compare the melodies in " + strings.Join(a.Files, ", ") + " with known themes"
}

func (CompareMelody) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct{ Files []string }
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	var out []originality.Finding

	for _, f := range a.Files {
		it, err := originality.Items(f)
		if err != nil {
			// A single file.
			out = append(out, originality.CheckMelodies(filepath.Base(f), filepath.Dir(f), []string{f})...)

			continue
		}

		for _, item := range it {
			out = append(out, originality.CheckMelodies(item.Name, item.Path, item.Melodies)...)
		}
	}

	if len(out) == 0 {
		return "No melodies were found in those files.", nil
	}

	return findingsText(out, 20), nil
}

// reportDir is where audit reports are kept: inside the brain, so they go
// wherever it goes.
func reportDir() string { return filepath.Join(Root, "reports", "originality") }

// WriteOriginalityReport saves findings as a report file.
type WriteOriginalityReport struct{}

func (WriteOriginalityReport) Name() string { return "write_originality_report" }

// Safe: it only ever creates a new file in the brain's own reports folder,
// never replaces one and never touches the user's files.
func (WriteOriginalityReport) Risk() Risk { return Safe }

func (WriteOriginalityReport) Description() string {
	return "Save originality findings as a report (a Markdown table and a CSV sheet: item | aspect | finding | " +
		"risk | action) in the brain's reports folder, to download or keep. Risk is high, medium, low or none."
}

func (WriteOriginalityReport) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
		"title":{"type":"string"},
		"findings":{"type":"array","items":{"type":"object","properties":{
			"item":{"type":"string"},"aspect":{"type":"string"},"finding":{"type":"string"},
			"risk":{"type":"string","enum":["high","medium","low","none"]},"action":{"type":"string"},
			"fixed":{"type":"string","description":"What was changed, if it was fixed"}},
			"required":["item","aspect","finding","risk"]}}},
		"required":["findings"]}`)
}

func (WriteOriginalityReport) Summarize(raw json.RawMessage) string {
	var a struct {
		Title    string
		Findings []originality.Finding
	}
	_ = argsOf(raw, &a)

	return fmt.Sprintf("Save an originality report with %d findings", len(a.Findings))
}

func (WriteOriginalityReport) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Title    string
		Findings []originality.Finding
	}
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if a.Title == "" {
		a.Title = "Originality report"
	}

	md, sheet, err := originality.Write(reportDir(), a.Title, a.Findings, time.Now())
	if err != nil {
		return "", err
	}

	return reportSaved(md, sheet, a.Findings), nil
}

func reportSaved(md, sheet string, f []originality.Finding) string {
	return fmt.Sprintf("Report saved: %s and %s (download: /api/reports/%s). %s.",
		md, sheet, filepath.Base(md), originality.Summary(f))
}

// AuditOriginality audits every item of a folder or catalogue.
type AuditOriginality struct {
	Search WebSearch
	// Web says whether looking online is allowed now.
	Web func() bool
}

func (AuditOriginality) Name() string { return "audit_originality" }

// Safe: it reads the folder and creates a new report in the brain's own
// reports folder; it changes nothing it audits.
func (AuditOriginality) Risk() Risk { return Safe }
func (AuditOriginality) Cues() []string {
	return []string{"audit", "catalogue", "catalog", "originality", "одит", "каталог", "оригинал"}
}

func (AuditOriginality) Description() string {
	return "Originality audit of a whole folder or catalogue: each item (each subfolder) is checked only for " +
		"what it has — its name against registers and stores, a game's core rule against the closest titles, " +
		"pictures against reference pictures, melodies against known themes, placeholder text — items in " +
		"parallel. Saves the report as a file and returns the findings. Changes nothing it audits."
}

func (AuditOriginality) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
		"folder":{"type":"string","description":"The folder or catalogue to audit"},
		"references":{"type":"array","items":{"type":"string"},"description":"Pictures or folders our pictures must not look like"}},
		"required":["folder"]}`)
}

func (AuditOriginality) Summarize(raw json.RawMessage) string {
	var a struct{ Folder string }
	_ = argsOf(raw, &a)

	return "Audit the originality of everything in " + a.Folder
}

func (t AuditOriginality) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Folder     string
		References []string
	}
	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	opt := originality.Options{Folder: a.Folder, References: a.References}
	if t.Web != nil && t.Web() {
		opt.Search = searcher(ctx, t.Search)
	}

	f, err := originality.Audit(ctx, opt)
	if err != nil {
		return "", err
	}

	md, sheet, err := originality.Write(reportDir(), "Originality audit: "+filepath.Base(a.Folder), f, time.Now())
	if err != nil {
		return "", err
	}

	return reportSaved(md, sheet, f) + "\n" + findingsText(f, 25), nil
}
