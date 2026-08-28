// Package appearance holds what the interface looks like, so the brain can
// change it when asked.
//
// This exists because of a conversation. Its owner asked, by voice, for the
// line that sweeps the core while it is thinking to be one colour and the rest
// of the core to be another. The brain said "Understood", then "Got it", then
// — after being asked a third time — admitted it had no way to change anything
// on screen at all. Three agreements and no action.
//
// That is the failure this whole program is built against, and the answer is
// not a better refusal. It is to make the thing it agreed to actually possible,
// so that agreeing and doing are the same act.
package appearance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Look is what the core is coloured with.
//
// Only what somebody has actually asked to change lives here. A settings file
// full of everything that could conceivably be adjusted is a settings file
// nobody reads.
type Look struct {
	// ThinkingLine is the band that sweeps the sphere while it is working.
	ThinkingLine string `json:"thinking_line"`
	// ThinkingCore is the sphere itself while it is working.
	ThinkingCore string `json:"thinking_core"`
	// Speaking and Listening are the sphere while it is doing those.
	Speaking  string `json:"speaking"`
	Listening string `json:"listening"`
	// Idle is the sphere when nothing is happening.
	Idle string `json:"idle"`
}

// Default is what it looks like before anybody asks for anything else.
func Default() Look {
	return Look{
		ThinkingLine: "#7bffa8",
		ThinkingCore: "#f0b26b",
		Speaking:     "#7bffa8",
		Listening:    "#5fe3f5",
		Idle:         "#5fe3f5",
	}
}

// Named colours somebody might reasonably say out loud.
//
// Spoken instructions arrive as words, not as hex. "Orange" has to mean
// something or the tool is only usable by somebody willing to say "hash eff
// zero bee two six bee".
var named = map[string]string{
	"green":   "#7bffa8",
	"cyan":    "#5fe3f5",
	"blue":    "#4d8ce1",
	"orange":  "#f0b26b",
	"amber":   "#f0b26b",
	"red":     "#e06c75",
	"yellow":  "#e8d16b",
	"purple":  "#b39ddb",
	"violet":  "#b39ddb",
	"pink":    "#e58fb8",
	"white":   "#dff8fd",
	"grey":    "#8fa3b8",
	"gray":    "#8fa3b8",
	"teal":    "#4dd0e1",
	"magenta": "#e06cd0",
}

// Colour turns what somebody said into something that can be drawn.
//
// Returns false rather than guessing. A colour nobody recognises should be
// reported back as unknown, not silently rendered as black — which is exactly
// how a setting ends up appearing to have been ignored.
func Colour(said string) (string, bool) {
	text := strings.ToLower(strings.TrimSpace(said))

	if hex, known := named[text]; known {
		return hex, true
	}

	text = strings.TrimPrefix(text, "#")

	if len(text) == 6 || len(text) == 3 {
		for _, c := range text {
			if !strings.ContainsRune("0123456789abcdef", c) {
				return "", false
			}
		}

		return "#" + text, true
	}

	return "", false
}

// Parts are the things that can be coloured, by the names somebody would use.
var Parts = map[string]string{
	"thinking line": "thinking_line",
	"thinking":      "thinking_core",
	"thinking core": "thinking_core",
	"core":          "thinking_core",
	"speaking":      "speaking",
	"listening":     "listening",
	"idle":          "idle",
}

// Store keeps the look on disk.
type Store struct {
	path string

	mu   sync.RWMutex
	look Look
}

// Open loads the look, or starts from the default.
func Open(root string) *Store {
	s := &Store{path: filepath.Join(root, "appearance.json"), look: Default()}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}

	var saved Look

	if err := json.Unmarshal(raw, &saved); err != nil {
		return s
	}

	// Merged over the default rather than replacing it, so a file written by an
	// older version is missing fields rather than blanking them.
	if saved.ThinkingLine != "" {
		s.look.ThinkingLine = saved.ThinkingLine
	}

	if saved.ThinkingCore != "" {
		s.look.ThinkingCore = saved.ThinkingCore
	}

	if saved.Speaking != "" {
		s.look.Speaking = saved.Speaking
	}

	if saved.Listening != "" {
		s.look.Listening = saved.Listening
	}

	if saved.Idle != "" {
		s.look.Idle = saved.Idle
	}

	return s
}

// Current reports the look.
func (s *Store) Current() Look {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.look
}

// Set changes one part and writes it down.
func (s *Store) Set(part, colour string) (string, error) {
	field, known := Parts[strings.ToLower(strings.TrimSpace(part))]
	if !known {
		return "", fmt.Errorf("I do not know what %q is. I can colour the thinking line, the core, speaking, listening or idle", part)
	}

	hex, ok := Colour(colour)
	if !ok {
		return "", fmt.Errorf("I do not know the colour %q", colour)
	}

	s.mu.Lock()

	switch field {
	case "thinking_line":
		s.look.ThinkingLine = hex
	case "thinking_core":
		s.look.ThinkingCore = hex
	case "speaking":
		s.look.Speaking = hex
	case "listening":
		s.look.Listening = hex
	case "idle":
		s.look.Idle = hex
	}

	look := s.look
	s.mu.Unlock()

	raw, err := json.MarshalIndent(look, "", "  ")
	if err != nil {
		return hex, err
	}

	// Written down, because a change that does not survive a restart is a
	// change somebody has to keep asking for.
	return hex, os.WriteFile(s.path, raw, 0o600)
}
