package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/sheet"
)

// The Tables page: a table looked over, a question answered and checked,
// and the result saved once — never over a file that is there.
func TestTheTablesPage(t *testing.T) {
	ts, _, b := newServer(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "sales.csv")
	_ = os.WriteFile(path, []byte("Region,Sales\nNorth,100\nSouth,250.50\nNorth,49.50\n"), 0o600)

	var looked struct {
		Summary struct {
			Rows    int `json:"rows"`
			Columns []struct {
				Name, Kind, Sum string
			} `json:"columns"`
		} `json:"summary"`
	}

	if code := sendJSON(t, ts.URL+"/api/tables/look", map[string]any{"path": path}, &looked); code != http.StatusOK ||
		looked.Summary.Rows != 3 || looked.Summary.Columns[1].Sum != "400" {
		t.Fatalf("%d %+v", code, looked)
	}

	b.Router = nil

	if code := sendJSON(t, ts.URL+"/api/tables/calculate", map[string]any{"path": path, "question": "by region"}, nil); code != http.StatusServiceUnavailable {
		t.Errorf("no model answered %d", code)
	}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", scribe{reply: `{"group_by":["Region"],"aggregate":[{"column":"Sales","fn":"sum"}]}`})
	b.Cfg.DefaultProvider = "ollama"

	var done struct {
		Rows   [][]string     `json:"rows"`
		Recipe map[string]any `json:"recipe"`
		Checks []struct {
			OK bool `json:"ok"`
		} `json:"checks"`
	}

	if code := sendJSON(t, ts.URL+"/api/tables/calculate", map[string]any{"path": path, "question": "by region"}, &done); code != http.StatusOK ||
		len(done.Rows) != 2 || done.Rows[0][1] != "149.5" {
		t.Fatalf("%d %+v", code, done)
	}

	for _, c := range done.Checks {
		if !c.OK {
			t.Errorf("a check failed: %+v", done.Checks)
		}
	}

	if code := sendJSON(t, ts.URL+"/api/tables/calculate", map[string]any{"path": path, "recipe": map[string]any{"group_by": []string{"Nope"}}}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("a missing column answered %d", code)
	}

	out := filepath.Join(dir, "result.xlsx")

	if code := sendJSON(t, ts.URL+"/api/tables/save", map[string]any{"path": path, "recipe": done.Recipe, "out": "result.xlsx"}, nil); code != http.StatusBadRequest {
		t.Errorf("a relative path answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/tables/save", map[string]any{"path": path, "recipe": done.Recipe, "out": out}, nil); code != http.StatusOK {
		t.Fatalf("save answered %d", code)
	}

	if info, err := os.Stat(out); err != nil || info.Size() == 0 {
		t.Fatalf("nothing was saved: %v", err)
	}

	if code := sendJSON(t, ts.URL+"/api/tables/save", map[string]any{"path": path, "recipe": done.Recipe, "out": out}, nil); code != http.StatusConflict {
		t.Errorf("saving over a file answered %d", code)
	}
}

// Every kind of finding the checks make is said in both languages.
func TestEveryTableFindingIsTranslated(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range sheet.FindingKeys() {
		if !strings.Contains(string(js), "'tab.f."+key+"': [") {
			t.Errorf("tab.f.%s is not in i18n.js", key)
		}
	}
}
