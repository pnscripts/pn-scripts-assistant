/*
 * Package ask is how the assistant asks its owner anything: one question at a
 * time, two to four answers to click, the recommended one first and marked,
 * and "Other" for anything else.
 *
 * A message with five questions in it gets one of them answered. A question
 * with no suggested answers makes its owner do the thinking the assistant was
 * there to do. So every question goes through here — from setup, from the
 * planner and from any expert — and Split turns a list of questions into a
 * queue that is asked one after another.
 *
 * Voice reads the question and the numbered answers, and Answer accepts a
 * number, an ordinal or the words of an answer, in Bulgarian or English.
 */
package ask

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Question is one thing to ask, with the answers to offer.
type Question struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
	// Recommended is the index of the suggested answer. Show puts it first.
	Recommended int `json:"recommended"`
}

// Most and Fewest are how many answers a question offers, besides Other.
const (
	Fewest = 2
	Most   = 4
)

// Validate says what is wrong with a question, or nil.
func Validate(q Question) error {
	text := strings.TrimSpace(q.Text)

	switch {
	case text == "":
		return errors.New("a question needs words")
	case strings.Count(text, "?") > 1:
		return errors.New("one question at a time: this asks several")
	case len(q.Options) < Fewest || len(q.Options) > Most:
		return fmt.Errorf("a question offers %d to %d answers, not %d", Fewest, Most, len(q.Options))
	case q.Recommended < 0 || q.Recommended >= len(q.Options):
		return errors.New("the recommended answer is not one of the answers")
	}

	for _, o := range q.Options {
		if strings.TrimSpace(o) == "" {
			return errors.New("an answer is empty")
		}
	}

	return nil
}

// Ordered is the answers with the recommended one first.
func Ordered(q Question) []string {
	out := []string{q.Options[q.Recommended]}

	for i, o := range q.Options {
		if i != q.Recommended {
			out = append(out, o)
		}
	}

	return out
}

// Show is the question as chat text and as speech: numbered answers, the
// first marked as recommended, and Other last.
func Show(q Question, lang string) string {
	recommended, other := " (Recommended)", "Other — say it in your own words"
	if lang == "bg" {
		recommended, other = " (Препоръчително)", "Друго — кажете го с ваши думи"
	}

	var b strings.Builder

	b.WriteString(strings.TrimSpace(q.Text))

	for i, o := range Ordered(q) {
		b.WriteString(fmt.Sprintf("\n%d. %s", i+1, o))

		if i == 0 {
			b.WriteString(recommended)
		}
	}

	b.WriteString(fmt.Sprintf("\n%d. %s", len(q.Options)+1, other))

	return b.String()
}

var questionEnd = regexp.MustCompile(`[^?]*\?`)

// Split turns text that asks several things into one question each, in
// order, so they can be asked one after another.
func Split(text string) []string {
	var out []string

	for _, m := range questionEnd.FindAllString(text, -1) {
		if q := strings.TrimSpace(strings.TrimLeft(m, " \n\t-*•0123456789.)")); q != "" {
			out = append(out, q)
		}
	}

	return out
}

var ordinals = map[string]int{
	"one": 1, "first": 1, "1st": 1, "едно": 1, "едното": 1, "първо": 1, "първото": 1, "първия": 1, "първа": 1, "първата": 1,
	"two": 2, "second": 2, "2nd": 2, "две": 2, "второ": 2, "второто": 2, "втория": 2, "втора": 2, "втората": 2,
	"three": 3, "third": 3, "3rd": 3, "три": 3, "трето": 3, "третото": 3, "третия": 3, "трета": 3, "третата": 3,
	"four": 4, "fourth": 4, "4th": 4, "четири": 4, "четвърто": 4, "четвъртото": 4, "четвъртия": 4, "четвърта": 4,
	"five": 5, "fifth": 5, "пет": 5, "пето": 5, "петото": 5,
	"recommended": 1, "препоръчителното": 1, "препоръчително": 1,
}

// Answer reads a reply to q. It returns the chosen answer, or other=true with
// the reply itself when it is none of them.
func Answer(q Question, reply string) (choice string, other bool) {
	shown := Ordered(q)
	r := strings.ToLower(strings.TrimSpace(strings.Trim(reply, ".!")))

	pick := func(n int) (string, bool) {
		if n >= 1 && n <= len(shown) {
			return shown[n-1], true
		}

		return "", false
	}

	if n, err := strconv.Atoi(r); err == nil {
		if c, ok := pick(n); ok {
			return c, false
		}

		return strings.TrimSpace(reply), true
	}

	// The words of an answer win over a number word inside them.
	for _, o := range shown {
		if strings.EqualFold(r, strings.TrimSpace(o)) {
			return o, false
		}
	}

	for _, w := range strings.Fields(r) {
		w = strings.Trim(w, ",.;:!")
		if n, ok := ordinals[w]; ok {
			if c, ok := pick(n); ok {
				return c, false
			}
		}
	}

	for _, o := range shown {
		lo := strings.ToLower(o)
		if len([]rune(r)) >= 3 && strings.Contains(lo, r) {
			return o, false
		}
	}

	return strings.TrimSpace(reply), true
}
