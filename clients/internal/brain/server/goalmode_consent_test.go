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

	// And in Bulgarian, word for word what setup agreed to (i18n.js).
	en, bg := strings2(t, readAsset(t, "js/i18n.js"), "sys.goalConsent")
	if en != tools.GoalModeConsent(false) {
		t.Errorf("i18n.js says in English:\n%s\nthe program says:\n%s", en, tools.GoalModeConsent(false))
	}

	if bg != tools.GoalModeConsent(true) {
		t.Errorf("i18n.js says in Bulgarian:\n%s\nthe program says:\n%s", bg, tools.GoalModeConsent(true))
	}

	if !regexp.MustCompile(`data-i18n="sys.goalConsent"[^>]*id="goal-mode-consent"|id="goal-mode-consent"[^>]*data-i18n="sys.goalConsent"`).Match(page) {
		t.Error("the consent note on Settings does not name its string in i18n.js")
	}
}
