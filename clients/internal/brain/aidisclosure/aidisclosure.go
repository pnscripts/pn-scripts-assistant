/*
 * Package aidisclosure is the assistant saying that it is an AI.
 *
 * From 2 August 2026, article 50 of the EU AI Act (Regulation (EU)
 * 2024/1689) asks two things of an AI system that talks with people and
 * writes text for them. Paragraph 1: the person is told, clearly and at the
 * latest at the first interaction, that they are dealing with an AI.
 * Paragraph 2: text it generates is marked, in a machine-readable way, as
 * made by an AI.
 *
 * Everything that says or marks it comes from here, so the words are the
 * same on the page, in the voice and in what goes out: the first meeting
 * says it out loud (Introduced), a sent mail carries an AI-Disclosure header
 * (MailHeader, MailValue) and a written document carries it in its
 * properties (DocumentNote, DocumentKeywords, Producer). The page's own line
 * is in i18n.js (cc.aiNotice), kept in step by a server test.
 */
package aidisclosure

import (
	"regexp"
	"strings"
	"unicode"
)

// The notice of a first meeting, said before anything else.
const (
	noticeEN = "Before anything else: I am an AI assistant, and my answers come from a language model, so they can be wrong."
	noticeBG = "Преди всичко: аз съм асистент с изкуствен интелект и отговорите ми идват от езиков модел, затова може да грешат."
)

// Notice is the first-meeting notice in the person's language: Bulgarian for
// "bg" (any case, any region), English otherwise.
func Notice(language string) string {
	if isBulgarian(language) {
		return noticeBG
	}

	return noticeEN
}

func isBulgarian(language string) bool {
	l := strings.ToLower(strings.TrimSpace(language))

	return l == "bg" || strings.HasPrefix(l, "bg-") || strings.HasPrefix(l, "bg_")
}

var (
	// "artificial intelligence" in any case; "изкуствен интелект" in any
	// of its forms (изкуственият, изкуствения, изкуствен).
	spelledOut = regexp.MustCompile(`(?i)artificial\s+intelligence|изкуствен\p{L}*\s+интелект`)
)

/*
 * Discloses says whether the words already tell the reader it is an AI.
 *
 * "AI" and "ИИ" count only as words of their own, in capitals: "said",
 * "MAIL" and "взаимно" hold the letters and say nothing of the kind.
 */
func Discloses(text string) bool {
	if spelledOut.MatchString(text) {
		return true
	}

	return hasWord(text, "AI") || hasWord(text, "ИИ")
}

// hasWord is word in text with no letter or digit touching it on either side.
func hasWord(text, word string) bool {
	for from := 0; ; {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return false
		}

		at += from
		end := at + len(word)

		if !letterBefore(text[:at]) && !letterAfter(text[end:]) {
			return true
		}

		from = end
	}
}

func letterBefore(s string) bool {
	r := []rune(s)

	return len(r) > 0 && isWordRune(r[len(r)-1])
}

func letterAfter(s string) bool {
	for _, r := range s {
		return isWordRune(r)
	}

	return false
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

/*
 * Introduced is the opener of a first meeting with the notice in it.
 *
 * The model is asked to say what it is, and usually does; when its words do
 * not, the notice goes in front of them, so the question it ends on (what to
 * call the person) stays last, where an answer is expected. Words written in
 * Cyrillic get the Bulgarian notice whatever the setting, because that is
 * the language the person is about to hear.
 *
 * Nothing said stays nothing: with no model there is no AI talking, and the
 * page's own line still says what answers are.
 */
func Introduced(said, language string) string {
	said = strings.TrimSpace(said)

	if said == "" || Discloses(said) {
		return said
	}

	if mostlyCyrillic(said) {
		language = "bg"
	}

	return Notice(language) + " " + said
}

func mostlyCyrillic(s string) bool {
	cyrillic, latin := 0, 0

	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}

	return cyrillic > latin
}

/*
 * The mark on a sent mail.
 *
 * There is no standard header for this in mail. The AI-Disclosure field of
 * the IETF draft draft-abaris-aicdh-01 (an individual draft for HTTP, read
 * 13.11.2026) is the closest published shape, a structured dictionary, so
 * mail uses the same name and syntax: mode=ai-originated is "AI produced the
 * core content and a person reviewed it", which is what happens here, since
 * nothing is sent without its owner reading and approving the words.
 */
const (
	MailHeader = "AI-Disclosure"
	MailValue  = `mode=ai-originated; provider="PN Scripts Assistant"`
)

// The mark on a written document, in its properties (PDF Info, Word core
// properties). Plain ASCII with nothing to escape in PDF strings or XML.
const (
	Producer         = "PN Scripts Assistant"
	DocumentKeywords = "AI-generated"
	DocumentNote     = "Written with PN Scripts Assistant, an AI assistant."
)
