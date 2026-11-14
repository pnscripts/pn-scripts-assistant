package brain

import (
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

// Each way to reach Claude is named in Bulgarian too, for the card.
func TestEveryWayToClaudeIsNamedInBulgarian(t *testing.T) {
	b := testBrain(t)

	for _, w := range b.ClaudeWays() {
		if w.Title == "" || !tools.SaysBulgarian(w.TitleBG) {
			t.Errorf("%s: %q / %q", w.ID, w.Title, w.TitleBG)
		}
	}
}
