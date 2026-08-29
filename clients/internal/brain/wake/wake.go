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
			if normalise(words[i+j]) != part {
				match = false

				break
			}
		}

		if match {
			return i, len(wanted)
		}
	}

	return -1, 0
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
