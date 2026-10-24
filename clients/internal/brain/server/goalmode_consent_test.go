package server

import (
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

// What Settings shows under "Work on goals on its own" is word for word what
// the program enforces and setup agrees to.
func TestSettingsShowTheGoalModeConsentWordForWord(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	m := regexp.MustCompile(`(?s)id="goal-mode-consent">(.*?)</small>`).FindSubmatch(page)
	if m == nil {
		t.Fatal("the consent note is missing from Settings")
	}

	shown := strings.Join(strings.Fields(string(m[1])), " ")

	if want := tools.GoalModeConsent(false); shown != want {
		t.Errorf("Settings show:\n%s\nthe program says:\n%s", shown, want)
	}
}
