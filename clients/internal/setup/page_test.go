package setup

import (
	"strings"
	"testing"
)

// stepHTML is the markup of one step of the page.
func stepHTML(t *testing.T, id string) string {
	t.Helper()

	start := strings.Index(setupPage, `<div class="step" id="step-`+id+`"`)
	if start < 0 {
		t.Fatalf("no step %q", id)
	}

	rest := setupPage[start+1:]
	end := strings.Index(rest, `<div class="step" id="step-`)

	if end < 0 {
		return rest
	}

	return rest[:end]
}

/*
 * Setup asks one question per screen, the recommended answer first and
 * marked (owner 2026-10-23): how much it asks and what may leave were one
 * screen, and are two now.
 */
func TestSetupAsksOneQuestionPerScreenWithTheRecommendedFirst(t *testing.T) {
	if strings.Contains(stepHTML(t, "privacy"), `id="privacy-pick"`) {
		t.Fatal("what may leave is still on the same screen as how much it asks")
	}

	if !strings.Contains(stepHTML(t, "leave"), `id="privacy-pick"`) {
		t.Fatal("what may leave has no screen of its own")
	}

	if strings.Contains(stepHTML(t, "name"), `id="voice-pick"`) || !strings.Contains(stepHTML(t, "voice"), `id="voice-pick"`) {
		t.Fatal("the name and the voice are still one screen")
	}

	order := []string{`{id: "name"`, `{id: "voice"`, `{id: "privacy"`, `{id: "leave"`, `{id: "apply"`}
	at := -1

	for _, step := range order {
		i := strings.Index(setupPage, step)
		if i < at {
			t.Fatalf("%s is out of order", step)
		}

		at = i
	}

	for _, want := range []string{
		`RECOMMENDED = {freedom: "everything", privacy: "open"}`,
		`recommendedFirst(FREEDOM, "level", RECOMMENDED.freedom)`,
		`recommendedFirst(state.privacies, "mode", RECOMMENDED.privacy)`,
	} {
		if !strings.Contains(setupPage, want) {
			t.Errorf("the page does not say %s", want)
		}
	}
}
