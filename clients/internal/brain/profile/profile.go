/*
 * Package profile is what the assistant knows about the person it works for.
 *
 * Prose, written by its owner, in a file they can open. Not a form: what
 * matters about somebody's work is not a set of fields, and a form would ask
 * for a job title where what is actually needed is "I bill in advance and I
 * never chase late payments myself".
 *
 * It is separate from what the brain has learned. Learned facts are scraped
 * from this machine's disk and quarantined before they count; this is written
 * deliberately, for the assistant to use, and is trusted the moment it is
 * saved. Those are different things and conflating them would mean either
 * making somebody approve their own words or letting a scraped guess sit
 * beside them as though it had been.
 */
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FileName is the file, in the brain's own folder, so that it travels with the
// brain and is editable by hand.
const FileName = "profile.md"

/*
 * MostRunes is as much of it as is kept.
 *
 * Everything here is read again on every single turn, and reading the prompt
 * is most of the wait on a processor that manages ten tokens a second. Four
 * thousand characters is roughly a thousand tokens, which is already a minute
 * and a half of reading before the question has been reached — so this is a
 * ceiling rather than a target, and the panel says so.
 */
const MostRunes = 4000

// SpokenRunes is how much of it a spoken turn gets: the opening, and only
// while it is short. A voice exchange is unbearable if every reply begins with
// a minute of silence, which is what a page of profile costs out loud.
const SpokenRunes = 400

// Path is where the profile lives for a given brain.
func Path(root string) string { return filepath.Join(root, FileName) }

// Read returns what its owner wrote, or empty when they have written nothing.
// A missing file is the ordinary case and is not an error.
func Read(root string) (string, error) {
	raw, err := os.ReadFile(Path(root))

	if os.IsNotExist(err) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("reading %s: %w", FileName, err)
	}

	return strings.TrimSpace(string(raw)), nil
}

// Write saves it, trimmed to what will be read back on every turn.
func Write(root, text string) error {
	text = strings.TrimSpace(text)

	if utf8.RuneCountInString(text) > MostRunes {
		text = string([]rune(text)[:MostRunes])
	}

	if text == "" {
		// Cleared rather than left as an empty file, so that "there is no
		// profile" is one state rather than two that behave the same.
		err := os.Remove(Path(root))

		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clearing %s: %w", FileName, err)
		}

		return nil
	}

	// Written whole and moved into place, like everything else in this folder:
	// a file half written by a program that was killed is worse than none.
	temp := Path(root) + ".new"

	if err := os.WriteFile(temp, []byte(text+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}

	return os.Rename(temp, Path(root))
}

/*
 * Block is the profile as it appears in the prompt, or empty.
 *
 * Labelled, and labelled as facts rather than as instructions. Prose dropped
 * into a prompt unannounced is read as something to respond to — the first
 * version of this produced an assistant that opened by commenting on its
 * owner's business.
 */
func Block(root string, most int) string {
	text, err := Read(root)
	if err != nil || text == "" {
		return ""
	}

	if most > 0 && utf8.RuneCountInString(text) > most {
		text = shortened(text, most)
	}

	return "What you have been told about the person you work for. These are " +
		"facts to work from, not something to reply to:\n\n" + text
}

/*
 * shortened cuts at a paragraph rather than mid-sentence.
 *
 * For the spoken turn, where the whole profile is more silence than anybody
 * will sit through. Half a sentence about somebody's business is worse than
 * the first paragraph of it, so this stops at the last break that fits and
 * only falls back to a hard cut when the first paragraph is itself too long.
 */
func shortened(text string, most int) string {
	if kept := firstThatFits(text, most); kept != "" {
		return kept
	}

	runes := []rune(text)

	return strings.TrimSpace(string(runes[:most])) + "…"
}

func firstThatFits(text string, most int) string {
	var kept string

	for _, paragraph := range strings.Split(text, "\n\n") {
		candidate := strings.TrimSpace(kept + "\n\n" + paragraph)

		if utf8.RuneCountInString(candidate) > most {
			break
		}

		kept = candidate
	}

	return strings.TrimSpace(kept)
}
