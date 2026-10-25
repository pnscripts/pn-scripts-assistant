package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

func TestTheGlossaryIsKeptFromThePage(t *testing.T) {
	ts, _, _ := newServer(t)

	if code := sendJSON(t, ts.URL+"/api/glossary", map[string]any{"source": "invoice", "target": "фактура", "from": "en", "to": "bg"}, nil); code != http.StatusOK {
		t.Fatalf("adding answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/glossary", map[string]any{"source": "x", "target": "", "to": "bg"}, nil); code != http.StatusBadRequest {
		t.Errorf("an empty translation answered %d", code)
	}

	var imported struct {
		Added    int      `json:"added"`
		Problems []string `json:"problems"`
	}

	sendJSON(t, ts.URL+"/api/glossary/import", map[string]any{"csv": "\ufeffsource;target\nдоговор;contract\n;nothing", "from": "bg", "to": "en"}, &imported)

	if imported.Added != 1 || len(imported.Problems) != 1 {
		t.Errorf("%+v", imported)
	}

	var list struct {
		Terms []struct {
			Source string `json:"source"`
			To     string `json:"to"`
		} `json:"terms"`
		Languages []struct {
			Code   string `json:"code"`
			NameBG string `json:"name_bg"`
		} `json:"languages"`
	}

	getJSON(t, ts.URL+"/api/glossary", &list)

	if len(list.Terms) != 2 || len(list.Languages) < 10 || list.Languages[0].NameBG == "" {
		t.Fatalf("%+v", list)
	}

	resp, err := http.Get(ts.URL + "/api/glossary.csv")
	if err != nil {
		t.Fatal(err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.HasPrefix(string(body), "\ufeffsource,target,from,to,note") || !strings.Contains(string(body), "invoice,фактура,en,bg") {
		t.Errorf("%q", body)
	}

	if code := sendJSON(t, ts.URL+"/api/glossary/remove", map[string]any{"source": "invoice", "from": "en", "to": "bg"}, nil); code != http.StatusOK {
		t.Errorf("removing answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/glossary/remove", map[string]any{"source": "invoice", "from": "en", "to": "bg"}, nil); code != http.StatusNotFound {
		t.Errorf("removing twice answered %d", code)
	}
}

func TestTextAndFilesAreTranslatedFromThePage(t *testing.T) {
	ts, _, b := newServer(t)

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: "Платете сметката."})
	b.Cfg.DefaultProvider = "ollama"

	sendJSON(t, ts.URL+"/api/glossary", map[string]any{"source": "invoice", "target": "фактура", "to": "bg"}, nil)

	var res struct {
		Text   string `json:"text"`
		Missed []struct {
			Kind, Source string
		} `json:"missed"`
	}

	if code := sendJSON(t, ts.URL+"/api/translate", map[string]any{"text": "Pay the invoice.", "to": "bg"}, &res); code != http.StatusOK {
		t.Fatalf("translating answered %d", code)
	}

	if res.Text != "Платете сметката." || len(res.Missed) != 1 || res.Missed[0].Source != "invoice" {
		t.Errorf("%+v", res)
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(in, []byte("Pay the invoice."), 0o600)

	if code := sendJSON(t, ts.URL+"/api/translate/file", map[string]any{"path": in, "to": "bg"}, &res); code != http.StatusOK {
		t.Fatalf("translating a file answered %d", code)
	}

	if got, _ := os.ReadFile(filepath.Join(dir, "a.bg.txt")); string(got) != "Платете сметката." || res.Text != filepath.Join(dir, "a.bg.txt") {
		t.Errorf("%q %+v", got, res)
	}

	if code := sendJSON(t, ts.URL+"/api/translate/file", map[string]any{"path": in, "to": "bg"}, nil); code != http.StatusBadRequest {
		t.Errorf("translating over an existing translation answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/translate/file", map[string]any{"path": "a.txt", "to": "bg"}, nil); code != http.StatusBadRequest {
		t.Errorf("a relative path answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/translate", map[string]any{"text": " ", "to": "bg"}, nil); code != http.StatusBadRequest {
		t.Errorf("no text answered %d", code)
	}
}
