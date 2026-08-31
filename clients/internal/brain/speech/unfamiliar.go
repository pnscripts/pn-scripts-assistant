package speech

import (
	"regexp"
	"strings"
)

/*
 * Noticing a name it has never heard, instead of guessing at it.
 *
 * Biasing the recogniser toward names the brain already knows fixes the names
 * the brain already knows, and no list is ever finished. The first time
 * somebody says a domain, a product, a person, it is not in memory, in browser
 * history or in any file — and whisper will still return the nearest thing in
 * its vocabulary rather than admitting it did not catch it.
 *
 * That is the failure worth designing for, because it is silent. A mangled
 * common word looks like a typo and the meaning survives. A mangled proper
 * noun looks like a different subject, and everything downstream is confidently
 * about the wrong thing: asked what it thought of pnscripts.com, the brain gave
 * a considered opinion of pncryptz.com, and nothing in the exchange marked it
 * as a guess.
 *
 * So the shape is detected even when the word is not known. Something spelled
 * like a domain or an identifier, which nothing on this machine has ever seen,
 * is exactly the thing to repeat back before acting on it — the way a person
 * takes down a name over the telephone and reads it back.
 */

// identifierish matches things that carry an exact spelling: domains,
// hostnames, dotted or camel-cased identifiers, package names.
//
// These are worth checking precisely because being nearly right is no use. A
// domain that is one letter out is a different website, or nobody's.
var identifierish = regexp.MustCompile(
	`\b(?:[a-zA-Z0-9][a-zA-Z0-9-]*\.)+(?:com|net|org|io|dev|local|bg|co|app|sh|me)\b` +
		`|\b[a-z]+[A-Z][a-zA-Z]{2,}\b`)

/*
 * Unfamiliar returns names in a transcript that nothing here has seen before.
 *
 * Compared against the same vocabulary the recogniser was biased toward, so a
 * name the brain does know passes silently — it has been heard before, it was
 * spelled correctly this time, and asking about it every time would be worse
 * than useless.
 */
func Unfamiliar(text string) []string {
	return UnfamiliarIn(text, ConfidentPeak)
}

/*
 * ConfidentPeak is the loudness below which a transcript is not trusted with
 * names.
 *
 * Whisper does not decline. Given a second of near-silence it returns
 * something, and what it returns often has the shape of a name —
 * "siga.joshina.com", "breenit.dancie.anyupcens.com" were all produced from
 * audio peaking at 270, 411 and 183 against a room of 3, while real speech in
 * the same room peaked between 3200 and 6700.
 *
 * Asking about those is worse than ignoring them: it teaches nothing, it
 * interrupts, and each question is another sentence for the microphone to hear
 * and mishear again. The transcript is still used — a quiet sentence is still
 * a sentence — but nothing in it is treated as a name worth checking.
 */
const ConfidentPeak = 1200

// UnfamiliarIn returns names in a transcript loud enough to be believed.
func UnfamiliarIn(text string, peak int) []string {
	if peak > 0 && peak < ConfidentPeak {
		return nil
	}

	return unfamiliarNames(text)
}

func unfamiliarNames(text string) []string {
	found := identifierish.FindAllString(text, -1)
	if len(found) == 0 {
		return nil
	}

	known := knownVocabulary()

	var out []string

	for _, word := range found {
		lower := strings.ToLower(word)

		if known[lower] {
			continue
		}

		/*
		 * A name is only unfamiliar once its stem is unfamiliar too.
		 *
		 * "hosting.pnscripts.com" and "pnscripts.local" are the same name
		 * wearing different clothes, and somebody who has said one of them a
		 * thousand times does not want to be asked to confirm the other.
		 */
		if stem := stemOf(lower); stem != "" && known[stem] {
			continue
		}

		if !contains(out, word) {
			out = append(out, word)
		}
	}

	return out
}

// knownVocabulary is every name this machine has met, by their stems as well
// as in full.
func knownVocabulary() map[string]bool {
	vocabularyMu.RLock()
	words := knownWords
	vocabularyMu.RUnlock()

	known := map[string]bool{}

	if words == nil {
		return known
	}

	for _, memory := range words() {
		for _, match := range identifierish.FindAllString(memory, -1) {
			lower := strings.ToLower(match)

			known[lower] = true

			if stem := stemOf(lower); stem != "" {
				known[stem] = true
			}
		}
	}

	return known
}

/*
 * stemOf reduces a hostname to the part that identifies it.
 *
 * "hosting.pnscripts.com" and "pnscripts.local" both give "pnscripts", which
 * is the word somebody actually says and the word the recogniser gets wrong.
 * The suffix is never the difficult part: whisper has heard "dot com" many
 * more times than it has heard any particular name in front of it.
 */
func stemOf(word string) string {
	parts := strings.Split(word, ".")
	if len(parts) < 2 {
		return ""
	}

	// The label before the suffix: the second-to-last part.
	return parts[len(parts)-2]
}

func contains(list []string, want string) bool {
	for _, have := range list {
		if strings.EqualFold(have, want) {
			return true
		}
	}

	return false
}
