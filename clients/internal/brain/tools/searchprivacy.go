package tools

import (
	"fmt"
	"regexp"
	"strings"
)

/*
 * What must never end up in a search query.
 *
 * Searching the web is the one thing this program does that leaves the
 * machine, and the query is composed by the model rather than typed by the
 * person. That is the whole of the risk. Asked "what is the weather in Sofia"
 * the model searches for the weather in Sofia; asked "when is my appointment
 * with Dr Ivanov about the test results" it may well search for exactly that,
 * and a search engine is not a place anybody meant to put it.
 *
 * The rule this program is held to is that no personal information is shared.
 * That has to be enforced where the data leaves, not asked for in a prompt: a
 * prompt is a request to a model, and a model can be argued out of it by the
 * user, by a page it has read, or by its own confusion. This cannot be argued
 * with.
 *
 * Deliberately narrow. It blocks the things that are identifying no matter the
 * context, and lets ordinary questions through untouched — a filter that stops
 * "weather in Sofia" because Sofia is a place somebody lives is a filter that
 * gets switched off, and then nothing is checked at all.
 */

var (
	// An email address is identifying wherever it appears.
	emailInQuery = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]{2,}`)

	// Long digit runs: card numbers, identity numbers, account numbers, and
	// telephone numbers with or without separators. Short numbers are left
	// alone, because years, prices and quantities are ordinary search terms.
	longNumber = regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`)

	// A key, token or password the model picked up while reading a file.
	secretish = regexp.MustCompile(`(?i)\b(api[_-]?key|token|password|passwd|secret|bearer)\b`)
)

/*
 * PrivateEnough reports whether a query may be sent to a search engine.
 *
 * Returns the reason when it may not, so the model is told what was wrong and
 * can search for the general question instead of the personal one. Silently
 * dropping the search would produce the failure this program has produced
 * before: the model concludes the web has nothing on the subject and answers
 * from imagination.
 */
func PrivateEnough(query string) (bool, string) {
	if emailInQuery.MatchString(query) {
		return false, "that search contains an email address, which is not " +
			"something to hand a search engine. Search for the general question instead"
	}

	if longNumber.MatchString(query) {
		return false, "that search contains a long number — a telephone, card or " +
			"account number. Search for the general question instead"
	}

	if secretish.MatchString(query) {
		return false, "that search looks like it contains a key or a password. " +
			"Search for the general question instead"
	}

	return true, ""
}

// CheckQuery is PrivateEnough as an error, for use at the point of sending.
func CheckQuery(query string) error {
	if ok, why := PrivateEnough(strings.TrimSpace(query)); !ok {
		return fmt.Errorf("%s", why)
	}

	return nil
}
