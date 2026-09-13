package speech

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

/*
 * Knowing when to speak, and what of it to speak.
 *
 * It used to read the answer out. All of it, exactly as written — which is
 * right for "what time is it" and wrong for everything else this program
 * produces. A written answer has headings, bullet points, file paths and code
 * in it, and read aloud those come out as "hash hash what it found, star,
 * slash media slash petar slash…". Nobody listens to the end of that, so the
 * voice gets switched off, and then it cannot say the one thing that was worth
 * hearing either.
 *
 * Two questions, and they are separate. Whether to speak at all: an answer
 * that is nothing but code or a directory listing is already on the screen,
 * being read, and saying it aloud is worse than silence. And what to speak: an
 * answer worth hearing is worth hearing the first part of, in sentences, not
 * in its entirety.
 *
 * Done in code rather than by asking a model for a spoken version. A second
 * call is most of a minute on this processor, which would mean the answer
 * arrives on screen and the voice starts a minute later — and the whole point
 * of the voice is that somebody can be doing something else.
 */

const (
	/*
	 * AllOfIt is how short an answer has to be to be read out entirely.
	 *
	 * About a paragraph. Below this, cutting it would be the annoying thing:
	 * somebody hears most of an answer and has to go and look at the screen
	 * for the last line, which is the worst of both.
	 */
	AllOfIt = 320

	/*
	 * MostSpoken is where a longer one stops.
	 *
	 * A little over a minute of speech. Past that somebody is no longer
	 * listening to an answer, they are being read at — and the rest is on the
	 * screen, which is where a long answer belongs.
	 */
	MostSpoken = 700
)

/*
 * WorthSaying is what to read aloud, and whether to read anything at all.
 *
 * The false case is not a failure. "That is already on screen and reading it
 * would help nobody" is a real answer, and the program having it is the
 * difference between a voice somebody keeps on and one they turn off.
 */
func WorthSaying(text string) (string, bool) {
	spoken := forSpeaking(text)

	if spoken == "" {
		return "", false
	}

	if utf8.RuneCountInString(spoken) <= AllOfIt {
		return spoken, true
	}

	return firstOf(spoken, MostSpoken), true
}

/*
 * forSpeaking takes out everything that is to be looked at rather than heard.
 *
 * Fenced code first, because it is the commonest and the worst: a model that
 * has just written thirty lines of Go has written thirty lines that are
 * unlistenable, and they are already on the screen in a box designed for
 * reading. Then the marks — headings, bullets, emphasis — which are how a
 * written answer is laid out and mean nothing said aloud.
 */
func forSpeaking(text string) string {
	var kept []string

	inCode := false

	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode

			continue
		}

		if inCode || onlyToBeLookedAt(trimmed) {
			continue
		}

		kept = append(kept, withoutMarks(trimmed))
	}

	return strings.TrimSpace(collapse(strings.Join(kept, " ")))
}

/*
 * onlyToBeLookedAt reports whether a line is a thing to read rather than hear.
 *
 * Three of them, and each was a real sentence that came out of the speakers:
 * a table row read as a row of pipes, a path read letter by letter, and a
 * rule read as four hyphens. None of them carries anything when spoken, and
 * all three are ordinary in an answer about files.
 */
func onlyToBeLookedAt(line string) bool {
	if line == "" {
		return false
	}

	// A table row, or a rule under one.
	if strings.HasPrefix(line, "|") || strings.Trim(line, "-|: ") == "" {
		return true
	}

	// A path or a command on a line of its own. Said aloud it is a spelling
	// test; on screen it is the answer.
	if strings.HasPrefix(line, "/") || strings.HasPrefix(line, "$ ") ||
		strings.HasPrefix(line, "~/") {
		return true
	}

	/*
	 * And anything that is mostly not letters.
	 *
	 * Which catches the shapes there is no point enumerating: a line of
	 * numbers, a JSON fragment, a diff, an indented block that lost its
	 * fence. Measured rather than matched, so a new one does not need a new
	 * rule here.
	 */
	letters, all := 0, 0

	for _, r := range line {
		if unicode.IsSpace(r) {
			continue
		}

		all++

		if unicode.IsLetter(r) {
			letters++
		}
	}

	return all > 8 && letters*2 < all
}

// withoutMarks drops the punctuation that is layout rather than language.
func withoutMarks(line string) string {
	line = strings.TrimLeft(line, "#>-*+ \t")

	for _, mark := range []string{"**", "__", "`", "*", "_"} {
		line = strings.ReplaceAll(line, mark, "")
	}

	return strings.TrimSpace(line)
}

func collapse(text string) string { return strings.Join(strings.Fields(text), " ") }

/*
 * firstOf keeps whole sentences up to a limit.
 *
 * Whole, because a voice that stops in the middle of a clause sounds like it
 * has crashed. Counted in runes rather than bytes, since half this program's
 * sentences are in Bulgarian and every letter there is two bytes — a limit in
 * bytes would cut a Bulgarian answer in half and an English one not at all.
 */
func firstOf(text string, most int) string {
	var out strings.Builder

	count := 0

	for _, sentence := range sentences(text) {
		length := utf8.RuneCountInString(sentence)

		if count > 0 && count+length > most {
			break
		}

		out.WriteString(sentence)

		count += length

		if count >= most {
			break
		}
	}

	return strings.TrimSpace(out.String())
}

// sentences splits on the ends of sentences, keeping the punctuation with the
// sentence it belongs to.
func sentences(text string) []string {
	var (
		out []string
		at  strings.Builder
	)

	for _, r := range text {
		at.WriteRune(r)

		if r == '.' || r == '!' || r == '?' || r == '\n' {
			out = append(out, at.String())
			at.Reset()
		}
	}

	if rest := at.String(); strings.TrimSpace(rest) != "" {
		out = append(out, rest)
	}

	return out
}
