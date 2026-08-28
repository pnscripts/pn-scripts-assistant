package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Listening is what the wake-word tool needs from the brain.
//
// An interface rather than the brain itself, because a tool that imported the
// brain would be a tool the brain could not hold.
type Listening interface {
	WakeWord() string
	SetWakeWord(word string) error
}

// SetWakeWord decides whether the brain answers everything it hears, or only
// what follows a chosen word.
//
// Safe, on the same reasoning as the appearance tool: it changes nothing
// outside this program, costs nothing, reaches nobody, and is undone by asking.
// The one hazard is being locked out of talking by a word transcription cannot
// hear — so the answer says the word back, typing is never gated by it, and it
// is visible and changeable in the settings.
type SetWakeWord struct {
	Brain Listening
}

func (SetWakeWord) Name() string { return "set_wake_word" }

func (SetWakeWord) Description() string {
	return "Choose a word that must be said before the brain answers what it hears, " +
		"or clear it so it answers everything. Use when asked to only respond when " +
		"called by a name, or to stop requiring one."
}

func (SetWakeWord) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"word": {"type": "string", "description": "The word to listen for. Empty or 'off' to answer everything."}
		},
		"required": ["word"]
	}`)
}

func (SetWakeWord) Risk() Risk { return Safe }

func (SetWakeWord) Summarize(raw json.RawMessage) string {
	var a struct {
		Word string `json:"word"`
	}
	argsOf(raw, &a)

	if clearing(a.Word) {
		return "Answer everything heard"
	}

	return "Only answer after hearing " + a.Word
}

func (s SetWakeWord) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Word string `json:"word"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if s.Brain == nil {
		return "", fmt.Errorf("there is nothing to set")
	}

	word := strings.TrimSpace(a.Word)

	if clearing(word) {
		if err := s.Brain.SetWakeWord(""); err != nil {
			return "", err
		}

		return "I will answer anything I hear now.", nil
	}

	// A word transcription will not recognise leaves somebody unable to talk to
	// the brain at all, and the failure is silent — nothing happens, and there
	// is no way to tell why from the outside.
	if len(strings.Fields(word)) > 2 {
		return "", fmt.Errorf("%q is too long to pick out reliably; one or two words works", word)
	}

	if err := s.Brain.SetWakeWord(word); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"From now on I will only answer when I hear %q. Say %q followed by what you want, "+
			"and I will keep listening for a while afterwards so you need not repeat it. "+
			"Typing always works, and you can tell me to answer everything again.",
		word, word), nil
}

func clearing(word string) bool {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "", "off", "none", "nothing", "anything", "everything":
		return true
	}

	return false
}
