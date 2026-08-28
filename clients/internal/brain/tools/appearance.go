package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"pn-brain/internal/brain/appearance"
)

// SetAppearance changes what the core looks like.
//
// Safe rather than Mutating, and the distinction is worth being careful about
// because the rule this program runs on is that anything which changes
// something waits for a person. This changes nothing outside the program's own
// window, costs nothing, reaches nobody, and is undone by asking for the old
// colour back. Stopping to approve it would mean the owner asks for a colour,
// is shown a box asking whether they meant it, and clicks yes — which is not a
// safeguard, it is a way of teaching somebody to click yes without reading.
type SetAppearance struct {
	Look *appearance.Store
}

func (SetAppearance) Name() string { return "set_appearance" }

func (SetAppearance) Description() string {
	return "Change the colour of part of the interface. " +
		"Parts: 'thinking line' (the band that sweeps the core while working), " +
		"'core' (the sphere while working), 'speaking', 'listening', 'idle'. " +
		"Colours can be named (green, orange, cyan, purple, red, white) or hex."
}

func (SetAppearance) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"part": {"type": "string", "description": "thinking line, core, speaking, listening or idle"},
			"colour": {"type": "string", "description": "A colour name or hex value"}
		},
		"required": ["part", "colour"]
	}`)
}

func (SetAppearance) Risk() Risk { return Safe }

func (SetAppearance) Summarize(raw json.RawMessage) string {
	var a struct {
		Part   string `json:"part"`
		Colour string `json:"colour"`
	}
	argsOf(raw, &a)

	return fmt.Sprintf("Colour the %s %s", a.Part, a.Colour)
}

func (s SetAppearance) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Part   string `json:"part"`
		Colour string `json:"colour"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if s.Look == nil {
		return "", fmt.Errorf("there is no interface to change")
	}

	hex, err := s.Look.Set(a.Part, a.Colour)
	if err != nil {
		return "", err
	}

	// The page picks this up within a second, so the change is visible by the
	// time the sentence saying so is read.
	return fmt.Sprintf("The %s is now %s (%s). It takes effect immediately.", a.Part, a.Colour, hex), nil
}
