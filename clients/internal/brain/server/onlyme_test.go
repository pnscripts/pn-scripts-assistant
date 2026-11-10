package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/voiceprint"
)

/*
 * "Answer only me" protects, or says why it cannot.
 *
 * After an upgrade the runtime from the previous release counts as missing
 * (voiceprint.Installed), so no voice was judged at all, and the filter only
 * ever refused a voice it had judged to be somebody else's: everybody's
 * words went through while the switch still showed on. Now, with the switch
 * on, a voice that cannot be recognised is acted on no more than a stranger's,
 * and the turn says why, on the page and in the record of what was heard.
 */
func TestAnswerOnlyMeActsOnNothingItCannotRecognise(t *testing.T) {
	offerAModel(t)

	cases := []struct {
		name  string
		setup func(t *testing.T, parts, root string)
		want  string
	}{
		{
			name:  "nothing installed",
			setup: func(t *testing.T, parts, root string) {},
			want:  "not installed",
		},
		{
			name: "the runtime of an older release, as after an upgrade",
			setup: func(t *testing.T, parts, root string) {
				os.WriteFile(filepath.Join(parts, voiceprint.ModelName), []byte("model"), 0o644)
				os.WriteFile(filepath.Join(parts, voiceprint.RuntimeName), []byte("the library from 1.29.0"), 0o755)
				os.WriteFile(filepath.Join(parts, voiceprint.RuntimeStamp), []byte("1.29.0\n"), 0o644)
			},
			want: "older version",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			parts := voiceprint.Parts()
			if err := os.MkdirAll(parts, 0o755); err != nil {
				t.Fatal(err)
			}

			root := t.TempDir()

			tc.setup(t, parts, root)

			db, err := store.Open(filepath.Join(root, "brain.sqlite"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { db.Close() })

			cfg := config.Default()
			cfg.Owner = "Alex"
			cfg.WakeWord = "Nova"
			cfg.OnlyMe = true

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			s := New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger)

			ts := httptest.NewServer(s.Handler())
			t.Cleanup(ts.Close)

			old := listenForTurn
			t.Cleanup(func() { listenForTurn = old })

			listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
				return speech.Heard{
					Text:        "Nova, delete my documents folder",
					HeardSpeech: true,
					PeakRMS:     3000,
					Samples:     make([]float64, 16000),
				}, nil
			}

			status, out := postBody(t, ts.URL+"/api/turn", `{"engaged":true}`)
			if status != http.StatusOK {
				t.Fatalf("the turn failed: %d %v", status, out)
			}

			if out["addressed"] != false {
				t.Errorf("a voice that could not be recognised was acted on with answer-only-me on: %v", out)
			}

			refused, _ := out["refused"].(string)
			if !strings.Contains(refused, "answer only me") || !strings.Contains(refused, tc.want) {
				t.Errorf("the page is not told why: refused = %q, want it to name %q", refused, tc.want)
			}

			heard := s.heard.recent()
			if len(heard) == 0 || heard[0].Addressed || !strings.Contains(heard[0].Why, tc.want) {
				t.Errorf("the record of what was heard does not say why: %+v", heard)
			}
		})
	}

	// And with the switch off, nothing changes: the name is enough.
	t.Run("switch off", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		root := t.TempDir()

		db, err := store.Open(filepath.Join(root, "brain.sqlite"))
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { db.Close() })

		cfg := config.Default()
		cfg.Owner = "Alex"
		cfg.WakeWord = "Nova"

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		ts := httptest.NewServer(New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger).Handler())
		t.Cleanup(ts.Close)

		old := listenForTurn
		t.Cleanup(func() { listenForTurn = old })

		listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
			return speech.Heard{Text: "Nova, what time is it", HeardSpeech: true, PeakRMS: 3000,
				Samples: make([]float64, 16000)}, nil
		}

		if _, out := postBody(t, ts.URL+"/api/turn", `{"engaged":true}`); out["addressed"] != true || out["refused"] != nil {
			t.Errorf("with answer-only-me off the turn was not acted on: %v", out)
		}
	})
}

// The Privacy page is told, too, while the switch shows on.
func TestThePrivacyPageSaysAnswerOnlyMeCannotWorkNow(t *testing.T) {
	offerAModel(t)

	t.Setenv("HOME", t.TempDir())

	ts, _, b := newServer(t)

	b.Cfg.OnlyMe = true

	res, err := http.Get(ts.URL + "/api/voiceprint")
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	if !strings.Contains(string(raw), `"only_me_blocked":"`) {
		t.Errorf("the voiceprint state does not say answer-only-me cannot work now: %s", raw)
	}
}

/*
 * "Answer only me" explains itself in the page's language.
 *
 * The refusal on a turn and the Privacy page's row were the server's English
 * sentence, shown as it was, on a page somebody had set to Bulgarian. This
 * takes the server's own answers (a turn that could not be recognised, a
 * voice judged to be somebody else's, the voiceprint state while the switch
 * is on), runs the page's own scripts on them in node with the browser in
 * each language, and reads what a person would see.
 */
func TestAnswerOnlyMeIsSaidInBothLanguages(t *testing.T) {
	offerAModel(t)

	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	t.Setenv("HOME", t.TempDir())

	ts, _, b := newServer(t)
	b.Cfg.OnlyMe = true
	b.Cfg.WakeWord = "Nova"

	old := listenForTurn
	t.Cleanup(func() { listenForTurn = old })

	listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
		return speech.Heard{Text: "Nova, delete my documents folder", HeardSpeech: true, PeakRMS: 3000,
			Samples: make([]float64, 16000)}, nil
	}

	// A turn that could not be recognised: nothing is installed here.
	_, unrecognised := postBody(t, ts.URL+"/api/turn", `{"engaged":true}`)

	if unrecognised["refused_key"] != "onlyme.unrecognised" {
		t.Fatalf("the turn's refusal carries no key for the page: %v", unrecognised)
	}

	// A voice judged and found to be somebody else's, as the turn would carry it.
	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))
	notYou := s.onlyMeRefuses(voiceprint.Verdict{Judged: true, Alike: 0.31}).fields()

	// And the Privacy page's state while the switch is on.
	res, err := http.Get(ts.URL + "/api/voiceprint")
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()

	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}

	state := raw
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Data) > 0 {
		state = wrapped.Data
	}

	turn, _ := json.Marshal(unrecognised)
	judged, _ := json.Marshal(notYou)

	en := onlyMeSays(t, "en-GB", turn, judged, state)
	bg := onlyMeSays(t, "bg-BG", turn, judged, state)

	if !strings.Contains(en.Turn, "Not acted on") || !strings.Contains(en.Turn, "not installed") {
		t.Errorf("in English the turn says %q", en.Turn)
	}

	if !strings.Contains(en.NotYou, "0.31 alike, 0.50 needed") {
		t.Errorf("in English somebody else's voice is %q", en.NotYou)
	}

	if !strings.Contains(en.Blocked[1], "cannot be recognised now (the voiceprint model is not installed)") {
		t.Errorf("in English the Privacy row is %q", en.Blocked)
	}

	for what, said := range map[string]string{
		"the turn": bg.Turn, "somebody else's voice": bg.NotYou,
		"the Privacy row's label": bg.Blocked[0], "the Privacy row": bg.Blocked[1],
	} {
		if !regexp.MustCompile(`[а-яА-Я]`).MatchString(said) || regexp.MustCompile(`[a-zA-Z]{3,}`).MatchString(said) {
			t.Errorf("in Bulgarian %s is %q", what, said)
		}
	}

	if !strings.Contains(bg.Turn, "не е инсталиран") || !strings.Contains(bg.NotYou, "0,31") {
		t.Errorf("in Bulgarian: %q, %q", bg.Turn, bg.NotYou)
	}

	// Every reason the server can give is one the page has words for, in
	// both languages.
	js, err := assets.ReadFile("assets/js/i18n.js")
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(en.Reasons)

	want := voiceprint.Reasons()
	sort.Strings(want)

	if strings.Join(en.Reasons, ",") != strings.Join(want, ",") {
		t.Errorf("onlyme.js knows the reasons %v, the server gives %v", en.Reasons, want)
	}

	for _, reason := range want {
		if !strings.Contains(string(js), "'onlyme.why."+reason+"': [") {
			t.Errorf("onlyme.why.%s is not in i18n.js", reason)
		}
	}

	// And the page loads the script before the two that use it.
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	at := strings.Index(string(page), `<script src="/js/onlyme.js">`)
	if at < 0 || at > strings.Index(string(page), `<script src="/js/console.js">`) ||
		at > strings.Index(string(page), `<script src="/js/voiceprint.js">`) {
		t.Error("onlyme.js is not loaded before console.js and voiceprint.js")
	}
}

type onlyMeSaid struct {
	Turn    string   `json:"turn"`
	NotYou  string   `json:"notYou"`
	Blocked []string `json:"blocked"`
	Reasons []string `json:"reasons"`
}

// onlyMeSays runs i18n.js and onlyme.js in node, with the browser's language
// set to language, on the server's answers.
func onlyMeSays(t *testing.T, language string, turn, judged, state []byte) onlyMeSaid {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "onlyme.js"} {
		script, err := assets.ReadFile("assets/js/" + name)
		if err != nil {
			t.Fatal(err)
		}

		os.WriteFile(filepath.Join(dir, name), script, 0o644)
	}

	os.WriteFile(filepath.Join(dir, "turn.json"), turn, 0o644)
	os.WriteFile(filepath.Join(dir, "judged.json"), judged, 0o644)
	os.WriteFile(filepath.Join(dir, "state.json"), state, 0o644)

	// Just enough of a browser for the two scripts: no page, no server.
	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
Object.defineProperty(globalThis, 'navigator', { value: { language: process.argv[3] }, configurable: true });
globalThis.window = globalThis;
window.dispatchEvent = () => true;
globalThis.CustomEvent = function () {};
globalThis.document = { readyState: 'complete', querySelectorAll: () => [], documentElement: {} };
globalThis.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
eval(read('i18n.js'));
eval(read('onlyme.js'));
const turn = JSON.parse(read('turn.json'));
const judged = JSON.parse(read('judged.json'));
const state = JSON.parse(read('state.json'));
process.stdout.write(JSON.stringify({
    turn: window.pnOnlyMe.notActedOn(turn),
    notYou: window.pnOnlyMe.refused(judged),
    blocked: window.pnOnlyMe.blocked(state),
    reasons: window.pnOnlyMe.reasons,
}));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said onlyMeSaid
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}

/*
 * offerAModel stands in a speaker model for a test of "Answer only me" itself.
 *
 * 1.0.1 offers none (voiceprint.Offered), so the switch cannot be turned on;
 * these tests keep proving how it behaves for the release that offers a clean
 * model again.
 */
func offerAModel(t *testing.T) {
	t.Helper()

	t.Cleanup(voiceprint.Offer("a model offered for this test", strings.Repeat("0", 64)))
}
