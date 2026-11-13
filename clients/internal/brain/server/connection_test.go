package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The status pill follows the model, not only the program.
 *
 * It said "online" and named the configured model whenever the page could
 * reach this program; with Ollama stopped it was green and every question
 * failed (the Windows screenshot of 2026-11-09 shows it). This runs the
 * page's own script on the server's own /api/status with an Ollama that does
 * not answer, and on a status where the model does.
 */
func TestTheStatusPillFollowsTheModel(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Alex"
	// A port nothing listens on: an Ollama that does not answer.
	cfg.OllamaURL = "http://127.0.0.1:9"

	agreedTo(t, root)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts := httptest.NewServer(New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger).Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()

	var body struct {
		Data json.RawMessage `json:"data"`
	}

	status := raw
	if json.Unmarshal(raw, &body) == nil && len(body.Data) > 0 {
		status = body.Data
	}

	silent := pill(t, status)

	if silent.Tone != "warn" || !silent.Why || !strings.Contains(silent.Text, "the model is not answering") {
		t.Errorf("with Ollama not answering the pill says %+v; want a warning, the words and the way to why", silent)
	}

	if !strings.Contains(silent.Title, "ollama") {
		t.Errorf("the explanation does not name what is not answering: %q", silent.Title)
	}

	answering := pill(t, []byte(`{"provider":"ollama","capabilities":["a","b","c"],
		"providers":[{"name":"ollama","reachable":true,"permitted":true,"default":true}]}`))

	if answering.Tone != "online" || answering.Why || answering.Text != "online · 3 capabilities" {
		t.Errorf("with the model answering the pill says %+v", answering)
	}

	forbidden := pill(t, []byte(`{"provider":"anthropic","capabilities":[],
		"providers":[{"name":"anthropic","reachable":false,"permitted":false,"default":true}]}`))

	if forbidden.Tone != "warn" || !forbidden.Why || !strings.Contains(forbidden.Text, "not allowed") {
		t.Errorf("with the model forbidden by privacy the pill says %+v", forbidden)
	}

	// The way to why exists, goes to the Models view and starts hidden.
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(page), `id="conn-why" data-view="models"`) {
		t.Error("the status pill has no button to the Models view")
	}

	if !strings.Contains(string(page), `<script src="/js/connection.js"></script>
<script src="/js/console.js"></script>`) {
		t.Error("connection.js is not loaded before console.js")
	}
}

type pillState struct {
	Tone  string `json:"tone"`
	Text  string `json:"text"`
	Title string `json:"title"`
	Why   bool   `json:"why"`
}

// pill runs connection.js on a status answer and returns what the pill shows.
func pill(t *testing.T, status []byte) pillState {
	t.Helper()

	script, err := assets.ReadFile("assets/js/connection.js")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "connection.js"), script, 0o644)
	os.WriteFile(filepath.Join(dir, "status.json"), status, 0o644)
	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
global.window = {};
eval(fs.readFileSync(path.join(dir, 'connection.js'), 'utf8'));
const status = JSON.parse(fs.readFileSync(path.join(dir, 'status.json'), 'utf8'));
process.stdout.write(JSON.stringify(window.pnConnection(status)));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("connection.js failed: %v\n%s", err, out)
	}

	var got pillState
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("connection.js said %s: %v", out, err)
	}

	return got
}
