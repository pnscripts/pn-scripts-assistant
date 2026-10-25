package brain

import (
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Every capability the program registers says what it does in both
 * languages, so the Control page never shows a Bulgarian reader a bare name.
 */
func TestEveryCapabilitySaysWhatItDoesInBothLanguages(t *testing.T) {
	b := quietBrain(t)

	for _, tool := range b.Agent.Registry.All() {
		for _, lang := range []string{"en", "bg"} {
			if tools.Plain(tool.Name(), lang) == "" {
				t.Errorf("%s has no plain description in %s (tools/plain.go)", tool.Name(), lang)
			}
		}
	}
}
