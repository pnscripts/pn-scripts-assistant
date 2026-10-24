package originality

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

/*
 * The catalogue audit: every item in a folder, each checked only for the
 * aspects it has — a name always, the look where there are pictures and
 * references, the sound where there are melodies, the game rule where it is a
 * game — items side by side.
 *
 * It reports; it does not change what it audits. These are the user's own
 * works, and changing them is theirs to decide. (What the assistant made in a
 * job of its own it fixes itself — see tasks.)
 */

// Options say what to audit and with what.
type Options struct {
	Folder string
	// References are pictures ours must not look like (files or folders).
	References []string
	// Search is nil when the web may not be used: the name and title checks
	// then say so instead of passing.
	Search Search
	// Workers is how many items are audited at once (default 4).
	Workers int
}

// Item is one thing in the catalogue.
type Item struct {
	Name, Path, Pitch, Genre string
	Game                     bool
	Pictures, Melodies, Text []string
}

var codeExt = map[string]bool{".js": true, ".mjs": true, ".ts": true, ".gd": true, ".py": true, ".cs": true,
	".go": true, ".lua": true, ".json": true, ".txt": true, ".html": true, ".abc": true}

// Items are the things in a folder: each subfolder, or the folder itself
// when it has none.
func Items(folder string) ([]Item, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	var items []Item

	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "node_modules" {
			items = append(items, describe(filepath.Join(folder, e.Name())))
		}
	}

	if len(items) == 0 {
		items = append(items, describe(folder))
	}

	return items, nil
}

var godotName = regexp.MustCompile(`(?m)^config/name="([^"]+)"`)

func describe(path string) Item {
	it := Item{Name: filepath.Base(path), Path: path}

	var meta struct {
		Name, Title, DisplayName, Pitch, Tagline, Description, Genre string
	}

	for _, f := range []string{"listing.json", "package.json", "manifest.json"} {
		if data, err := os.ReadFile(filepath.Join(path, f)); err == nil && json.Unmarshal(data, &meta) == nil {
			break
		}
	}

	switch {
	case meta.DisplayName != "":
		it.Name = meta.DisplayName
	case meta.Title != "":
		it.Name = meta.Title
	case meta.Name != "":
		it.Name = meta.Name
	}

	it.Pitch = firstNonEmpty(meta.Pitch, meta.Tagline, firstSentence(meta.Description))
	it.Genre = meta.Genre

	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if p != path && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}

			return nil
		}

		ext := strings.ToLower(filepath.Ext(p))
		info, _ := d.Info()

		switch {
		case d.Name() == "project.godot":
			it.Game = true

			if data, err := os.ReadFile(p); err == nil {
				if m := godotName.FindSubmatch(data); m != nil && meta.Name == "" && meta.Title == "" {
					it.Name = string(m[1])
				}
			}
		case IsPicture(p):
			it.Pictures = append(it.Pictures, p)
		case ext == ".mid" || ext == ".midi":
			it.Melodies = append(it.Melodies, p)
		case strings.EqualFold(strings.TrimSuffix(d.Name(), ext), "readme"):
			it.Text = append(it.Text, p)
			fallthrough
		case codeExt[ext] && info != nil && info.Size() < 2<<20:
			it.Melodies = append(it.Melodies, p)

			if ext == ".html" && !it.Game {
				if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), "<canvas") {
					it.Game = true
				}
			}
		}

		return nil
	})

	if it.Pitch == "" {
		for _, t := range it.Text {
			if data, err := os.ReadFile(t); err == nil {
				it.Pitch = firstSentence(firstParagraph(string(data)))

				break
			}
		}
	}

	if !it.Game && strings.Contains(strings.ToLower(it.Pitch+" "+it.Genre), "game") {
		it.Game = true
	}

	return it
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}

	return ""
}

func firstParagraph(text string) string {
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p != "" && !strings.HasPrefix(p, "#") && !strings.HasPrefix(p, "![") && !strings.HasPrefix(p, "[!") {
			return p
		}
	}

	return ""
}

func firstSentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if loc := sentences.FindStringIndex(text); loc != nil {
		return text[:loc[0]+1]
	}

	return text
}

// Audit checks every item in the folder, the aspects each needs only.
func Audit(ctx context.Context, opt Options) ([]Finding, error) {
	items, err := Items(opt.Folder)
	if err != nil {
		return nil, err
	}

	refs := Pictures(opt.References...)

	workers := opt.Workers
	if workers <= 0 {
		workers = 4
	}

	var (
		mu       sync.Mutex
		findings []Finding
		wg       sync.WaitGroup
		queue    = make(chan Item)
		searches = make(chan struct{}, 2) // engines refuse a crowd
	)

	search := opt.Search
	if search != nil {
		search = func(q string) ([]Hit, error) {
			searches <- struct{}{}
			defer func() { <-searches }()

			return opt.Search(q)
		}
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for it := range queue {
				f := CheckItem(ctx, it, refs, search)

				mu.Lock()
				findings = append(findings, f...)
				mu.Unlock()
			}
		}()
	}

	for _, it := range items {
		if ctx.Err() != nil {
			break
		}

		queue <- it
	}

	close(queue)
	wg.Wait()

	Sort(findings)

	return findings, ctx.Err()
}

// CheckItem runs the checks one item needs.
func CheckItem(ctx context.Context, it Item, refs []string, search Search) []Finding {
	var out []Finding

	add := func(aspect, finding string, risk Risk, action string) {
		out = append(out, Finding{Item: it.Name, Aspect: aspect, Finding: finding, Risk: risk, Action: action})
	}

	// The name: always, because everything published has one.
	if ctx.Err() == nil {
		out = append(out, CheckName(it.Name, search)...)
	}

	// The game rule, for games.
	if it.Game && ctx.Err() == nil {
		out = append(out, CheckGame(it.Name, it.Pitch, it.Genre, search)...)
	}

	// The look, where there are pictures.
	if len(it.Pictures) > 0 && ctx.Err() == nil {
		if len(refs) == 0 {
			add("look", fmt.Sprintf("%d pictures, no reference pictures to compare with", len(it.Pictures)), Low,
				ReverseSearch(mainPicture(it.Pictures)))
		} else {
			matches, problems := ComparePictures(it.Pictures, refs)

			for _, m := range matches {
				how := ""
				if m.Mirrored {
					how = " (mirrored)"
				}

				add("look", fmt.Sprintf("%s looks like %s%s — %d of 64 differ", rel(it.Path, m.Ours), filepath.Base(m.Reference), how, m.Distance),
					m.Risk, "redraw it so it no longer resembles the reference")
			}

			for _, p := range problems {
				add("look", p, Low, "convert it to PNG to have it compared")
			}

			if len(matches) == 0 {
				add("look", fmt.Sprintf("%d pictures compared with %d references, none alike", len(it.Pictures), len(refs)), None, "")
			}
		}
	}

	// The sound, where there are melodies.
	if len(it.Melodies) > 0 && ctx.Err() == nil {
		out = append(out, CheckMelodies(it.Name, it.Path, it.Melodies)...)
	}

	// The text, lightly: placeholders are not text.
	for _, t := range it.Text {
		if data, err := os.ReadFile(t); err == nil {
			lower := strings.ToLower(string(data))

			for _, ph := range []string{"lorem ipsum", "todo:", "tbd", "your description here", "coming soon..."} {
				if strings.Contains(lower, ph) {
					add("text and meaning", fmt.Sprintf("%s still has placeholder text (%q)", rel(it.Path, t), ph), Medium,
						"write what this item really is and does")

					break
				}
			}
		}
	}

	return out
}

func rel(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}

	return p
}

func mainPicture(p []string) string {
	for _, path := range p {
		n := strings.ToLower(filepath.Base(path))
		for _, cue := range []string{"cover", "icon", "logo", "capsule", "banner", "hero"} {
			if strings.Contains(n, cue) {
				return path
			}
		}
	}

	return p[0]
}

// CheckName searches the name where names are registered and sold.
func CheckName(name string, search Search) []Finding {
	if search == nil {
		return []Finding{{Item: name, Aspect: "name and trademarks", Risk: Low,
			Finding: "not searched: looking online is turned off",
			Action:  "search it in the registers: " + strings.Join(RegisterLinks(name), " · ")}}
	}

	var hits []Hit

	var failed []string

	places := NameQueries(name)
	keys := make([]string, 0, len(places))

	for k := range places {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, where := range keys {
		found, err := search(places[where])
		if err != nil {
			failed = append(failed, where)

			continue
		}

		for _, h := range found {
			h.Where = where
			hits = append(hits, h)
		}
	}

	var out []Finding

	for i, m := range CloseNames(name, hits) {
		if i == 3 {
			break
		}

		action := "rename it, or have the name cleared before publishing"
		if m.Risk == Low {
			action = "keep it in mind; check the register if it is in the same class"
		}

		out = append(out, Finding{Item: name, Aspect: "name and trademarks", Risk: m.Risk, Action: action,
			Finding: fmt.Sprintf("%q in %s (%.0f%% alike) %s", m.Name, m.Where, m.Score*100, m.URL)})
	}

	if len(out) == 0 {
		f := Finding{Item: name, Aspect: "name and trademarks", Risk: None,
			Finding: fmt.Sprintf("no close name in %d searches", len(keys)-len(failed)),
			Action:  "confirm in the registers before publishing: " + strings.Join(RegisterLinks(name), " · ")}
		out = append(out, f)
	}

	if len(failed) > 0 {
		out = append(out, Finding{Item: name, Aspect: "name and trademarks", Risk: Low,
			Finding: "could not search " + strings.Join(failed, ", "), Action: "search again later"})
	}

	return out
}

// CheckGame tests the one sentence and compares it with the closest titles.
func CheckGame(name, pitch, genre string, search Search) []Finding {
	var out []Finding

	if err := UniquenessSentence(pitch); err != nil {
		out = append(out, Finding{Item: name, Aspect: "game rules", Risk: Medium, Finding: err.Error(),
			Action: "write the one sentence of what only this game does; if there is none, give it its own rule"})
	}

	if search == nil || strings.TrimSpace(pitch) == "" {
		return out
	}

	var hits []Hit

	for _, q := range TitleQueries(pitch, genre) {
		if found, err := search(q); err == nil {
			hits = append(hits, found...)
		}
	}

	var named []Hit

	for _, h := range hits {
		if fold(nameOf(h.Title)) != fold(name) {
			named = append(named, h)
		}
	}

	closest := ClosestTitles(pitch, named)

	worst := None

	var list []string

	for _, t := range closest {
		list = append(list, fmt.Sprintf("%s (%.0f%%)", t.Title, t.Overlap*100))
		if riskOrder[t.Risk] < riskOrder[worst] {
			worst = t.Risk
		}
	}

	if len(list) > 0 {
		action := ""
		if worst == High || worst == Medium {
			action = "make the core rule its own: change what the player does, not only the look"
		}

		out = append(out, Finding{Item: name, Aspect: "game rules", Risk: worst, Action: action,
			Finding: "closest titles: " + strings.Join(list, ", ")})
	}

	return out
}

// CheckMelodies looks for the known themes in MIDI files and code.
func CheckMelodies(name, base string, files []string) []Finding {
	var out []Finding

	scanned := 0

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		var tunes [][]int

		ext := strings.ToLower(filepath.Ext(f))
		if ext == ".mid" || ext == ".midi" {
			tunes, _ = NotesInMIDI(data)
		} else {
			tunes = NotesInCode(string(data))
		}

		for _, notes := range tunes {
			scanned++

			for _, m := range CompareMelody(rel(base, f), notes) {
				names := make([]string, len(m.Played))
				for i, n := range m.Played {
					names[i] = NoteName(n)
				}

				known := m.Theme.Name + " (" + m.Theme.By
				if m.Theme.Note != "" {
					known += "; " + m.Theme.Note
				}

				out = append(out, Finding{Item: name, Aspect: "sound", Risk: m.Risk,
					Finding: fmt.Sprintf("%s plays %d notes of %s): %s", m.Where, m.Steps+1, known, strings.Join(names, " ")),
					Action:  "compose a new melody; changing the key or the speed does not make it new"})
			}
		}
	}

	if scanned > 0 && len(out) == 0 {
		out = append(out, Finding{Item: name, Aspect: "sound", Risk: None,
			Finding: fmt.Sprintf("%d melodies compared with %d known themes, none alike", scanned, len(Themes))})
	}

	return out
}
