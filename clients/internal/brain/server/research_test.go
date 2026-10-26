package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The Research page. Nothing here reaches the web: research is refused with
 * the web off and with no model before anything is searched, and the check
 * reads a file.
 */
func TestTheResearchPage(t *testing.T) {
	ts, db, b := newServer(t)

	b.Router = nil

	b.Mode = llm.ModePrivate
	if code := sendJSON(t, ts.URL+"/api/research", map[string]any{"question": "What is the VAT rate?"}, nil); code != http.StatusForbidden {
		t.Errorf("research with the web off answered %d", code)
	}

	b.Mode = llm.ModeResearch
	if code := sendJSON(t, ts.URL+"/api/research", map[string]any{"question": "What is the VAT rate?"}, nil); code != http.StatusServiceUnavailable {
		t.Errorf("research with no model answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/research", map[string]any{"question": "VAT?", "depth": "deep"}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown depth answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/research", map[string]any{"question": "?"}, nil); code != http.StatusBadRequest {
		t.Errorf("a question too short answered %d", code)
	}

	kept, err := db.SaveResearch(store.ResearchReport{
		Question: "VAT in Bulgaria", Lang: "en", Depth: "quick", Report: "# VAT in Bulgaria\n\nThe rate is 20 % [1].",
		Sources:  json.RawMessage(`[{"n":1,"title":"NRA","url":"https://nra.bg/vat","text":"what the page said"}]`),
		Claims:   json.RawMessage(`[{"text":"The rate is 20 % [1].","status":"supported","cites":[1]}]`),
		NSources: 1, NClaims: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	var list struct {
		Reports []map[string]any `json:"reports"`
		Web     bool             `json:"web"`
	}

	getJSON(t, ts.URL+"/api/research", &list)

	if len(list.Reports) != 1 || !list.Web || list.Reports[0]["report"] != nil {
		t.Fatalf("%+v", list)
	}

	id := strconv.FormatInt(kept.ID, 10)

	resp, err := http.Get(ts.URL + "/api/research/" + id)
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(raw), "nra.bg") || strings.Contains(string(raw), "what the page said") {
		t.Errorf("a source went to the page with the text read from it, or not at all: %s", raw)
	}

	resp, err = http.Get(ts.URL + "/api/research/" + id + "/download")
	if err != nil {
		t.Fatal(err)
	}

	md, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/markdown") ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), "research-"+id+".md") || !strings.HasPrefix(string(md), "# VAT") {
		t.Errorf("%v %s", resp.Header, md)
	}

	// Checking a text against a file of its own.
	notes := filepath.Join(t.TempDir(), "notes.txt")
	_ = os.WriteFile(notes, []byte("The bridge was finished in 1907 and is 300 metres long."), 0o600)

	var checked struct {
		Summary struct {
			Claims, Supported, Flagged int
		} `json:"summary"`
	}

	if code := sendJSON(t, ts.URL+"/api/research/check", map[string]any{
		"text":    "The bridge was finished in 1907 [1]. It is 450 metres long and very famous [1].",
		"sources": []string{notes},
	}, &checked); code != http.StatusOK || checked.Summary.Claims != 2 || checked.Summary.Flagged != 1 {
		t.Errorf("%d %+v", code, checked)
	}

	if code := sendJSON(t, ts.URL+"/api/research/check", map[string]any{"text": "x"}, nil); code != http.StatusBadRequest {
		t.Errorf("a check with no sources answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/research/"+id+"/remove", map[string]any{}, nil); code != http.StatusOK {
		t.Errorf("remove answered %d", code)
	}

	if resp, _ := http.Get(ts.URL + "/api/research/" + id); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a removed report answered %d", resp.StatusCode)
	}
}
