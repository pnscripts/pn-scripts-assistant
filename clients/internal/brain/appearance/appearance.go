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

	/*
	 * The rest of the statuses, so that every part of the interface can be
	 * coloured from one list.
	 *
	 * They were not here before, and the consequence was that each panel
	 * invented its own. The core knew four states and drew them from these
	 * settings; the feed knew nine and hard-coded five tones of its own; the
	 * talk button had a third set in CSS. So the same moment was violet in one
	 * corner, cyan in another and amber in a third, and nothing on screen
	 * agreed with anything else about what the brain was doing.
	 *
	 * One list, sent to the page, written into CSS variables and used by
	 * everything — including the core, which no longer has a private palette.
	 */

	// Hearing is making out the words, after speech has been recorded.
	Hearing string `json:"hearing"`

	// Tool is running a tool: reading a file, searching, opening something.
	Tool string `json:"tool"`

	// Waiting is holding for a decision from its owner.
	Waiting string `json:"waiting"`

	// Learning is work the brain gave itself, which nobody is waiting on.
	Learning string `json:"learning"`
}

// Default is what it looks like before anybody asks for anything else.
// Default is what it looks like before anybody asks for anything else.
//
// Four states, four colours, chosen to be told apart at a glance rather than to
// sit nicely together: resting, hearing you, thinking, and speaking. A palette
// where two states are neighbouring blues is a palette that answers "what is it
// doing" with "something".
func Default() Look {
	return Look{
		ThinkingLine: "#7bffa8",
		// Working.
		ThinkingCore: "#f0b26b",
		// Its own voice.
		Speaking: "#7bffa8",
		/*
		 * Yours.
		 *
		 * Violet before, which was the odd one out: everything else on this
		 * screen is in the blue family and the microphone being open is the
		 * state somebody sees most, so the interface spent most of its life
		 * looking like a different program. Blue, but brighter and cooler than
		 * the resting blue below, because these two do have to be told apart —
		 * waiting for you and listening to you are not the same thing.
		 */
		Listening: "#4db8ff",
		// At rest.
		Idle: "#3d8ce8",

		/*
		 * Distinct at a glance rather than harmonious.
		 *
		 * The question this palette answers is "what is it doing", and it can
		 * only answer it if no two statuses are neighbouring shades. Hearing
		 * sits next to listening in meaning, so the two are kept apart by
		 * temperature rather than by hue: a plain blue while the microphone is
		 * open, cyan while the words are being worked out.
		 */

		// Working out the words.
		Hearing: "#5fe3f5",

		// Doing something in the world, which is the one worth noticing.
		Tool: "#ff9d5c",

		// Stopped, needing an answer from its owner.
		Waiting: "#ff6b6b",

		// Its own background work, deliberately quiet.
		Learning: "#4a8f8a",
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

	/*
	 * The rest of the statuses, by the names somebody would actually say.
	 *
	 * Several ways in for each, because this is reached by voice as well as by
	 * the panel and nobody says "the hearing status" — they say "when you are
	 * working out what I said". Every one of these is a phrase that could only
	 * mean the thing it maps to.
	 */
	"hearing":       "hearing",
	"making out":    "hearing",
	"understanding": "hearing",
	"transcribing":  "hearing",

	"tool":    "tool",
	"tools":   "tool",
	"working": "tool",
	"doing":   "tool",

	"waiting":  "waiting",
	"approval": "waiting",
	"asking":   "waiting",

	"learning":   "learning",
	"background": "learning",
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

	if saved.Hearing != "" {
		s.look.Hearing = saved.Hearing
	}

	if saved.Tool != "" {
		s.look.Tool = saved.Tool
	}

	if saved.Waiting != "" {
		s.look.Waiting = saved.Waiting
	}

	if saved.Learning != "" {
		s.look.Learning = saved.Learning
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
		return "", fmt.Errorf(
			"I do not know what %q is. I can colour: idle, listening, hearing, "+
				"thinking, tool, speaking, waiting, learning, or the thinking line",
			part)
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
	case "hearing":
		s.look.Hearing = hex
	case "tool":
		s.look.Tool = hex
	case "waiting":
		s.look.Waiting = hex
	case "learning":
		s.look.Learning = hex
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
