package tools

import (
	"encoding/json"
	"unicode"
)

/*
 * Approval cards in the page's language.
 *
 * Summarize is English: the model reads it, the deeds keep it and the page
 * matches nothing else in it. A person deciding on a Bulgarian page read the
 * same English words around the path, the command or the message they were
 * judging. Each tool now says its card in both languages from one function,
 * SummarizeIn, with every sentence written twice side by side (Say), so a
 * card changed in one language is seen to need changing in the other. What
 * the card is about (paths, commands, addresses, the text of a message) is
 * the same in both: only the words around it are translated, so what is
 * approved does not change with the language it is read in.
 */

// CardSaying is a tool whose approval card is said in both languages.
type CardSaying interface {
	SummarizeIn(args json.RawMessage, lang string) string
}

// SummaryIn is a tool's card in a language ("en" or "bg"). A tool that
// cannot say it in Bulgarian gives its English one.
func SummaryIn(t Tool, args json.RawMessage, lang string) string {
	if c, ok := t.(CardSaying); ok {
		return c.SummarizeIn(args, lang)
	}

	return t.Summarize(args)
}

// Say is the sentence in the language asked for: Bulgarian for "bg",
// English otherwise.
func Say(lang, en, bg string) string {
	if lang == "bg" {
		return bg
	}

	return en
}

// SaysBulgarian reports whether a text has any Cyrillic in it.
func SaysBulgarian(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}

	return false
}
