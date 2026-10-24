package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/store"
)

func sendJSON(t *testing.T, url string, body any, into any) int {
	t.Helper()

	raw, _ := json.Marshal(body)

	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if into != nil {
		_ = json.NewDecoder(resp.Body).Decode(into)
	}

	return resp.StatusCode
}

// What it did and why, over HTTP, with the filters the page offers.
func TestDeedsAreListedWithTheirFilters(t *testing.T) {
	ts, db, _ := newServer(t)

	db.AddDeed(store.Deed{Tool: "read_file", Summary: "Read notes", Outcome: "done"})
	db.AddDeed(store.Deed{Tool: "fetch_url", Summary: "Fetch a page", Outcome: "done", Leaves: "example.org"})
	db.AddDeed(store.Deed{Tool: "write_file", Summary: "Write", Outcome: "waiting", Changes: true})

	var all struct {
		Deeds  []store.Deed   `json:"deeds"`
		Counts map[string]int `json:"counts"`
	}

	getJSON(t, ts.URL+"/api/deeds?since=day", &all)

	if len(all.Deeds) != 3 || all.Counts["leaves"] != 1 || all.Counts["changes"] != 1 || all.Counts["waiting"] != 1 {
		t.Fatalf("listed %+v", all)
	}

	var leaving struct {
		Deeds []store.Deed `json:"deeds"`
	}

	getJSON(t, ts.URL+"/api/deeds?since=all&leaves=1", &leaving)

	if len(leaving.Deeds) != 1 || leaving.Deeds[0].Tool != "fetch_url" {
		t.Errorf("only what left: %+v", leaving.Deeds)
	}

	resp, err := http.Get(ts.URL + "/api/deeds?since=century")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a made-up period answered %d", resp.StatusCode)
	}
}

// The Control page: every capability says whether it can send anything off
// this machine and whether it is switched off, and the switch works.
func TestControlListsWhatLeavesAndSwitchesOff(t *testing.T) {
	ts, _, b := newServer(t)

	type capability struct {
		Name   string `json:"name"`
		Off    bool   `json:"off"`
		Leaves bool   `json:"leaves"`
		What   string `json:"leaves_what"`
	}

	var page struct {
		Capabilities []capability `json:"capabilities"`
		Cite         bool         `json:"cite_sources"`
	}

	getJSON(t, ts.URL+"/api/permissions", &page)

	if !page.Cite {
		t.Error("citing is not on by default")
	}

	find := func(name string) *capability {
		for i := range page.Capabilities {
			if page.Capabilities[i].Name == name {
				return &page.Capabilities[i]
			}
		}

		return nil
	}

	if c := find("web_search"); c == nil || !c.Leaves || c.What == "" {
		t.Errorf("web search: %+v", c)
	}

	if c := find("read_file"); c == nil || c.Leaves {
		t.Errorf("read file: %+v", c)
	}

	if code := sendJSON(t, ts.URL+"/api/environment/off", map[string]any{"id": "tool:read_file", "off": true}, nil); code != http.StatusOK {
		t.Fatalf("switching off answered %d", code)
	}

	getJSON(t, ts.URL+"/api/permissions", &page)

	if c := find("read_file"); c == nil || !c.Off {
		t.Errorf("read file after switching off: %+v", c)
	}

	if !b.Agent.OffLimits("read_file") {
		t.Error("switched off on the page and still offered to the model")
	}

	// Citing is switched from here, and saved.
	if code := sendJSON(t, ts.URL+"/api/settings/cite-sources", map[string]any{"on": false}, nil); code != http.StatusOK {
		t.Fatalf("switching citing off answered %d", code)
	}

	if b.Cfg.CiteSources {
		t.Error("citing is still on")
	}
}

// A source opens as an address or as the folder its file is in — never as a
// program, and never anything that is neither.
func TestOpeningASourceIsHeldToAddressesAndFolders(t *testing.T) {
	ts, _, _ := newServer(t)

	var opened []string

	was := openOutside
	openOutside = func(target string) error { opened = append(opened, target); return nil }

	t.Cleanup(func() { openOutside = was })

	file := filepath.Join(t.TempDir(), "plan.md")
	os.WriteFile(file, []byte("x"), 0o644)

	for _, where := range []string{"https://example.org/a?b=1&c=2", file} {
		if code := sendJSON(t, ts.URL+"/api/open-source", map[string]string{"where": where}, nil); code != http.StatusOK {
			t.Errorf("%s answered %d", where, code)
		}
	}

	if len(opened) != 2 || opened[0] != "https://example.org/a?b=1&c=2" || opened[1] != filepath.Dir(file) {
		t.Errorf("opened %v", opened)
	}

	for _, where := range []string{"javascript:alert(1)", "file:///etc/passwd", "relative/path", "/no/such/file"} {
		if code := sendJSON(t, ts.URL+"/api/open-source", map[string]string{"where": where}, nil); code != http.StatusBadRequest {
			t.Errorf("%q answered %d", where, code)
		}
	}

	if len(opened) != 2 {
		t.Errorf("something refused was opened: %v", opened)
	}
}

func TestThePageLanguageFollowsTheSpokenOne(t *testing.T) {
	for spoken, want := range map[string]string{"bg": "bg", "bg-BG": "bg", "en": "en", "de": "en", "": ""} {
		if got := uiLanguage(spoken); got != want {
			t.Errorf("%q: %q, expected %q", spoken, got, want)
		}
	}
}
