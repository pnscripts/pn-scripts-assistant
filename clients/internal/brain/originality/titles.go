package originality

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

/*
 * The game-design check: what only this game does, in one sentence, and the
 * closest titles to it.
 *
 * The sentence is the test. A game that cannot say what only it does in one
 * sentence, or says it as "X but with Y", is a reskin until shown otherwise.
 */

var (
	reskin    = regexp.MustCompile(`(?i)\b(like|clone of|-like|inspired by|based on|version of|but with|като|подобна на|клонинг)\b`)
	sentences = regexp.MustCompile(`[.!?](\s|$)`)
	wordRe    = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

// UniquenessSentence says what is wrong with the one sentence, or nil.
func UniquenessSentence(s string) error {
	s = strings.TrimSpace(s)

	switch n := len(wordRe.FindAllString(s, -1)); {
	case n == 0:
		return errors.New("there is no sentence saying what only this game does")
	case n < 6:
		return errors.New("the sentence is too short to say what only this game does")
	case n > 45:
		return errors.New("it takes more than one sentence to say what only this game does — find the one rule")
	}

	if len(sentences.FindAllStringIndex(strings.TrimRight(s, ".!? "), -1)) > 0 {
		return errors.New("it takes more than one sentence to say what only this game does — find the one rule")
	}

	if m := reskin.FindString(s); m != "" {
		return errors.New("the sentence defines the game by another one (\"" + m + "\") — say the rule only this game has")
	}

	return nil
}

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the and or of to in on at for with by from as is are be it its this that you your
		player players game games where which who into out up down each every one two all can will must only
		и в на за с от по до че се е са да не ще това този тази играч игра игри който която`) {
		stop[w] = true
	}
}

func contentWords(s string) map[string]bool {
	out := map[string]bool{}

	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if len([]rune(w)) < 3 || stop[w] {
			continue
		}

		// A light stem, so "matches" and "matching" meet.
		for _, end := range []string{"ing", "es", "s", "ed"} {
			if strings.HasSuffix(w, end) && len(w)-len(end) >= 4 {
				w = strings.TrimSuffix(w, end)

				break
			}
		}

		out[w] = true
	}

	return out
}

// Overlap is how much of the pitch's content the other text shares, 0..1.
func Overlap(pitch, other string) float64 {
	p, o := contentWords(pitch), contentWords(other)
	if len(p) == 0 {
		return 0
	}

	shared := 0

	for w := range p {
		if o[w] {
			shared++
		}
	}

	return float64(shared) / float64(len(p))
}

// TitleMatch is a known title near ours.
type TitleMatch struct {
	Title, URL, Snippet string
	Overlap             float64
	Risk                Risk
}

// ClosestTitles are the three hits closest to the pitch.
func ClosestTitles(pitch string, hits []Hit) []TitleMatch {
	seen := map[string]bool{}

	var out []TitleMatch

	for _, h := range hits {
		name := nameOf(h.Title)
		if seen[fold(name)] || fold(name) == "" {
			continue
		}

		seen[fold(name)] = true

		score := Overlap(pitch, h.Title+" "+h.Snippet)
		risk := None

		switch {
		case score >= 0.6:
			risk = High
		case score >= 0.4:
			risk = Medium
		case score >= 0.2:
			risk = Low
		}

		out = append(out, TitleMatch{Title: name, URL: h.URL, Snippet: h.Snippet, Overlap: score, Risk: risk})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Overlap > out[j].Overlap })

	if len(out) > 3 {
		out = out[:3]
	}

	return out
}

// TitleQueries search for the games nearest a pitch.
func TitleQueries(pitch, genre string) []string {
	words := wordRe.FindAllString(pitch, -1)
	if len(words) > 12 {
		words = words[:12]
	}

	q := []string{strings.Join(words, " ") + " game"}

	if genre = strings.TrimSpace(genre); genre != "" {
		q = append(q, "best "+genre+" games", "games like "+genre)
	}

	return q
}
