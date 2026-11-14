package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/mcpimport"
)

// otherProgramsHome is a home with Claude Desktop and Cursor set up, and
// every variable that names a settings folder pointed into it.
func otherProgramsHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	write := func(path, text string) {
		os.MkdirAll(filepath.Dir(path), 0o755)

		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"),
		`{"mcpServers": {"memory": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-memory"], "env": {"MEM_TOKEN": "secret-value-9"}}}}`)
	write(filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers": {"old": {"type": "sse", "url": "https://old.example.com/sse"}}}`)

	return home
}

/*
 * What other programs have set up is listed without a model or the
 * network, never with a secret's value; importing one makes an integration
 * nobody has approved; one that cannot run here is refused with its reason.
 */
func TestServersInOtherProgramsAreListedAndImported(t *testing.T) {
	otherProgramsHome(t)

	ts, _, _ := newServer(t)

	var listed struct {
		Found      []brain.MCPFound `json:"found"`
		Files      []brain.MCPFile  `json:"found_files"`
		Problems   []string         `json:"problems"`
		FileStates []string         `json:"file_states"`
	}

	getJSON(t, ts.URL+"/api/integrations", &listed)

	if len(listed.Found) != 2 || len(listed.Files) != 4 || len(listed.Problems) != len(mcpimport.Problems()) || len(listed.FileStates) != 5 {
		t.Fatalf("%+v", listed)
	}

	res, err := http.Get(ts.URL + "/api/integrations")
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()

	if strings.Contains(string(raw), "secret-value-9") {
		t.Error("a secret's value is in the answer")
	}

	var memory, old brain.MCPFound

	for _, f := range listed.Found {
		switch f.Name {
		case "memory":
			memory = f
		case "old":
			old = f
		}
	}

	if memory.Title != "Claude Desktop" || memory.Problem != "" || memory.Key == "" || len(memory.Server.Secrets) != 1 {
		t.Errorf("memory %+v", memory)
	}

	if old.Problem != mcpimport.ProblemSSE {
		t.Errorf("old %+v", old)
	}

	code, body := postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(old.Key)+`}`)
	if code != 400 || !strings.Contains(body["error"].(string), "sse") {
		t.Errorf("the old transport was imported: %d %v", code, body)
	}

	code, body = postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(memory.Key)+`}`)
	// "memory" is a catalogue server's id: the imported one gets its own.
	if code != 200 || body["imported"] != "memory2" {
		t.Fatalf("import: %d %v", code, body)
	}

	code, _ = postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(memory.Key)+`}`)
	if code != 400 {
		t.Error("imported twice")
	}

	code, _ = postBody(t, ts.URL+"/api/integrations/import", `{"key":"nothing"}`)
	if code != 400 {
		t.Error("an entry that is not there was imported")
	}

	getJSON(t, ts.URL+"/api/integrations", &listed)

	for _, f := range listed.Found {
		if f.Name == "memory" && f.Imported != "memory2" {
			t.Errorf("not said to be imported: %+v", f)
		}
	}

	var all struct {
		Integrations []struct {
			ID       string `json:"id"`
			Approved bool   `json:"approved"`
			From     *struct {
				Client string `json:"client"`
			} `json:"from"`
		} `json:"integrations"`
	}

	getJSON(t, ts.URL+"/api/integrations", &all)

	seen := false

	for _, v := range all.Integrations {
		if v.ID == "memory2" {
			seen = true

			if v.Approved || v.From == nil || v.From.Client != "claude-desktop" {
				t.Errorf("imported as %+v", v)
			}
		}
	}

	if !seen {
		t.Error("the imported server is not listed")
	}
}

func quote(s string) string {
	raw, _ := json.Marshal(s)

	return string(raw)
}

// Every reason a found server cannot be imported, and every state of a
// settings file, has words on the page in both languages, with no dash.
func TestTheFoundServersAreSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	keys := []string{"org.imp.title", "org.imp.intro", "org.imp.none", "org.imp.import", "org.imp.cannot",
		"org.imp.files", "org.imp.watched", "org.int.from", "org.int.gone", "org.int.goneBadge"}

	for _, p := range mcpimport.Problems() {
		keys = append(keys, "org.imp.p."+p)
	}

	for _, s := range brain.MCPFileStates() {
		keys = append(keys, "org.imp.f."+s)
	}

	for _, key := range keys {
		en, bg := strings2(t, js, key)

		if en == "" || !bulgarianLetters.MatchString(bg) {
			t.Errorf("%s: %q / %q", key, en, bg)
		}

		if strings.ContainsAny(en+bg, "–—") {
			t.Errorf("%s uses a dash as punctuation: %q / %q", key, en, bg)
		}
	}
}

// Each program's name is on the page as mcpimport says it.
func TestTheProgramsAreNamedAsTheServerNamesThem(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	for _, c := range mcpimport.Clients {
		en, bg := strings2(t, js, "org.imp.c."+string(c))
		if en != c.Title() || bg != c.Title() {
			t.Errorf("%s: %q / %q, want %q", c, en, bg, c.Title())
		}
	}
}
