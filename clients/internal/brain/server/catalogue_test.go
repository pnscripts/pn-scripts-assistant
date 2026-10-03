package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/catalogue"
	"pn-scripts-assistant/internal/brain/models"
)

// The machine the bug was reported on: four cores, 31GB, no graphics card.
var reportedMachine = models.Power{Cores: 4, RAMBytes: 31 << 30}

var someLibrary = []catalogue.Model{
	{Name: "qwen3", What: "The latest Qwen.", Can: []string{"tools", "thinking"}, Sizes: []string{"0.6b", "8b", "235b"}},
	{Name: "nomic-embed-text", What: "An embedding model.", Can: []string{"embedding"}},
	{Name: "mystery", What: "Published in one size nobody named."},
}

func TestEverySizeIsJudgedAgainstThisMachine(t *testing.T) {
	out := listCatalogue(someLibrary, map[string]bool{"qwen3:8b": true}, reportedMachine)

	if len(out) != 3 {
		t.Fatalf("listed %d models, want 3", len(out))
	}

	qwen := out[0]

	if !qwen.AnyInstalled || len(qwen.Variants) != 3 {
		t.Fatalf("qwen3: %+v", qwen)
	}

	for _, v := range qwen.Variants {
		if v.NeedsGB <= 0 || v.Verdict == "" {
			t.Errorf("%s has a parameter count and still no estimate or verdict: %+v", v.Name, v)
		}
	}

	if v := qwen.Variants[1]; v.Name != "qwen3:8b" || v.Size != "8b" || !v.Installed || !v.Fits {
		t.Errorf("the installed 8b: %+v", v)
	}

	if v := qwen.Variants[2]; v.Fits || !strings.Contains(v.Verdict, "memory") {
		t.Errorf("235b on 31GB should not fit, and say why: %+v", v)
	}

	if v := out[1].Variants[0]; v.Name != "nomic-embed-text" || !v.Fits || v.Verdict == "" {
		t.Errorf("the embedding model: %+v", v)
	}

	/*
	 * Nothing to judge it by, so nothing is claimed.
	 *
	 * This used to be treated as wanting 1GB, which made every model with no
	 * listed size "quick enough on a processor" — a verdict about a number
	 * that had been made up.
	 */
	if v := out[2].Variants[0]; v.Name != "mystery" || v.NeedsGB != 0 || v.Verdict != "" || !v.Fits {
		t.Errorf("a model with no size: %+v", v)
	}
}

/*
 * The Models view shows what the catalogue sends, and never "undefined".
 *
 * The server began sending each model with its sizes in a list while the page
 * went on reading a flat model with a size and a verdict of its own, so every
 * row read "qwen3 · Undefined" and "undefined to download · undefined". Both
 * sides had tests and neither noticed, because neither was looking at the
 * other. This runs the page's own script on the server's own answer.
 */
func TestTheModelsViewShowsWhatTheCatalogueSends(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	script, err := assets.ReadFile("assets/js/install.js")
	if err != nil {
		t.Fatal(err)
	}

	answer, err := json.Marshal(map[string]any{
		"machine": reportedMachine.Describe(),
		"models":  listCatalogue(someLibrary, map[string]bool{"qwen3:8b": true}, reportedMachine),
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "install.js"), script, 0o644)
	os.WriteFile(filepath.Join(dir, "answer.json"), answer, 0o644)
	os.WriteFile(filepath.Join(dir, "run.js"), []byte(fakePage), 0o644)

	shown, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("the page script failed: %v\n%s", err, shown)
	}

	text := string(shown)

	if strings.Contains(strings.ToLower(text), "undefined") {
		t.Errorf("the Models view printed undefined:\n%s", text)
	}

	for _, want := range []string{
		"This machine: 4 cores, 31GB memory",
		"qwen3 · tools, thinking",
		"8b — installed",
		"Update",
		"usable on a processor",
		"nomic-embed-text",
		"mystery",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Models view does not show %q:\n%s", want, text)
		}
	}
}

// Just enough of a page for install.js to draw the catalogue into, after which
// every piece of text it put on screen is printed.
const fakePage = `
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];

class Node {
    constructor(tag) {
        this.tag = tag; this.children = []; this.dataset = {}; this.hidden = false;
        this._text = ''; this.className = ''; this.title = ''; this.value = '';
        this.classList = { add: (c) => { this.className += ' ' + c; }, remove: () => {} };
    }
    set textContent(t) { this._text = String(t); this.children = []; }
    get textContent() { return this._text + this.children.map((c) => c.textContent).join(''); }
    appendChild(c) { this.children.push(c); return c; }
    append(...cs) { for (const c of cs) this.appendChild(c); }
    setAttribute() {}
    addEventListener() {}
    contains() { return false; }
    lines() {
        if (this.hidden) return [];
        const own = this._text ? [this._text] : [];
        return own.concat(...this.children.map((c) => c.lines()));
    }
}

const ids = { 'catalogue-list': new Node('div'), 'catalogue-machine': new Node('p') };
const answer = JSON.parse(fs.readFileSync(path.join(dir, 'answer.json'), 'utf8'));

global.document = {
    getElementById: (id) => ids[id] || null,
    querySelectorAll: () => [],
    createElement: (tag) => new Node(tag),
    activeElement: null,
};

global.fetch = async (url) => ({
    ok: true,
    json: async () => (url === '/api/models/available' ? answer : {}),
});

eval(fs.readFileSync(path.join(dir, 'install.js'), 'utf8'));

setTimeout(() => {
    console.log(ids['catalogue-machine'].lines().join('\n'));
    console.log(ids['catalogue-list'].lines().join('\n'));
}, 50);
`
