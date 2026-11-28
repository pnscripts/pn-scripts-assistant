package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/mcp"
)

// integrationWords is what the Organisation view says of one integration:
// its title, why it is there and the badge beside it.
type integrationWords struct {
	Title   string `json:"title"`
	Why     string `json:"why"`
	Badge   string `json:"badge"`
	Licence string `json:"licence"`
}

/*
 * The integrations list says the catalogue in the page's language, and its
 * badge says no more than its sentence.
 *
 * Seen 15.11 on a Bulgarian page: every catalogue server was titled and
 * explained in English ("the protocol's own reference server..."), because
 * the page drew the Go catalogue's own words. And a program imported from
 * another program, whose sentence says it "may reach the internet", carried
 * the badge "leaves this machine", which says it does. This runs the page's
 * own script on the server's real answer, with a catalogue and an imported
 * program in it, in both languages.
 */
func TestTheIntegrationsAreSaidInThePagesLanguageAndTheBadgeMatchesTheSentence(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	otherProgramsHome(t)

	ts, _, _ := newServer(t)

	var listed struct {
		Found []brain.MCPFound `json:"found"`
	}

	getJSON(t, ts.URL+"/api/integrations", &listed)

	imported := false

	for _, f := range listed.Found {
		if f.Name == "memory" {
			code, body := postBody(t, ts.URL+"/api/integrations/import", `{"key":`+quote(f.Key)+`,"print":`+quote(f.Print)+`}`)
			if code != 200 {
				t.Fatalf("import: %d %v", code, body)
			}

			imported = true
		}
	}

	if !imported {
		t.Fatal("the program set up in Claude Desktop was not found")
	}

	res, err := http.Get(ts.URL + "/api/integrations")
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()

	en := integrationsSaid(t, "en-GB", raw)
	bg := integrationsSaid(t, "bg-BG", raw)

	cyrillic := regexp.MustCompile(`[А-Яа-я]`)

	for _, s := range mcp.Catalogue() {
		e, b := en[s.ID], bg[s.ID]

		if e.Title != s.Title || e.Why != s.Why {
			t.Errorf("%s in English reads %+v, want the catalogue's own %q, %q", s.ID, e, s.Title, s.Why)
		}

		// A product's name is the same in every language.
		name := s.ID == "git"

		if (!name && !cyrillic.MatchString(b.Title)) || !cyrillic.MatchString(b.Why) {
			t.Errorf("%s on a Bulgarian page reads %+v", s.ID, b)
		}

		if b.Why == s.Why || (!name && b.Title == s.Title) {
			t.Errorf("%s on a Bulgarian page is the English: %+v", s.ID, b)
		}

		// A licence is its identifiers, the same in every language, and
		// whatever words join them in the page's.
		if e.Licence != s.Licence {
			t.Errorf("%s licence in English: %q, want %q", s.ID, e.Licence, s.Licence)
		}

		if words := regexp.MustCompile(`\b[a-z]{2,}\b`).FindAllString(b.Licence, -1); len(words) > 0 {
			t.Errorf("%s licence on a Bulgarian page has English words %q: %q", s.ID, words, b.Licence)
		}

		if strings.ContainsAny(b.Why+b.Title, "—–") {
			t.Errorf("%s carries a dash: %+v", s.ID, b)
		}
	}

	// The catalogue's web fetch reaches the internet itself: it leaves.
	if en["fetch"].Badge != "leaves this machine" || bg["fetch"].Badge != "напуска компютъра" {
		t.Errorf("web fetch badge: %q, %q", en["fetch"].Badge, bg["fetch"].Badge)
	}

	// Nothing says the imported one does, only that it may.
	if en["memory2"].Badge != "may leave this machine" || bg["memory2"].Badge != "може да напуска компютъра" {
		t.Errorf("the imported program's badge: %q, %q", en["memory2"].Badge, bg["memory2"].Badge)
	}

	// Its own words are its own: what another program called it is shown as
	// given, never replaced by the catalogue's entry of the same name.
	if en["memory2"].Title == en["memory"].Title && bg["memory2"].Title == bg["memory"].Title && bg["memory"].Title != "" {
		t.Errorf("the imported program took the catalogue's title: %+v", bg["memory2"])
	}

	// A program on this machine with no network says nothing of leaving.
	if en["git"].Badge != "" || bg["git"].Badge != "" {
		t.Errorf("git badge: %q, %q", en["git"].Badge, bg["git"].Badge)
	}
}

// integrationsSaid runs i18n.js and equip.js in node with the browser's
// language set, and returns what the page says of each integration by id.
func integrationsSaid(t *testing.T, language string, answer []byte) map[string]integrationWords {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "equip.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "integrations.json"), answer, 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
Object.defineProperty(globalThis, 'navigator', { value: { language: process.argv[3] }, configurable: true });
globalThis.window = globalThis;
window.dispatchEvent = () => true;
window.addEventListener = () => {};
globalThis.CustomEvent = function () {};
globalThis.document = { readyState: 'complete', querySelectorAll: () => [], querySelector: () => null,
    getElementById: () => null, documentElement: { dataset: {}, style: { setProperty() {} } },
    addEventListener: () => {} };
globalThis.setInterval = () => 0;
globalThis.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
(0, eval)(read('i18n.js'));
(0, eval)(read('equip.js'));
const body = JSON.parse(read('integrations.json'));
const list = body.data !== undefined ? body.data.integrations : body.integrations;
const said = {};
for (const v of list) said[v.id] = window.pnIntegrationWords(v);
process.stdout.write(JSON.stringify(said));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	said := map[string]integrationWords{}
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}
