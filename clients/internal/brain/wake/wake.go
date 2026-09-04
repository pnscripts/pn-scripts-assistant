// Package wake decides whether something said was addressed to the brain.
//
// A microphone left open hears everything in the room: a television, somebody
// else talking, the brain's own voice coming back off the speakers. Without a
// name to listen for, all of it becomes a turn — the brain answers the room,
// and its owner watches it hold a conversation with a doorbell.
//
// So it waits to be addressed, and then stays engaged for a while, because
// having to say the name before every sentence is not a conversation either.
package wake

import (
	"strings"
	"unicode"
)

// Heard is what a transcript turned out to be.
type Heard struct {
	// Addressed is true when the brain should act on this.
	Addressed bool
	// Text is what was said with the name taken out of it, from wherever in
	// the sentence it appeared: both "Brain, what time is it" and "what time
	// is it, brain" ask the time.
	Text string
}

// Listen decides whether a transcript was meant for the brain.
//
// An empty name means everything is, which is the right behaviour for a
// headset and the wrong one for a room. With a name set, nothing that does not
// carry it gets through.
//
// engaged is whether it is already in a conversation, in which case anything
// said counts.
func Listen(transcript, name string, engaged bool) Heard {
	text := strings.TrimSpace(transcript)

	if text == "" {
		return Heard{}
	}

	if strings.TrimSpace(name) == "" || engaged {
		return Heard{Addressed: true, Text: text}
	}

	words := strings.Fields(text)

	for _, candidate := range Names(name) {
		at, length := find(words, strings.Fields(candidate))

		if at < 0 {
			continue
		}

		// A word of attention in front of the name belongs to the name, not to
		// the request: "okay brain, turn the lights off" is not asking about
		// okay.
		from := at

		if at > 0 && filler[normalise(words[at-1])] {
			from = at - 1
		}

		// What is left once the name is out of it — from both sides, because
		// the name is as often at the end as the start, and taking only what
		// followed it threw the question away.
		rest := append(append([]string{}, words[:from]...), words[at+length:]...)

		return Heard{Addressed: true, Text: tidy(strings.Join(rest, " "))}
	}

	return Heard{Text: text}
}

// Ends reports a sentence that closes the conversation.
//
// Without this, being engaged means the next forty seconds of the room are the
// brain's business: turn to say something to somebody else and it answers. A
// person leaving a conversation says so, and it costs nothing to listen for it.
func Ends(text string) bool {
	switch tidy(normalise(text)) {
	case "thanks", "thank you", "thanks brain", "thank you brain",
		"that is all", "thats all", "that will be all",
		"stop", "stop listening", "never mind", "nevermind",
		"goodbye", "bye", "bye bye", "go to sleep":
		return true
	}

	return false
}

// Names are the ways somebody might say the brain's name.
//
// Separated by commas, so its owner can list what it actually gets called.
// That list is the answer to transcription being imperfect, and it is a better
// answer than the one tried first: forgiving a wrong letter automatically.
//
// Forgiving one letter cannot work here, and the arithmetic says why. Whisper
// writes "Brain" as "Bryan", which is two edits away, not one — while "rain"
// is one edit away and is a word a film will say. Any rule loose enough to
// catch the mishearing is loose enough to wake on the weather. Whoever owns
// the brain knows what their microphone writes down, and can say so; the
// interface shows them what it heard and ignored so they can find out.
//
// The last word of a name is included on its own because that is what people
// actually say: an assistant called "PN Brain" gets called "Brain".
func Names(name string) []string {
	var out []string

	seen := map[string]bool{}

	add := func(candidate string) {
		if candidate == "" || seen[candidate] {
			return
		}

		seen[candidate] = true
		out = append(out, candidate)
	}

	for _, part := range strings.Split(name, ",") {
		full := normalise(part)

		if full == "" {
			continue
		}

		add(full)

		if words := strings.Fields(full); len(words) > 1 {
			if last := words[len(words)-1]; len([]rune(last)) >= 3 {
				add(last)
			}
		}
	}

	return out
}

// filler is what people put in front of a name to get attention.
var filler = map[string]bool{
	"hey": true, "hi": true, "hello": true, "ok": true, "okay": true, "yo": true,
}

// find locates the name in what was said, as whole words.
//
// Whole words because matching letter-by-letter through the sentence had a
// television saying "brains" wake a brain called Brain, and then hand it an
// empty request, since the part that pulled the name back out did match whole
// words. One rule for both, and the two cannot disagree.
//
// Returns where the name starts and how many words it took, or -1.
func find(words, wanted []string) (int, int) {
	if len(wanted) == 0 {
		return -1, 0
	}

	for i := 0; i+len(wanted) <= len(words); i++ {
		match := true

		for j, part := range wanted {
			word := normalise(words[i+j])

			if word == part {
				continue
			}

			/*
			 * The last word of the name may have the next one welded to it.
			 *
			 * Only the last, because that is where it happens: the recogniser
			 * runs the name into whatever follows and writes one word.
			 * Measured on this machine, "Brain, tell me what you are going to
			 * do" came back as "brainlue, tell me what you are going to do",
			 * and a whole-word match threw the whole turn away — which is what
			 * "it does not listen for its name" is, seen from a chair.
			 */
			if j == len(wanted)-1 && weldedOnto(word, part) {
				continue
			}

			match = false

			break
		}

		if match {
			return i, len(wanted)
		}
	}

	return -1, 0
}

/*
 * weldedOnto reports a name with the next word run into it by the recogniser.
 *
 * Narrow on purpose, because the failure on the other side is worse. The rule
 * this softens exists because a television saying "brains" woke a brain called
 * Brain, and anything loose enough to catch every mishearing is loose enough
 * to answer the weather.
 *
 * So: the word has to begin with the whole name, and what is left over has to
 * be two to four letters that are not an ordinary ending. "brainlue" is the
 * recogniser welding a syllable on. "brains", "brained", "braining" are
 * English, and are refused — those are the ones a room actually says.
 */
func weldedOnto(word, name string) bool {
	if len([]rune(name)) < 4 || !strings.HasPrefix(word, name) {
		return false
	}

	tail := word[len(name):]

	switch len([]rune(tail)) {
	case 2, 3, 4:
	default:
		return false
	}

	for _, ending := range []string{"s", "es", "ed", "er", "ing", "ish", "ly", "y"} {
		if tail == ending {
			return false
		}
	}

	return true
}

// normalise lowers the case and drops anything that is not a letter, digit or
// space, so that "Brain," and "brain" and "Brain?" are the same word.
func normalise(text string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// tidy cleans up what is left after a word has been lifted out of a sentence.
//
// Separators go, because lifting the name out of "what time is it, brain"
// leaves a comma hanging off the end of the question. What ends a sentence
// stays: a question mark is part of what was asked, not punctuation left
// behind by the edit.
func tidy(text string) string {
	const (
		joins = ",;:-—–"
		ends  = ".!?"
	)

	return strings.TrimSpace(strings.TrimRight(
		strings.TrimLeft(strings.TrimSpace(text), joins+ends+" "), joins+" "))
}

/*
 * NearMiss reports a word that was nearly the name.
 *
 * The point is not to wake on it — deciding that is find's job and it is
 * deliberately strict. The point is to be able to say so. "No name in it" and
 * "you said something that sounded like my name and I did not think it was"
 * are completely different problems with completely different fixes, and until
 * they are told apart, a recogniser that mangles the name looks exactly like
 * an assistant that is not listening.
 *
 * Returns the word that nearly matched, or empty.
 */
func NearMiss(transcript, name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}

	words := strings.Fields(transcript)

	for _, candidate := range Names(name) {
		wanted := strings.Fields(candidate)

		if len(wanted) != 1 {
			continue
		}

		want := wanted[0]

		if len([]rune(want)) < 4 {
			continue
		}

		for _, raw := range words {
			word := normalise(raw)

			if word == "" || word == want {
				continue
			}

			/*
			 * The name buried inside a longer word, or a word one edit away
			 * from it.
			 *
			 * Both are what the recogniser actually produces: "brainlue" has
			 * the name welded to the next syllable, and "brian" is the two
			 * middle letters swapped, which is the commonest single thing that
			 * happens to this name.
			 *
			 * Not everything is catchable. "Piembring" for "PN Brain" shares
			 * four letters with it and would need a rule loose enough to match
			 * half the dictionary, so it goes unremarked — the log still shows
			 * what was heard, which is the part that matters.
			 */
			if strings.Contains(word, want) || oneEditApart(word, want) {
				return word
			}
		}
	}

	return ""
}

/*
 * oneEditApart reports two words within a single insertion, deletion or
 * substitution of each other.
 *
 * Written out rather than a full edit distance, because one edit is all this
 * needs and the general version invites somebody to raise the number later —
 * which is how a wake word starts answering the weather.
 */
func oneEditApart(a, b string) bool {
	x, y := []rune(a), []rune(b)

	if len(x) < len(y) {
		x, y = y, x
	}

	if len(x)-len(y) > 1 {
		return false
	}

	var i, j, edits int

	for i < len(x) && j < len(y) {
		if x[i] == y[j] {
			i++
			j++

			continue
		}

		edits++

		if edits > 1 {
			return false
		}

		if len(x) == len(y) {
			/*
			 * Two letters the wrong way round counts as one edit.
			 *
			 * Which is not textbook and is the point: "brian" for "brain" is
			 * the single commonest thing said about this name, by people as
			 * well as by recognisers, and calling it two edits away puts it
			 * outside every rule here.
			 */
			if i+1 < len(x) && x[i] == y[j+1] && x[i+1] == y[j] {
				i += 2
				j += 2

				continue
			}

			i++
			j++

			continue
		}

		i++
	}

	return edits+(len(x)-i)+(len(y)-j) <= 1
}
