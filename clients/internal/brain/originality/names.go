// Package originality is what the originality specialists check with: how
// close a name is to names already in use, how close a game's core rule is to
// the closest known titles, how close a picture is to reference pictures, how
// close a melody is to well-known themes, and the audit report that collects
// what they found (knowledge/guides/originality.md).
//
// Everything here is measured, not judged: a score and a risk for a person or
// a specialist to act on. Nothing in it reaches the network itself; the
// searches are passed in, so the same checks run offline in tests.
package originality

import (
	"sort"
	"strings"
	"unicode"
)

// Risk is how much a finding needs action.
type Risk string

const (
	High   Risk = "high"
	Medium Risk = "medium"
	Low    Risk = "low"
	None   Risk = "none"
)

// Hit is one search result, whichever engine found it.
type Hit struct {
	Title, URL, Snippet string
	// Where is what was searched: "trademark registers", "app stores"...
	Where string
}

// Search finds pages for a query. Passed in, so nothing here goes online.
type Search func(query string) ([]Hit, error)

// fold is a name reduced to what is heard and read: lower case, letters and
// digits only, accents removed for the common Latin ones.
func fold(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unaccent(r))
		}
	}

	return b.String()
}

func unaccent(r rune) rune {
	switch r {
	case 'á', 'à', 'â', 'ä', 'ã', 'å':
		return 'a'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ó', 'ò', 'ô', 'ö', 'õ':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	case 'ç':
		return 'c'
	case 'ñ':
		return 'n'
	}

	return r
}

// skeleton is how a name sounds, roughly: vowels dropped after the first
// letter, look-alike letters merged, doubles collapsed. "Tetriz" and "Tetris",
// "Pakman" and "Pac-Man" share one.
func skeleton(name string) string {
	f := fold(name)
	if f == "" {
		return ""
	}

	same := strings.NewReplacer("ph", "f", "ck", "k", "qu", "k", "c", "k", "q", "k", "z", "s", "x", "ks", "y", "i", "w", "v")
	f = same.Replace(f)

	var b strings.Builder

	var last rune

	for i, r := range f {
		if i > 0 && strings.ContainsRune("aeiou", r) {
			continue
		}

		if r != last {
			b.WriteRune(r)
		}

		last = r
	}

	return b.String()
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i

		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}

			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}

		prev = cur
	}

	return prev[len(rb)]
}

func ratio(a, b string) float64 {
	if a == "" && b == "" {
		return 1
	}

	longest := max(len([]rune(a)), len([]rune(b)))

	return 1 - float64(levenshtein(a, b))/float64(longest)
}

// NameSimilarity is 0..1: spelling, sound and containment, the highest wins.
func NameSimilarity(ours, theirs string) float64 {
	a, b := fold(ours), fold(theirs)
	if a == "" || b == "" {
		return 0
	}

	score := ratio(a, b)

	if sa, sb := skeleton(ours), skeleton(theirs); len(sa) >= 3 && sa == sb {
		score = max(score, 0.9)
	}

	// Their whole name inside ours, or ours inside theirs ("Tetris Blast").
	if len(a) >= 4 && len(b) >= 4 && (wholeWord(theirs, ours) || wholeWord(ours, theirs)) {
		score = max(score, 0.85)
	}

	return score
}

func wholeWord(in, word string) bool {
	w := fold(word)

	for _, part := range strings.FieldsFunc(in, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if fold(part) == w {
			return true
		}
	}

	return false
}

// NameRisk turns a similarity into what to do about it.
func NameRisk(score float64) Risk {
	switch {
	case score >= 0.85:
		return High
	case score >= 0.7:
		return Medium
	case score >= 0.55:
		return Low
	}

	return None
}

// NameMatch is a name in use that is close to ours.
type NameMatch struct {
	Name, URL, Where string
	Score            float64
	Risk             Risk
}

// nameOf is the product name a result's title gives: the part before the
// first separator ("Tetris® - Apps on Google Play" → "Tetris").
func nameOf(title string) string {
	cut := len(title)

	for _, sep := range []string{" - ", " | ", " – ", " — ", ": ", " · ", " on Steam", " by "} {
		if i := strings.Index(title, sep); i > 0 && i < cut {
			cut = i
		}
	}

	name := strings.TrimSpace(title[:cut])

	return strings.Trim(name, "®™©\"' ")
}

// CloseNames are the names in hits that are close to ours, the closest first,
// one entry per name.
func CloseNames(ours string, hits []Hit) []NameMatch {
	seen := map[string]bool{}

	var out []NameMatch

	for _, h := range hits {
		name := nameOf(h.Title)
		key := fold(name)

		if key == "" || seen[key] {
			continue
		}

		score := NameSimilarity(ours, name)
		risk := NameRisk(score)

		if risk == None {
			continue
		}

		seen[key] = true
		out = append(out, NameMatch{Name: name, URL: h.URL, Where: h.Where, Score: score, Risk: risk})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	return out
}

// NameQueries are the public places a name is searched in: trademark
// registers, app and game stores, and the web.
func NameQueries(name string) map[string]string {
	q := "\"" + name + "\""

	return map[string]string{
		"trademark registers": q + " trademark (site:tmdn.org OR site:euipo.europa.eu OR site:uspto.gov OR site:wipo.int)",
		"app stores":          q + " (site:play.google.com OR site:apps.apple.com OR site:microsoft.com/store)",
		"game stores":         q + " game (site:store.steampowered.com OR site:itch.io OR site:gamejolt.com OR site:crazygames.com)",
		"the web":             q,
	}
}

// RegisterLinks open the official registers on the name, for confirming a
// finding by hand; the registers have no free programmatic search.
func RegisterLinks(name string) []string {
	q := queryEscape(name)

	return []string{
		"EUIPO TMview: https://www.tmdn.org/tmview/#/tmview/results?page=1&pageSize=30&criteria=C&basicSearch=" + q,
		"USPTO: https://tmsearch.uspto.gov/search/search-results?query=" + q,
		"WIPO Global Brand Database: https://branddb.wipo.int/",
	}
}

func queryEscape(s string) string {
	var b strings.Builder

	for _, r := range s {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case r == ' ':
			b.WriteString("%20")
		default:
			for _, c := range []byte(string(r)) {
				b.WriteString("%")
				b.WriteString(strings.ToUpper(string("0123456789abcdef"[c>>4]) + string("0123456789abcdef"[c&15])))
			}
		}
	}

	return b.String()
}
