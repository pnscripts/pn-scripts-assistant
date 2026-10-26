package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// The Skills page: every skill with its source, a switch that takes it out
// of what the model is offered, and folders read on Reload.
func TestTheSkillsPage(t *testing.T) {
	ts, _, b := newServer(t)

	var cat struct {
		Skills []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
			On     bool   `json:"on"`
		} `json:"skills"`
		Folders []struct {
			Path, Kind string
			Exists     bool
		} `json:"folders"`
	}

	getJSON(t, ts.URL+"/api/skills/catalogue", &cat)

	found := false
	for _, s := range cat.Skills {
		if s.Name == "scam_guard" && s.Source == "playbook" && s.On {
			found = true
		}
	}

	if !found {
		t.Fatalf("the built-in playbook is not listed on: %+v", cat.Skills)
	}

	if _, ok := b.Agent.Registry.Get("scam_guard"); !ok {
		t.Fatal("the playbook is not offered to begin with")
	}

	if code := sendJSON(t, ts.URL+"/api/settings/skills/scam_guard", map[string]any{"on": false}, nil); code != http.StatusOK {
		t.Fatalf("switching off answered %d", code)
	}

	if _, ok := b.Agent.Registry.Get("scam_guard"); ok {
		t.Error("a skill switched off is still offered")
	}

	_ = sendJSON(t, ts.URL+"/api/settings/skills/scam_guard", map[string]any{"on": true}, nil)

	if _, ok := b.Agent.Registry.Get("scam_guard"); !ok {
		t.Error("switching back on did not offer it again")
	}

	agent := t.TempDir()
	_ = os.MkdirAll(filepath.Join(agent, "pdf-tips"), 0o755)
	_ = os.WriteFile(filepath.Join(agent, "pdf-tips", "SKILL.md"), []byte("---\nname: pdf-tips\ndescription: Working with PDF files.\n---\nUse the PDF forms page."), 0o644)

	if code := sendJSON(t, ts.URL+"/api/settings/skill-folders", map[string]any{"folders": []string{"relative/folder"}}, nil); code != http.StatusBadRequest {
		t.Errorf("a relative folder answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/settings/skill-folders", map[string]any{"folders": []string{agent}}, nil); code != http.StatusOK {
		t.Fatalf("folders answered %d", code)
	}

	if _, ok := b.Agent.Registry.Get("pdf_tips"); !ok {
		t.Error("a skill in an added folder was not learnt")
	}

	// Changed on disk, picked up by Reload.
	_ = os.MkdirAll(filepath.Join(agent, "travel"), 0o755)
	_ = os.WriteFile(filepath.Join(agent, "travel", "SKILL.md"), []byte("---\nname: travel\ndescription: Plan a trip.\n---\nAsk the dates."), 0o644)

	if code := sendJSON(t, ts.URL+"/api/skills/reload", map[string]any{}, nil); code != http.StatusOK {
		t.Fatalf("reload answered %d", code)
	}

	if _, ok := b.Agent.Registry.Get("travel"); !ok {
		t.Error("Reload did not pick up a new skill")
	}

	getJSON(t, ts.URL+"/api/skills/catalogue", &cat)

	kinds := map[string]bool{}
	for _, f := range cat.Folders {
		kinds[f.Kind] = true
	}

	if !kinds["mine"] || !kinds["added"] {
		t.Errorf("%+v", cat.Folders)
	}
}
