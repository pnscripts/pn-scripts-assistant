package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/places"
)

/*
 * Ten pages said in both languages.
 *
 * 1.0.0 gave every menu entry a Bulgarian name and left the pages behind
 * them partly English: Command centre, Models, System, Operations, The room,
 * Teaching, Organisation, Tasks, Reaching it and Privacy, with the one-time
 * recovery key notice above them. These tests read the page and its scripts
 * the way a person reads the screen: every word on those pages names its
 * string in i18n.js, and every string has Bulgarian in it.
 *
 * The "Your voice" card on Privacy is included since 1.0.1, which rewrote it
 * when "Answer only me" was withdrawn.
 */

// The views whose every word must come from i18n.js.
var bilingualViews = []string{
	"console", "models", "system", "operations", "room",
	"teaching", "organisation", "tasks", "reach", "privacy",
	"programs",
}

// Text that reads the same in both languages: names, not words.
var sameInBothLanguages = map[string]bool{
	"{{ .Name }}": true, // the assistant's own name, filled in by the server
	"English":     true, // a language is offered under its own name
	"Deutsch":     true,
	"Français":    true,
	"Español":     true,
	"Italiano":    true,
	"Türkçe":      true,
	"ESCO":        true, // the names of two classifications
	"O*NET":       true,
	"postgresql":  true, // a program's name, as a person would type it
	"alex":        true, // a name
}

// sameEverywhere says whether a text is a name, a path or an address rather
// than words: those read the same on a Bulgarian page.
func sameEverywhere(text string) bool {
	if sameInBothLanguages[text] {
		return true
	}

	if strings.HasPrefix(text, "/") {
		return true // a path
	}

	return regexp.MustCompile(`^[\w.-]+(@[\w.-]+)?\.[a-z]{2,}$`).MatchString(text) // a host or an address
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true,
	"track": true, "wbr": true, "circle": true, "path": true, "rect": true,
}

// A piece of the page a person reads.
type pageText struct {
	line  int
	where string // the view, or the id of the block outside the views
	tag   string
	attrs string // of the element the text is in
	text  string
	attr  string // "" for text; title, placeholder or aria-label for an attribute
}

// withoutComments blanks out comments, scripts and styles, keeping
// every line where it was so a failure names the right one.
func withoutComments(t *testing.T, page string) string {
	t.Helper()

	blank := func(s string) string {
		return regexp.MustCompile(`[^\n]`).ReplaceAllString(s, " ")
	}

	page = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllStringFunc(page, blank)
	page = regexp.MustCompile(`(?s)<script\b.*?</script>`).ReplaceAllStringFunc(page, blank)
	page = regexp.MustCompile(`(?s)<style\b.*?</style>`).ReplaceAllStringFunc(page, blank)

	return page
}

/*
 * readablePage walks the page and returns what a person reads in the given
 * views and blocks (by id), and every element carrying data-i18n that has an
 * element inside it, which filling it in would wipe.
 */
func readablePage(t *testing.T, page string, views []string, blocks []string) ([]pageText, []string) {
	t.Helper()

	page = withoutComments(t, page)

	type open struct {
		tag, attrs string
		line       int
		where      string
		keyed      bool
	}

	var (
		stack  []*open
		texts  []pageText
		wiped  []string
		inside = func() string {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].where != "" {
					return stack[i].where
				}
			}

			return ""
		}
	)

	wanted := map[string]string{}
	for _, v := range views {
		wanted[`section|data-view="`+v+`"`] = v
	}

	for _, b := range blocks {
		wanted[`|id="`+b+`"`] = b
	}

	tagRE := regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)((?:[^>"']|"[^"]*"|'[^']*')*)>`)
	attrRE := regexp.MustCompile(`\s(title|placeholder|aria-label)="([^"]*)"`)
	letters := regexp.MustCompile(`[A-Za-z]`)

	lineAt := func(at int) int { return strings.Count(page[:at], "\n") + 1 }
	last := 0

	text := func(from, to int) {
		raw := page[from:to]
		if !letters.MatchString(raw) || len(stack) == 0 || inside() == "" {
			return
		}

		top := stack[len(stack)-1]
		texts = append(texts, pageText{
			line: lineAt(from), where: inside(), tag: top.tag, attrs: top.attrs,
			text: strings.Join(strings.Fields(strings.ReplaceAll(raw, "&amp;", "&")), " "),
		})
	}

	for _, m := range tagRE.FindAllStringSubmatchIndex(page, -1) {
		text(last, m[0])
		last = m[1]

		closing := page[m[2]:m[3]] == "/"
		name := strings.ToLower(page[m[4]:m[5]])
		attrs := page[m[6]:m[7]]

		if closing {
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				if top.tag == name {
					break
				}
			}

			continue
		}

		if len(stack) > 0 {
			if p := stack[len(stack)-1]; p.keyed && name != "br" {
				wiped = append(wiped, fmt.Sprintf("line %d: <%s%s> holds <%s>", p.line, p.tag, p.attrs, name))
				p.keyed = false // once is enough
			}
		}

		o := &open{tag: name, attrs: attrs, line: lineAt(m[0])}

		for sig, where := range wanted {
			tagWanted, attr, _ := strings.Cut(sig, "|")
			if (tagWanted == "" || tagWanted == name) && strings.Contains(attrs, attr) {
				o.where = where
			}
		}

		o.keyed = strings.Contains(attrs, `data-i18n="`)

		if inside() != "" || o.where != "" {
			where := o.where
			if where == "" {
				where = inside()
			}

			for _, a := range attrRE.FindAllStringSubmatch(attrs, -1) {
				if letters.MatchString(a[2]) {
					texts = append(texts, pageText{
						line: lineAt(m[0]), where: where, tag: name, attrs: attrs,
						text: strings.Join(strings.Fields(a[2]), " "), attr: a[1],
					})
				}
			}
		}

		if !voidElements[name] && !strings.HasSuffix(attrs, "/") {
			stack = append(stack, o)
		}
	}

	return texts, wiped
}

func TestTheTenPagesAreSaidInBothLanguages(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	texts, wiped := readablePage(t, page, bilingualViews, []string{"recovery", "update-banner"})
	if len(texts) < 300 {
		t.Fatalf("found %d pieces of text on the ten pages; has the page moved?", len(texts))
	}

	cyrillic := regexp.MustCompile(`[а-яА-Я]`)
	keyOf := map[string]string{"": "data-i18n", "title": "data-i18n-title", "placeholder": "data-i18n-placeholder", "aria-label": "data-i18n-label"}
	missing := map[string]int{}

	for _, p := range texts {
		if sameEverywhere(p.text) {
			continue
		}

		k := regexp.MustCompile(keyOf[p.attr] + `="([^"]+)"`).FindStringSubmatch(p.attrs)
		if k == nil {
			missing[p.where]++

			what := "text"
			if p.attr != "" {
				what = p.attr
			}

			t.Errorf("%s, line %d: English only (%s of <%s>): %q", p.where, p.line, what, p.tag, p.text)

			continue
		}

		en, bg := strings2(t, js, k[1])
		if en != p.text {
			t.Errorf("%s, line %d: the page says %q and i18n.js %s says %q", p.where, p.line, p.text, k[1], en)
		}

		if !cyrillic.MatchString(bg) {
			t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", k[1], bg)
		}
	}

	for _, w := range wiped {
		t.Errorf("filling in this element would wipe what is inside it: %s", w)
	}

	if len(missing) > 0 {
		var where []string
		for v, n := range missing {
			where = append(where, fmt.Sprintf("%s %d", v, n))
		}

		sort.Strings(where)
		t.Logf("English only, by page: %s", strings.Join(where, ", "))
	}
}

// Every key the pages and their scripts name is in i18n.js in both languages.
func TestEveryKeyThePagesNameIsInBothLanguages(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	keys := map[string]string{}

	for _, m := range regexp.MustCompile(`data-i18n(?:-title|-placeholder|-label)?="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		keys[m[1]] = "index.html"
	}

	for _, file := range bilingualScripts {
		src := readAsset(t, filepath.Join("js", file))

		for _, m := range regexp.MustCompile(`\b(?:pnT|T)\(\s*'([a-zA-Z][\w]*\.[\w.]+)'`).FindAllStringSubmatch(src, -1) {
			keys[m[1]] = file
		}
	}

	for key, file := range keys {
		if !strings.Contains(js, "'"+key+"': [") {
			t.Errorf("%s names %s, which i18n.js does not have", file, key)

			continue
		}

		if _, bg := strings2(t, js, key); !cyrillic.MatchString(bg) {
			t.Errorf("%s (named in %s) has no Bulgarian in it: %q", key, file, bg)
		}
	}
}

// The scripts that draw the ten pages, the recovery key and the conversation.
var bilingualScripts = []string{
	"console.js", "dashboard.js", "feed.js", "working.js", "history.js", "status.js", "boot.js", "vault.js",
	"models.js", "providers.js", "install.js", "updates.js",
	"settings.js", "copies.js", "places.js", "journey.js",
	"operations.js",
	"room.js", "diary.js",
	"teaching.js",
	"organisation.js", "equip.js",
	"tasks.js", "team.js",
	"devices.js", "pairing.js",
	"protection.js", "voiceprint.js",
	"programs.js",
}

// Literals in those scripts that are code, not words a person reads.
var scriptWordsThatAreCode = regexp.MustCompile(`^(use strict|Content-Type|Accept|Enter|Escape|Return|Tab|ArrowUp|ArrowDown|POST|GET|PUT|DELETE` +
	// A shell command shown for copying is a command in either language.
	`|\|? ?sudo .*` +
	// The part names /api/appearance takes (settings.js SPOKEN_PART): read by the server, never shown.
	`|thinking line)$`)

// cssClasses is every class the stylesheet names: a literal made only of
// those is a list of classes, not words.
func cssClasses(t *testing.T) map[string]bool {
	t.Helper()

	css := readAsset(t, filepath.Join("css", "console.css"))
	out := map[string]bool{}

	for _, m := range regexp.MustCompile(`\.([a-zA-Z][\w-]*)`).FindAllStringSubmatch(css, -1) {
		out[m[1]] = true
	}

	return out
}

func onlyClasses(s string, classes map[string]bool) bool {
	words := strings.Fields(s)
	if len(words) == 0 {
		return false
	}

	for _, w := range words {
		if !classes[w] {
			return false
		}
	}

	return true
}

/*
 * No English sentence is written into the scripts of the ten pages: each is
 * a string in i18n.js, so the Bulgarian page says it in Bulgarian. A phrase
 * is two words or more, or one capitalised word on its own ('Save').
 */
func TestTheTenPagesScriptsKeepTheirWordsInI18n(t *testing.T) {
	phrase := regexp.MustCompile(`[A-Za-z]{2,} [a-z]{2,}`)
	word := regexp.MustCompile(`^[A-Z][a-z]+[.…]?$`)
	literal := regexp.MustCompile("'((?:[^'\\\\\n]|\\\\.)*)'|\"((?:[^\"\\\\\n]|\\\\.)*)\"|`((?:[^`\\\\]|\\\\.)*)`")
	blockComment := regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment := regexp.MustCompile(`(?m)^\s*//.*$|\s//[^'"` + "`" + `\n]*$`)
	interpolation := regexp.MustCompile(`\$\{[^}]*\}`)
	classes := cssClasses(t)
	count := 0

	for _, file := range bilingualScripts {
		src := readAsset(t, filepath.Join("js", file))
		src = blockComment.ReplaceAllStringFunc(src, func(s string) string {
			return strings.Repeat("\n", strings.Count(s, "\n"))
		})
		src = lineComment.ReplaceAllString(src, "")

		for _, m := range literal.FindAllStringSubmatchIndex(src, -1) {
			var s string

			for g := 2; g < 8; g += 2 {
				if m[g] >= 0 {
					s = src[m[g]:m[g+1]]
				}
			}

			s = interpolation.ReplaceAllString(s, " ")
			if scriptWordsThatAreCode.MatchString(s) || onlyClasses(s, classes) || !(phrase.MatchString(s) || word.MatchString(strings.TrimSpace(s))) {
				continue
			}

			if strings.ContainsAny(s, "<>{};=") && !strings.ContainsAny(s, ".,") {
				continue // markup or code in a string
			}

			count++
			t.Errorf("%s:%d: English in the script: %q", file, strings.Count(src[:m[0]], "\n")+1, s)
		}
	}

	if count > 0 {
		t.Logf("%d English strings in the scripts of the ten pages", count)
	}
}

/*
 * Words the pages recognise in what the server says, so they can say them in
 * Bulgarian, are still what the server says: a changed sentence on the server
 * would otherwise stay English on a Bulgarian page without anybody noticing.
 */
func TestTheWordsThePagesRecogniseAreStillTheServers(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	if en, _ := strings2(t, js, "sys.knowledgeName"); en != places.KnowledgeName {
		t.Errorf("sys.knowledgeName says %q and the server names the knowledge folder %q", en, places.KnowledgeName)
	}

	source := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		return string(raw)
	}

	recipes := source(filepath.Join("..", "provision", "recipes.go"))
	for _, said := range []*regexp.Regexp{
		regexp.MustCompile(`"your distribution's package " \+ pkg`),
		regexp.MustCompile(`Cost:\s+"free"`),
	} {
		if !said.MatchString(recipes) {
			t.Errorf("provision/recipes.go no longer says %s, which equip.js translates", said)
		}
	}

	parts := source(filepath.Join("..", "..", "preflight", "requirements_list.go"))
	for _, said := range []string{": system libraries, installed by apt", ": system headers, installed by apt", ", installed by apt"} {
		if !strings.Contains(parts, said) {
			t.Errorf("preflight no longer says %q, which install.js translates", said)
		}
	}
}

// Every job a shipped position stands on is named in both languages, with
// the English the job really has.
func TestTheChartsJobsAreNamedInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	titles := map[string]string{}
	jobs, _ := occupations.Seed()

	for _, j := range jobs {
		titles[j.ID] = j.Title
	}

	for _, u := range org.BuiltIn() {
		for _, p := range u.Seats {
			if p.Job == "" {
				continue
			}

			en, bg := strings2(t, js, "org.job."+p.Job)
			if en != titles[p.Job] {
				t.Errorf("org.job.%s says %q and the job is called %q", p.Job, en, titles[p.Job])
			}

			if !cyrillic.MatchString(bg) {
				t.Errorf("org.job.%s has no Bulgarian in it: %q", p.Job, bg)
			}
		}
	}
}
