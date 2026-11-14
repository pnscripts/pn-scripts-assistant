package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

func postPermit(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()

	raw, _ := json.Marshal(body)

	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)

	return resp.StatusCode, out
}

// A card's buttons: decided, served with the inventory, changed back, and
// an "always" given to an older version shown asleep after an update.
func TestAProgramsCardDecisionsAreKeptAndServed(t *testing.T) {
	ts, db, _ := newServer(t)

	now := time.Now().UTC()
	save, _ := programs.Reconcile(programs.Snap, nil, []programs.Program{{Kind: programs.Snap, ID: "blender", Name: "Blender", Version: "4.5.3"}}, now)
	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	code, _ := postPermit(t, ts.URL+"/api/programs/permit", map[string]string{"kind": "snap", "id": "blender", "verb": "start", "answer": "allow"})
	if code != http.StatusOK {
		t.Fatalf("always: %d", code)
	}

	code, out := postPermit(t, ts.URL+"/api/programs/permit", map[string]string{"kind": "snap", "id": "blender", "verb": "control-input", "answer": "never"})
	if code != http.StatusOK || len(asList(out["permits"])) != 2 {
		t.Fatalf("never: %d %v", code, out)
	}

	var got struct {
		Permits map[string][]struct {
			Verb    string `json:"verb"`
			Answer  string `json:"answer"`
			Version string `json:"version"`
			Asleep  bool   `json:"asleep"`
		} `json:"permits"`
		Verbs    []string `json:"verbs"`
		Honoured *bool    `json:"honoured"`
	}

	getJSON(t, ts.URL+"/api/programs", &got)

	p := got.Permits["snap:blender"]
	if len(p) != 2 || p[0].Verb != "start" || p[0].Answer != "allow" || p[0].Version != "4.5.3" || p[0].Asleep ||
		p[1].Verb != "control-input" || p[1].Answer != "never" {
		t.Fatalf("served %+v", got)
	}

	if len(got.Verbs) != 3 || got.Honoured == nil {
		t.Errorf("verbs %v honoured %v", got.Verbs, got.Honoured)
	}

	// Updated: the always sleeps, and says so.
	known, _ := db.Programs(programs.Snap)
	save, _ = programs.Reconcile(programs.Snap, known, []programs.Program{{Kind: programs.Snap, ID: "blender", Name: "Blender", Version: "4.5.4"}}, now)
	db.SavePrograms(save)

	getJSON(t, ts.URL+"/api/programs", &got)
	if p := got.Permits["snap:blender"]; len(p) != 2 || !p[0].Asleep || p[1].Asleep {
		t.Errorf("after an update %+v", p)
	}

	// Back to asking: no decision left for that verb.
	postPermit(t, ts.URL+"/api/programs/permit", map[string]string{"kind": "snap", "id": "blender", "verb": "start", "answer": "ask"})
	getJSON(t, ts.URL+"/api/programs", &got)
	if p := got.Permits["snap:blender"]; len(p) != 1 || p[0].Verb != "control-input" {
		t.Errorf("ask each time left %+v", p)
	}
}

func TestAProgramsCardRefusesWhatItCannotMean(t *testing.T) {
	ts, db, _ := newServer(t)

	save, _ := programs.Reconcile(programs.Deb, nil, []programs.Program{{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"}}, time.Now())
	db.SavePrograms(save)

	for _, c := range []struct {
		body map[string]string
		code int
	}{
		{map[string]string{"kind": "deb", "id": "nothing", "verb": "start", "answer": "never"}, http.StatusNotFound},
		{map[string]string{"kind": "rpm", "id": "gimp", "verb": "start", "answer": "never"}, http.StatusBadRequest},
		{map[string]string{"kind": "deb", "id": " ", "verb": "start", "answer": "never"}, http.StatusBadRequest},
		{map[string]string{"kind": "deb", "id": "gimp", "verb": "format", "answer": "never"}, http.StatusBadRequest},
		{map[string]string{"kind": "deb", "id": "gimp", "verb": "start", "answer": "maybe"}, http.StatusBadRequest},
	} {
		if code, _ := postPermit(t, ts.URL+"/api/programs/permit", c.body); code != c.code {
			t.Errorf("%v: %d, want %d", c.body, code, c.code)
		}
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
