package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/vault"
	"pn-scripts-assistant/internal/brain/voiceprint"
)

/*
 * 1.0.1 withdrew "Answer only me".
 *
 * The model it measured voices with (CAM++, trained on VoxCeleb) was made from
 * recordings offered for research only, and the owner withdrew it on
 * 2026-11-10 until a model whose whole lineage may be used commercially
 * exists. These are what a person who had it on in 1.0.0 meets now: the
 * switch cannot be turned on, nothing is downloaded, a voice taught before
 * can still be forgotten, every voice that says the name is answered as
 * before the feature existed, and the Privacy page says why in English and
 * in Bulgarian.
 */

// enrolled leaves a voiceprint taught by an older release in root.
func enrolled(t *testing.T, root string) {
	t.Helper()

	print := make([]float32, voiceprint.Dimensions)
	print[0] = 1

	raw, err := json.Marshal(voiceprint.Known{Print: print, Samples: 3, Agreement: 0.8})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(voiceprint.Folder(root), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := vault.WriteFile(filepath.Join(voiceprint.Folder(root), voiceprint.KnownFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if !voiceprint.Enrolled(root) {
		t.Fatal("the test voiceprint does not read back as enrolled")
	}
}

func withdrawnServer(t *testing.T) (string, *brain.Brain) {
	t.Helper()

	if voiceprint.Offered() {
		t.Skip("a speaker model is offered in this build")
	}

	t.Setenv("HOME", t.TempDir())

	ts, _, b := newServer(t)

	enrolled(t, b.Root)

	return ts.URL, b
}

func TestThePrivacyPageIsToldAnswerOnlyMeIsWithdrawn(t *testing.T) {
	url, _ := withdrawnServer(t)

	var state map[string]any

	getJSON(t, url+"/api/voiceprint", &state)

	if state["withdrawn"] != true {
		t.Errorf("the voiceprint state does not say the feature is withdrawn: %v", state)
	}

	if state["enrolled"] != true {
		t.Errorf("a voice taught before is not reported, so it could not be forgotten: %v", state)
	}

	if state["installed"] != false || state["only_me"] != false {
		t.Errorf("the withdrawn feature reads as available or on: %v", state)
	}
}

func TestAnswerOnlyMeCannotBeSwitchedOnWhenWithdrawn(t *testing.T) {
	url, b := withdrawnServer(t)

	status, out := postBody(t, url+"/api/settings", `{"only_me":true}`)

	if status != http.StatusBadRequest {
		t.Errorf("switching it on answered %d, want 400: %v", status, out)
	}

	if why, _ := out["error"].(string); !strings.Contains(why, "not offered") {
		t.Errorf("the refusal does not say why: %v", out)
	}

	if b.Cfg.OnlyMe {
		t.Error("answer only me was switched on")
	}
}

func TestNothingIsDownloadedOrTaughtWhenWithdrawn(t *testing.T) {
	url, _ := withdrawnServer(t)

	called := false

	old := installVoiceprint
	t.Cleanup(func() { installVoiceprint = old })

	installVoiceprint = func(ctx context.Context, say func(string)) error {
		called = true

		return nil
	}

	for _, where := range []string{"/api/voiceprint/install", "/api/voiceprint/teach"} {
		status, out := postBody(t, url+where, `{}`)

		if status != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400: %v", where, status, out)
		}

		if why, _ := out["error"].(string); !strings.Contains(why, "not offered") {
			t.Errorf("%s does not say why: %v", where, out)
		}
	}

	if called {
		t.Error("the install button started a download of a withdrawn model")
	}
}

func TestAVoiceTaughtBeforeCanStillBeForgotten(t *testing.T) {
	url, b := withdrawnServer(t)

	if status, out := postBody(t, url+"/api/voiceprint/forget", `{}`); status != http.StatusOK {
		t.Fatalf("forgetting answered %d: %v", status, out)
	}

	if voiceprint.Enrolled(b.Root) {
		t.Error("the voiceprint is still there after forgetting it")
	}
}

/*
 * Every voice that says the name is answered, even if the switch somehow
 * reads on (a settings file edited by hand after the brain opened): with
 * nothing to measure a voice with, holding every turn would answer nobody.
 */
func TestEveryVoiceIsAnsweredWhenWithdrawn(t *testing.T) {
	url, b := withdrawnServer(t)

	b.Cfg.OnlyMe = true
	b.Cfg.WakeWord = "Nova"

	old := listenForTurn
	t.Cleanup(func() { listenForTurn = old })

	listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
		return speech.Heard{Text: "Nova, what time is it", HeardSpeech: true, PeakRMS: 3000,
			Samples: make([]float64, 16000)}, nil
	}

	status, out := postBody(t, url+"/api/turn", `{"engaged":true}`)
	if status != http.StatusOK {
		t.Fatalf("the turn failed: %d %v", status, out)
	}

	if out["addressed"] != true || out["refused"] != nil {
		t.Errorf("a turn was held back by a withdrawn feature: %v", out)
	}
}

// The card says why, in both languages, without a dash.
func TestTheWithdrawnCardIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	script := readAsset(t, filepath.Join("js", "voiceprint.js"))
	page := readAsset(t, "index.html")

	for _, key := range []string{"voice.withdrawn", "onlyme.why.withdrawn"} {
		en, bg := strings2(t, js, key)

		if en == "" || bg == "" {
			t.Errorf("%s is not in i18n.js in both languages", key)

			continue
		}

		if !regexp.MustCompile(`[а-яА-Я]`).MatchString(bg) {
			t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", key, bg)
		}

		for _, text := range []string{en, bg} {
			if strings.ContainsAny(text, "–—") {
				t.Errorf("%s uses a dash: %q", key, text)
			}
		}
	}

	if !strings.Contains(script, "state.withdrawn") || !strings.Contains(script, "'voice-withdrawn'") ||
		!strings.Contains(page, `id="voice-withdrawn" data-i18n="voice.withdrawn"`) {
		t.Error("the card does not show the withdrawn notice")
	}

	// The card's title and its switches are said in Bulgarian too.
	for _, key := range []string{"voice.title", "voice.room", "voice.quiet", "voice.forget"} {
		if !strings.Contains(page, `data-i18n="`+key+`"`) && !strings.Contains(script, "'"+key+"'") {
			t.Errorf("%s is not used on the card", key)
		}

		if en, bg := strings2(t, js, key); en == "" || bg == "" {
			t.Errorf("%s is not in i18n.js in both languages", key)
		}
	}
}

// Nothing credits or names the withdrawn model as if it were used.
func TestTheWithdrawnModelIsNotCreditedAsUsed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "THIRD-PARTY-NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}

	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	for _, gone := range []string{"CAM++", "VoxCeleb", "3D-Speaker", "speaker-embedding-models"} {
		if strings.Contains(page, gone) || strings.Contains(js, gone) {
			t.Errorf("the page still credits %s", gone)
		}

		if strings.Contains(string(raw), gone) {
			t.Errorf("THIRD-PARTY-NOTICES.md still lists %s as a part", gone)
		}
	}

}
