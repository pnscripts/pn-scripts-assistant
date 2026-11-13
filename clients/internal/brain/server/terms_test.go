package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/terms"
)

// agreedTo accepts the terms of use for the brain in root, as its owner does
// once on the first start: the tests of everything else start from there.
func agreedTo(t *testing.T, root string) {
	t.Helper()

	if _, err := terms.Accept(root, terms.Version, "en", time.Now()); err != nil {
		t.Fatal(err)
	}
}

// notAgreed is a server whose owner has not accepted the terms yet.
func notAgreed(t *testing.T) (*httptest.Server, *brain.Brain) {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Owner = "Alex"
	cfg.OllamaURL = "http://127.0.0.1:9"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger)
	ts := httptest.NewServer(New(b, logger).Handler())

	t.Cleanup(func() {
		ts.Close()
		db.Close()
	})

	return ts, b
}

func call(t *testing.T, method, url, body string) (int, string) {
	t.Helper()

	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	return res.StatusCode, string(raw)
}

/*
 * Nothing is used before the terms are accepted: the page is the agreement
 * and nothing else, and every part of the API that talks, listens, reads or
 * changes something refuses with a reason a terminal client can print.
 */
func TestNothingIsUsedBeforeTheTermsAreAccepted(t *testing.T) {
	ts, _ := notAgreed(t)

	code, page := call(t, "GET", ts.URL+"/", "")
	if code != 200 || !strings.Contains(page, `id="agreement"`) {
		t.Fatalf("the first page is not the agreement (%d)", code)
	}

	// Whose terms these are, in the markup itself (the Windows CI job reads
	// the page for the product's name before any script runs).
	if !strings.Contains(page, `class="agreement-product">PN Scripts Assistant</p>`) {
		t.Error("the agreement page does not name the product")
	}

	for _, console := range []string{`id="transcript"`, `/js/console.js`, `id="composer"`} {
		if strings.Contains(page, console) {
			t.Errorf("the agreement page carries the console's %s", console)
		}
	}

	for _, r := range []struct{ method, path, body string }{
		{"POST", "/api/chat", `{"message":"hello"}`},
		{"GET", "/api/greeting", ""},
		{"POST", "/api/turn", ""},
		{"POST", "/api/listen", ""},
		{"POST", "/api/speak", `{"text":"hello"}`},
		{"GET", "/api/status", ""},
		{"GET", "/api/knowledge", ""},
		{"GET", "/api/pulse", ""},
		{"POST", "/api/todos", `{"text":"x"}`},
		{"POST", "/api/settings", `{}`},
		{"GET", "/api/terms/../status", ""},
	} {
		code, body := call(t, r.method, ts.URL+r.path, r.body)
		if code != http.StatusForbidden {
			t.Errorf("%s %s before the terms were accepted answered %d: %s", r.method, r.path, code, body)

			continue
		}

		var refused struct{ Error, Terms string }
		if err := json.Unmarshal([]byte(body), &refused); err != nil || refused.Terms != "not accepted" ||
			!strings.Contains(refused.Error, "accept") {
			t.Errorf("%s %s refused without saying why: %s", r.method, r.path, body)
		}
	}

	// What the agreement page itself needs, and the window, still answer.
	for _, path := range []string{"/health", "/api/terms", "/api/ui-language", "/api/v1/version", "/css/console.css",
		"/js/i18n.js", "/js/agreement.js"} {
		if code, body := call(t, "GET", ts.URL+path, ""); code != 200 {
			t.Errorf("GET %s before the terms were accepted answered %d: %.200s", path, code, body)
		}
	}

	// The template is never served raw.
	if code, _ := call(t, "GET", ts.URL+"/agreement.html", ""); code == 200 {
		t.Error("the agreement template is served as a file")
	}
}

// What /api/terms says: the terms in force, in both languages, and whether
// they were accepted.
func TestTheTermsAreServedInBothLanguages(t *testing.T) {
	ts, _ := notAgreed(t)

	code, body := call(t, "GET", ts.URL+"/api/terms", "")
	if code != 200 {
		t.Fatalf("GET /api/terms = %d", code)
	}

	var got struct {
		Version  int
		Accepted bool
		En, Bg   terms.Text
		Online   struct{ En, Bg string }
	}

	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}

	if got.Version != terms.Version || got.Accepted {
		t.Fatalf("version %d, accepted %v", got.Version, got.Accepted)
	}

	if got.En.Title != terms.In("en").Title || got.Bg.Title != terms.In("bg").Title ||
		len(got.Bg.Sections) != len(terms.In("bg").Sections) {
		t.Fatalf("the terms served are not the terms: %+v", got)
	}

	for lang, url := range map[string]string{"en": got.Online.En, "bg": got.Online.Bg} {
		if !regexp.MustCompile(`^https://github\.com/pnscripts/pn-scripts-assistant/blob/main/docs/guide/terms(\.bg)?\.md$`).MatchString(url) ||
			(lang == "bg") != strings.HasSuffix(url, ".bg.md") {
			t.Errorf("the %s terms online are at %q", lang, url)
		}
	}
}

// Accepting opens the assistant: the console, the API, and a record that the
// next start reads.
func TestAcceptingTheTermsOpensTheAssistant(t *testing.T) {
	ts, b := notAgreed(t)

	code, body := call(t, "POST", ts.URL+"/api/terms", `{"version":`+itoa(int64(terms.Version))+`,"language":"bg"}`)
	if code != 200 {
		t.Fatalf("accepting answered %d: %s", code, body)
	}

	if !terms.Accepted(b.Root) {
		t.Fatal("accepted, and nothing was kept")
	}

	if r, _ := terms.Load(b.Root); r.Language != "bg" {
		t.Errorf("accepted in %q, want bg", r.Language)
	}

	if code, page := call(t, "GET", ts.URL+"/", ""); code != 200 || !strings.Contains(page, `id="transcript"`) ||
		strings.Contains(page, `id="agreement"`) {
		t.Fatalf("after accepting, the page is not the console (%d)", code)
	}

	if code, body := call(t, "GET", ts.URL+"/api/status", ""); code != 200 {
		t.Fatalf("after accepting, /api/status answered %d: %s", code, body)
	}

	if code, body := call(t, "GET", ts.URL+"/api/terms", ""); code != 200 || !strings.Contains(body, `"accepted":true`) {
		t.Fatalf("after accepting, /api/terms says %d %s", code, body)
	}
}

// Only the terms in force are accepted, and only by somebody who sent them.
func TestAcceptingOtherTermsIsRefused(t *testing.T) {
	ts, b := notAgreed(t)

	for _, body := range []string{`{"version":` + itoa(int64(terms.Version+1)) + `}`, `{"version":0}`, `{}`, `not json`} {
		code, answer := call(t, "POST", ts.URL+"/api/terms", body)
		if code != http.StatusConflict && code != http.StatusBadRequest {
			t.Errorf("accepting %s answered %d: %s", body, code, answer)
		}
	}

	if terms.Accepted(b.Root) {
		t.Fatal("a refused acceptance was kept")
	}

	if code, _ := call(t, "GET", ts.URL+"/api/status", ""); code != http.StatusForbidden {
		t.Fatalf("after a refused acceptance the API answered %d", code)
	}
}

// The agreement page: drawn from /api/terms, both languages, one button, and
// every string it names in i18n.js.
func TestTheAgreementPageAsksOnceWithOneButton(t *testing.T) {
	page := readAsset(t, "agreement.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	script := readAsset(t, filepath.Join("js", "agreement.js"))

	for _, want := range []string{`id="agreement"`, `id="agreement-accept"`, `id="agreement-language"`,
		`id="agreement-text"`, `/js/i18n.js`, `/js/agreement.js`, `/css/console.css`, `{{ .Name }}`} {
		if !strings.Contains(page, want) {
			t.Errorf("the agreement page has no %s", want)
		}
	}

	if n := strings.Count(page, "<button"); n != 3 {
		t.Errorf("the agreement page has %d buttons, want accept, the language and the documentation", n)
	}

	for _, key := range regexp.MustCompile(`data-i18n(?:-title|-label)?="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		if en, bg := strings2(t, js, key[1]); en == "" || bg == "" {
			t.Errorf("%s is not in both languages", key[1])
		}
	}

	for _, key := range regexp.MustCompile(`pnT\('([^']+)'`).FindAllStringSubmatch(script, -1) {
		if en, bg := strings2(t, js, key[1]); en == "" || bg == "" {
			t.Errorf("%s is not in both languages", key[1])
		}
	}

	if strings.Contains(script, "innerHTML") {
		t.Error("agreement.js writes HTML; the terms are text")
	}

	for _, want := range []string{"/api/terms", "pn-language", "location.reload", "/api/open-source"} {
		if !strings.Contains(script, want) {
			t.Errorf("agreement.js does not use %s", want)
		}
	}
}

/*
 * It behaves as an assistant: the 1.0.3 line "Answers come from AI and can
 * be wrong." beside the conversation and the AI mark beside its name are
 * gone. The terms say it once, and stay readable on "What it runs on".
 */
func TestThePageNoLongerAnnouncesAnAI(t *testing.T) {
	page := readAsset(t, "index.html")
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	css := readAsset(t, filepath.Join("css", "console.css"))

	for _, gone := range []string{"ai-badge", "ai-notice", "cc.aiNotice"} {
		for name, text := range map[string]string{"index.html": page, "i18n.js": js, "console.css": css} {
			if strings.Contains(text, gone) {
				t.Errorf("%s still has %s", name, gone)
			}
		}
	}

	updates := regexp.MustCompile(`(?s)<section class="view" data-view="updates" hidden>(.*?)</section>`).FindStringSubmatch(page)
	if updates == nil || !strings.Contains(updates[1], `id="terms-card"`) || !strings.Contains(updates[1], `id="terms-read"`) {
		t.Fatal(`"What it runs on" has no card with the terms`)
	}

	if !strings.Contains(page, `/js/terms.js`) {
		t.Error("the terms card has no script")
	}
}
