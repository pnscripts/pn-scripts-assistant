package speech

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

/*
 * Teaching the recogniser the words this particular person uses.
 *
 * Whisper knows the language and not the speaker. Handed a name it has never
 * seen it does not hesitate or mark the word uncertain — it returns the
 * nearest thing in its vocabulary, confidently. "pnscripts.com" came back as
 * "pncryptz.com", and the brain then gave a thoughtful answer about a website
 * that does not exist. "PN Brain" has come back as "Piembring" and "Piendren".
 *
 * That failure is worse than mishearing an ordinary word, because there is
 * nothing in the sentence to give it away. A wrong common word reads as a typo
 * and the meaning survives; a wrong proper noun reads as a different subject
 * entirely, and every step after it is about the wrong thing.
 *
 * Whisper takes an initial prompt and biases decoding toward it, which is the
 * mechanism intended for exactly this. What goes in it should be the names
 * this person actually says — and the brain already has them, because it has
 * been reading their files and their browser history for weeks. The most
 * visited site on this machine is pnscripts.local, at eleven hundred visits.
 * Nothing was passing that to the recogniser.
 */

// KnownWords supplies the names worth biasing toward, newest or most-used
// first. Set by the brain, which is the part that has a memory.
//
// A function rather than a list because memory grows while the program runs:
// something learned this morning should be recognised this afternoon without
// a restart.
var (
	vocabularyMu sync.RWMutex
	knownWords   func() []string
)

// SetVocabulary tells the recogniser where to find this person's own words.
func SetVocabulary(words func() []string) {
	vocabularyMu.Lock()
	knownWords = words
	vocabularyMu.Unlock()
}

/*
 * MostPromptTokens caps how much is sent.
 *
 * Whisper documents the limit as half its text context, and going over does
 * not fail loudly — it truncates, which would silently drop whichever names
 * happened to sort last. Kept well under, because a long prompt also biases
 * the model toward producing the prompt itself: ask it to expect forty domain
 * names and it will start hearing domain names in the pauses.
 */
const MostPromptTokens = 90

// distinctive matches the shapes of word worth teaching: domains, hostnames,
// and CamelCase or dotted identifiers.
//
// Deliberately not "any capitalised word". Ordinary names — London, Monday,
// Petar — are already known, and filling the prompt with them spends the
// budget without buying anything.
var distinctive = regexp.MustCompile(
	`\b(?:[a-zA-Z0-9][a-zA-Z0-9-]*\.)+(?:com|net|org|io|dev|local|bg|co|app|sh|me)\b` +
		`|\b[a-z]+[A-Z][a-zA-Z]{2,}\b`)

/*
 * PromptFrom picks the names worth teaching out of what the brain knows.
 *
 * Ordered by how often each appears, because a name mentioned in six memories
 * is one this person says out loud and a name mentioned once is probably from
 * a file they opened by accident.
 */
func PromptFrom(memories []string) string {
	seen := map[string]int{}
	order := map[string]int{}

	for i, memory := range memories {
		for _, match := range distinctive.FindAllString(memory, -1) {
			key := strings.ToLower(match)

			seen[key]++

			if _, had := order[key]; !had {
				order[key] = i
			}
		}
	}

	if len(seen) == 0 {
		return ""
	}

	words := make([]string, 0, len(seen))
	for word := range seen {
		words = append(words, word)
	}

	sort.SliceStable(words, func(i, j int) bool {
		if seen[words[i]] != seen[words[j]] {
			return seen[words[i]] > seen[words[j]]
		}

		return order[words[i]] < order[words[j]]
	})

	/*
	 * Written as a sentence rather than a list.
	 *
	 * The initial prompt is treated as though it were the transcript of what
	 * came just before, so it works best as something a person could have
	 * said. A bare comma-separated list biases toward producing bare
	 * comma-separated lists, which is its own kind of wrong.
	 */
	var (
		kept   []string
		budget int
	)

	for _, word := range words {
		// Roughly a token per four characters, plus one for the separator.
		cost := len(word)/4 + 2
		if budget+cost > MostPromptTokens {
			break
		}

		kept = append(kept, word)
		budget += cost
	}

	if len(kept) == 0 {
		return ""
	}

	return "We are talking about " + strings.Join(kept, ", ") + "."
}

// Prompt is the initial prompt for this machine, or "" if nothing is known.
func Prompt() string {
	vocabularyMu.RLock()
	words := knownWords
	vocabularyMu.RUnlock()

	if words == nil {
		return ""
	}

	return PromptFrom(words())
}

// withVocabulary adds the initial prompt to a whisper command line.
func withVocabulary(args []string) []string {
	prompt := Prompt()
	if prompt == "" {
		return args
	}

	// carry-initial-prompt, or the bias applies only to the first window and a
	// name said thirty seconds into a turn is heard as wrongly as before.
	return append(args, "--prompt", prompt, "--carry-initial-prompt")
}
