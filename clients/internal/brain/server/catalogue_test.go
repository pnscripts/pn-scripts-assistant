package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	{Name: "gemma3", What: "Lightweight.", Can: []string{"vision"}, Sizes: []string{"27b", "270m", "4b", "12b", "1b"}},
	{Name: "gemma3n", What: "Everyday devices.", Sizes: []string{"e2b", "e4b"}},
	{Name: "dolphin-mixtral", What: "Experts.", Sizes: []string{"8x22b", "8x7b"}},
}

func TestEverySizeIsJudgedAgainstThisMachine(t *testing.T) {
	out := listCatalogue(someLibrary, map[string]bool{"qwen3:8b": true}, reportedMachine)

	if len(out) != len(someLibrary) {
		t.Fatalf("listed %d models, want %d", len(out), len(someLibrary))
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
 * Sizes are listed smallest first, and comfortable is told apart from fits.
 *
 * On four cores and 31GB a 12b fits and takes minutes per answer, so it is
 * not where the interface should start; a 4b is. A size with no number to go
 * on is listed last and judged not at all.
 */
func TestSizesAreOrderedAndComfortIsSeparateFromFitting(t *testing.T) {
	out := listCatalogue(someLibrary, nil, reportedMachine)

	byName := map[string]Listed{}
	for _, m := range out {
		byName[m.Name] = m
	}

	var order []string
	comfortable := map[string]bool{}

	for _, v := range byName["gemma3"].Variants {
		order = append(order, v.Size)
		comfortable[v.Size] = v.Comfortable

		if !v.Fits {
			t.Errorf("%s does not fit in 31GB: %+v", v.Name, v)
		}
	}

	if strings.Join(order, " ") != "270m 1b 4b 12b 27b" {
		t.Errorf("gemma3 sizes are listed as %v", order)
	}

	for size, want := range map[string]bool{"270m": true, "1b": true, "4b": true, "12b": false, "27b": false} {
		if comfortable[size] != want {
			t.Errorf("gemma3:%s comfortable on four cores is %v, want %v", size, comfortable[size], want)
		}
	}

	for _, v := range byName["gemma3n"].Variants {
		if v.NeedsGB != 0 || v.Verdict != "" || v.Comfortable || !v.Fits {
			t.Errorf("an effective size was judged: %+v", v)
		}
	}

	if v := byName["dolphin-mixtral"].Variants; v[0].Size != "8x7b" || v[0].Fits || v[1].Fits {
		t.Errorf("every expert counts, and neither mixture fits in 31GB: %+v", v)
	}

	// A card with room for it is comfortable; spilling onto the processor is not.
	card := models.Power{Cores: 8, RAMBytes: 64 << 30, Accelerated: true, VRAMBytes: 12 << 30}

	for _, v := range listCatalogue(someLibrary[:1], nil, card)[0].Variants {
		if want := v.NeedsGB <= 12; v.Comfortable != want {
			t.Errorf("%s on a 12GB card: comfortable %v, want %v", v.Name, v.Comfortable, want)
		}
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
	css, err := assets.ReadFile("assets/css/console.css")
	if err != nil {
		t.Fatal(err)
	}

	// Model names are shown as they are typed: "all-minilm", not "All-Minilm".
	if !regexp.MustCompile(`\.permit-name\.as-typed\s*\{[^}]*text-transform:\s*none`).Match(css) {
		t.Error("nothing stops the capitalising .permit-name rule from changing model names")
	}

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

	if strings.Contains(text, "not shown as typed") {
		t.Errorf("a model name is drawn without the as-typed class, so CSS capitalises it")
	}

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
		// Not installed, so it opens on the largest size that runs well
		// here rather than the largest that fits.
		"gemma3 · vision\nLightweight.\nwants about 4GB while running · usable on a processor — seconds, not instant\nchosen: 4b",
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
        this.classes = () => this.className.trim().split(/\s+/);
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
        if (this.tag === 'select') {
            const chosen = this.children.find((o) => o.selected);
            return ['chosen: ' + (chosen ? chosen._text : 'nothing')];
        }
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
    const names = [];
    const walk = (n) => {
        if (n.classes().includes('permit-name')) names.push(n);
        n.children.forEach(walk);
    };
    walk(ids['catalogue-list']);
    if (!names.length || names.some((n) => !n.classes().includes('as-typed'))) {
        console.log('a model name is not shown as typed');
    }
    console.log(ids['catalogue-machine'].lines().join('\n'));
    console.log(ids['catalogue-list'].lines().join('\n'));
}, 50);
`
