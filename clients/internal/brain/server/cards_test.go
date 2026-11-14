package server

import (
	"encoding/json"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Every tool the program registers says its approval card in Bulgarian.
 *
 * Each is asked for its card twice, with no arguments and with every
 * argument its schema names filled in, so both the bare branch and the full
 * one are read. The English card must stay what Summarize says (the model,
 * the deeds and the rules read it), and the Bulgarian one must be in
 * Bulgarian. A tool added without a Bulgarian card fails here.
 */
func TestEveryToolsCardIsInBulgarian(t *testing.T) {
	_, _, b := newServer(t)

	all := b.Agent.Registry.All()
	if len(all) < 100 {
		t.Fatalf("only %d tools registered: the list is not the program's", len(all))
	}

	for _, tool := range all {
		if _, ok := tool.(tools.CardSaying); !ok {
			t.Errorf("%s has no card in Bulgarian (no SummarizeIn)", tool.Name())

			continue
		}

		for _, args := range []json.RawMessage{json.RawMessage(`{}`), sampleArguments(tool.Parameters())} {
			en := tools.SummaryIn(tool, args, "en")
			if en != tool.Summarize(args) {
				t.Errorf("%s %s: the English card changed:\n%q\n%q", tool.Name(), args, en, tool.Summarize(args))
			}

			if bg := tools.SummaryIn(tool, args, "bg"); !tools.SaysBulgarian(bg) {
				t.Errorf("%s %s: the card is not in Bulgarian: %q", tool.Name(), args, bg)
			}
		}
	}
}

// sampleArguments fills every argument a schema names with a value of its
// type (an enum's first value).
func sampleArguments(schema json.RawMessage) json.RawMessage {
	var s struct {
		Properties map[string]struct {
			Type  any   `json:"type"`
			Enum  []any `json:"enum"`
			Items *struct {
				Type any `json:"type"`
			} `json:"items"`
		} `json:"properties"`
	}

	if json.Unmarshal(schema, &s) != nil {
		return json.RawMessage(`{}`)
	}

	out := map[string]any{}

	for name, p := range s.Properties {
		kind, _ := p.Type.(string)

		switch {
		case len(p.Enum) > 0:
			out[name] = p.Enum[0]
		case kind == "integer" || kind == "number":
			out[name] = 2
		case kind == "boolean":
			out[name] = true
		case kind == "array":
			out[name] = []any{"zz-sample"}
		case kind == "object":
			out[name] = map[string]any{}
		default:
			out[name] = "zz-sample"
		}
	}

	raw, _ := json.Marshal(out)

	return raw
}
