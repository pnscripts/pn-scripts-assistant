package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Two small things a Bulgarian page still said in English.
 *
 * Found in the 1.1.0 step 10 harness. Under a Doing now step the line that
 * is only a duration read "0ms" while every line with words beside it read
 * "0 мс": the page knew the shapes of the lines and a bare duration is none
 * of them. And a message already in the conversation kept the language it
 * was drawn in when the page's language was switched: who said it, what was
 * done under the answer and how long each thing took stayed as they were
 * until a reload.
 *
 * The page's own scripts are run here in node, as the page loads them.
 */

// pageSays runs the given page scripts in node after i18n.js, with the
// browser's language set, then the test's own lines, and returns what they
// wrote. The page here has just enough of a document to draw a message.
func pageSays(t *testing.T, language string, scripts []string, lines string) []byte {
	t.Helper()

	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()

	load := ""

	for _, name := range append([]string{"i18n.js"}, scripts...) {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)

		load += "(0, eval)(read('" + name + "'));\n"
	}

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
Object.defineProperty(globalThis, 'navigator', { value: { language: process.argv[3] }, configurable: true });
globalThis.window = globalThis;
const heard = {};
window.addEventListener = (name, fn) => { (heard[name] = heard[name] || []).push(fn); };
window.dispatchEvent = (event) => { (heard[event.type] || []).forEach((fn) => fn(event)); return true; };
globalThis.CustomEvent = function (type, init) { this.type = type; this.detail = init && init.detail; };
function element(tag) {
    const node = { tagName: tag, className: '', textContent: '', children: [], dataset: {}, open: false,
        parent: null, isConnected: true,
        appendChild(child) { child.parent = node; node.children.push(child); return child; },
        append(...all) { all.forEach((child) => node.appendChild(child)); },
        replaceWith(other) {
            const at = node.parent.children.indexOf(node);
            other.parent = node.parent;
            node.parent.children[at] = other;
            node.parent = null;
        },
    };
    return node;
}
globalThis.document = { readyState: 'complete', querySelectorAll: () => [], querySelector: () => null,
    getElementById: () => null, documentElement: { dataset: {}, style: { setProperty() {} } },
    addEventListener: () => {}, createElement: element };
globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.setInterval = () => 0;
globalThis.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
`+load+lines), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	return out
}

func TestALoneDurationUnderAStepIsSaidInBulgarian(t *testing.T) {
	lines := []string{"0ms", "80ms", "1.5s", "59.9s", "1m 05s", "12m 00s",
		// Not durations: left exactly as they came.
		"3 lines back", "1.2.3", "42", "ms", "5s later", "nothing back"}

	said := func(language string) []string {
		raw, _ := json.Marshal(lines)

		out := pageSays(t, language, []string{"status.js"},
			`process.stdout.write(JSON.stringify(`+string(raw)+`.map((line) => window.pnStepLine(line))));`)

		var got []string
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("the page said %s: %v", out, err)
		}

		return got
	}

	if got, want := strings.Join(said("en-GB"), "|"), strings.Join(lines, "|"); got != want {
		t.Errorf("the English page changed the lines: %q, want %q", got, want)
	}

	want := "0 мс|80 мс|1,5 с|59,9 с|1 мин 05 с|12 мин 00 с|3 lines back|1.2.3|42|ms|5s later|nothing back"

	if got := strings.Join(said("bg-BG"), "|"); got != want {
		t.Errorf("the Bulgarian page said %q, want %q", got, want)
	}
}

type drawnMessage struct {
	Who   string   `json:"who"`
	Steps []string `json:"steps"`
	Times []string `json:"times"`
	Kept  []string `json:"kept"`
	Open  []bool   `json:"open"`
	Other string   `json:"other"`
	Gone  int      `json:"gone"`
}

func TestMessagesAlreadyDrawnFollowTheLanguageOfThePage(t *testing.T) {
	const lines = `
const T = window.pnTranscript;
const steps = [
    { tool: 'read_file', summary: 'Read notes.txt', summary_bg: 'Прочитане на notes.txt', millis: 1500, asked: 'path: notes.txt', result: 'milk' },
    { tool: 'list_directory', summary: 'List zz', millis: 0 },
    { summary: 'not this tool: x', millis: 80, failed: true },
    {},
];
function message(who, list) {
    const wrap = document.createElement('div');
    const label = document.createElement('div');
    label.textContent = T.who(who);
    wrap.appendChild(label);
    let box = null;
    if (list) { box = T.steps(list); wrap.appendChild(box); }
    let other = 0;
    T.keep(wrap, { label, who, steps: list, box, also: () => { other += 1; wrap.dataset.other = String(other); } });
    return wrap;
}
const mine = message('you');
const answer = message('Ada', steps);
const gone = message('error');
function shown() {
    const box = answer.children[1];
    return {
        who: [mine.children[0].textContent, answer.children[0].textContent].join('|'),
        steps: box.children.map((row) => row.children[0].children[0].textContent),
        times: box.children.map((row) => row.children[0].children[1].textContent),
        kept: box.children.map((row) => row.children[1].children.map((part) => part.className + '=' + part.textContent).join(';')),
        open: box.children.map((row) => row.open),
        other: [mine.dataset.other || '', answer.dataset.other || ''].join('|'),
        gone: Number(gone.dataset.other || 0),
    };
}
const said = [shown()];
// Somebody opened the first thing done, then switched the page's language.
answer.children[1].children[0].open = true;
// A message of a conversation that was cleared away is not drawn again.
gone.isConnected = false;
window.pnSetLanguage(window.pnLang() === 'bg' ? 'en' : 'bg');
said.push(shown());
window.pnSetLanguage(window.pnLang() === 'bg' ? 'en' : 'bg');
said.push(shown());
process.stdout.write(JSON.stringify(said));
`

	english := drawnMessage{
		Who:   "you|Ada",
		Steps: []string{"Read notes.txt", "List zz", "not this tool: x", "did something"},
		Times: []string{"1.5s", "0ms", "80ms", "0ms"},
		Kept: []string{
			"step-tool=read_file;step-asked=path: notes.txt;step-result=milk",
			"step-tool=list_directory",
			"",
			"",
		},
	}

	bulgarian := drawnMessage{
		Who:   "вие|Ada",
		Steps: []string{"Прочитане на notes.txt", "Съдържание на папка", "not this tool: x", "направи нещо"},
		Times: []string{"1,5 с", "0 мс", "80 мс", "0 мс"},
		Kept: []string{
			"step-tool=read_file;step-asked=path: notes.txt;step-result=milk",
			// No Bulgarian words came with it: named by its tool, the
			// English summary kept where it can be read.
			"step-tool=list_directory;step-tool=List zz",
			"",
			"",
		},
	}

	for _, c := range []struct {
		language      string
		first, second drawnMessage
	}{
		{"en-GB", english, bulgarian},
		{"bg-BG", bulgarian, english},
	} {
		out := pageSays(t, c.language, []string{"status.js", "transcript.js"}, lines)

		var got []drawnMessage
		if err := json.Unmarshal(out, &got); err != nil || len(got) != 3 {
			t.Fatalf("the page said %s: %v", out, err)
		}

		for i, want := range []drawnMessage{c.first, c.second, c.first} {
			at := []string{"as drawn", "after the language was switched", "after it was switched back"}[i]

			want.Open = []bool{i > 0, false, false, false}
			want.Other = []string{"|", "1|1", "2|2"}[i]

			a, _ := json.Marshal(got[i])
			b, _ := json.Marshal(want)

			if string(a) != string(b) {
				t.Errorf("a page opened in %s, %s:\n got %s\nwant %s", c.language, at, a, b)
			}
		}
	}
}

// The script that redraws the conversation is loaded like every page
// script, before the console that draws with it.
func TestTheConversationsRedrawIsLoadedBeforeTheConsole(t *testing.T) {
	page := readAsset(t, "index.html")

	at := strings.Index(page, `<script src="/js/transcript.js"></script>`)
	if at < 0 {
		t.Fatal("index.html does not load transcript.js")
	}

	if at > strings.Index(page, `<script src="/js/connection.js"></script>`) {
		t.Error("transcript.js is loaded after connection.js, which must stand immediately before console.js")
	}

	if at < strings.Index(page, `<script src="/js/status.js"></script>`) {
		t.Error("transcript.js is loaded before status.js, whose words it says")
	}
}

/*
 * The name on the top bar follows the page's language.
 *
 * Found in the real window with a Bulgarian system language: every panel
 * was Bulgarian and the bar above them said "Command centre", until the
 * first turn. The stage writes the name when it turns, and the language
 * arrives after the first drawing (the program says which, once it has
 * answered), so the name stayed as first drawn. The stage now says where
 * it is again when asked (retitle), and the page asks on a change of
 * language. Proven in the harness, which reads the bar before and after a
 * switch; this keeps the two ends joined.
 */
func TestTheTopBarsNameIsSaidAgainWhenTheLanguageChanges(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "stage", "stage.js"))
	if err != nil {
		t.Fatal(err)
	}

	stage := string(raw)

	back := stage[strings.LastIndex(stage, "\n    return {"):]

	if !strings.Contains(back, "retitle: sayWhere,") {
		t.Error("the stage does not hand back a way to say where it is again (retitle)")
	}

	scenes := readAsset(t, filepath.Join("js", "scenes.js"))

	if !strings.Contains(scenes, "const stage = startStage({") {
		t.Fatal("scenes.js no longer keeps the stage it starts")
	}

	at := strings.Index(scenes, "window.addEventListener('pn-language'")
	if at < 0 || !strings.Contains(scenes[at:at+120], "stage.retitle()") {
		t.Error("scenes.js does not ask the stage to say where it is again when the language changes")
	}
}
