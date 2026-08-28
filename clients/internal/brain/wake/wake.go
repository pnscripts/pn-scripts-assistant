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
	// Text is what was said, with the name taken off the front when it was
	// used only to get attention: "Brain, what time is it" asks the time.
	Text string
}

// Listen decides whether a transcript was meant for the brain.
//
// An empty name means everything is. That is the default, and it is the default
// because requiring the name was tried and got in the way: transcription has to
// get the name right before anything can match it, and a name that is an
// abbreviation — or spoken in one language while the transcript is being made
// in another — is exactly the kind of thing it gets wrong. The result is an
// assistant that ignores its owner, which is a far worse failure than one that
// occasionally answers the television.
//
// The capability is kept rather than deleted, because the reason for wanting it
// was sound. It is switched on by naming a word to listen for.
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

	spoken := normalise(text)

	for _, candidate := range Names(name) {
		at := strings.Index(spoken, candidate)

		if at < 0 {
			continue
		}

		// Everything after the name is the request. If nothing follows, the
		// name on its own is how somebody gets attention before speaking.
		rest := strings.TrimSpace(afterWord(text, candidate))

		return Heard{Addressed: true, Text: rest}
	}

	return Heard{Text: text}
}

// Names are the ways somebody might say the brain's name.
//
// The last word on its own is included because that is what people actually
// say: an assistant called "PN Brain" gets called "Brain". Transcription is
// also imperfect, and a single common word survives it far better than a pair
// where one half is an abbreviation.
func Names(name string) []string {
	full := normalise(name)

	if full == "" {
		return nil
	}

	out := []string{full}

	if parts := strings.Fields(full); len(parts) > 1 {
		last := parts[len(parts)-1]

		if len(last) >= 3 {
			out = append(out, last)
		}
	}

	return out
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

// afterWord returns what follows the name in the original text.
//
// Works on the original rather than the normalised copy so that punctuation and
// capitalisation in the request survive: what the brain is asked should read the
// way it was said.
func afterWord(text, name string) string {
	words := strings.Fields(text)
	wanted := strings.Fields(name)

	for i := 0; i+len(wanted) <= len(words); i++ {
		match := true

		for j, part := range wanted {
			if normalise(words[i+j]) != part {
				match = false

				break
			}
		}

		if match {
			return strings.Join(words[i+len(wanted):], " ")
		}
	}

	return ""
}
