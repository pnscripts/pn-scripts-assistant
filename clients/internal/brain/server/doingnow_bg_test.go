package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Doing now and the record say a tool's words in the page's language.
 *
 * Found in the 1.1.0 step 9 harness: on a Bulgarian page the approval card
 * read "Винаги се пита (...): Създаване на ...", while the line above it in
 * Doing now read "Чака вашето решение: Always asked (...): Create ..." and
 * the record of what was done listed the same English. The server now hands
 * the Bulgarian card with each step and each deed (summary_bg), and the
 * page's own scripts, run here in node on the server's real answers, put it
 * where the English was: on a Bulgarian page only, with the English page
 * and anything written before unchanged.
 */
func TestDoingNowAndTheRecordSayTheCardInThePagesLanguage(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, _ := newServer(t)

	const (
		create   = "Always asked (a protected file): Create /home/you/zz-doing-now.txt"
		createBG = "Винаги се пита (защитен файл): Създаване на /home/you/zz-doing-now.txt"
		odd      = "Run: echo {1} $& zz-doing-now\nThis waits for your quiet hours."
		oddBG    = "Изпълнение: echo {1} $& zz-doing-now\nТова чака тихите ви часове."
	)

	progress.Begin()
	progress.SetToolIn("read_file", "Read zz-doing-now.txt", "Прочитане на zz-doing-now.txt")
	progress.SetTool("list_directory", "List zz-doing-now")
	progress.WaitingIn("Waiting for you: "+create, createBG, 990001)
	progress.WaitingIn("Waiting for you: "+create+" (2)", createBG+" (2)", 990002)
	progress.WaitingIn("Waiting for you: "+create+" (3)", createBG+" (3)", 990003)
	progress.WaitingIn("Waiting for you: "+odd, oddBG, 990004)
	progress.Waiting("Waiting for you: Write zz-doing-now-old.txt", 990005)
	progress.WaitingIn("Waiting for you: "+create+" (now)", createBG+" (сега)", 990006)

	now := fetched(t, ts.URL+"/api/progress")

	progress.Done()

	progress.Decided(990002, progress.Refused)
	progress.Decided(990003, progress.Approved)
	progress.Decided(990004, progress.ByItself)
	progress.Decided(990005, progress.Refused)

	for _, d := range []store.Deed{
		{Tool: "write_file", Summary: create, SummaryBG: createBG, Asked: "path: /home/you/zz-doing-now.txt", Outcome: "waiting", Changes: true},
		{Tool: "read_file", Summary: "Read zz-old.txt", Asked: "path: zz-old.txt · lines: 3", Outcome: "done"},
		{Tool: "list_directory", Summary: "List zz", Outcome: "done"},
		{Tool: "write_file", Summary: "Create zz-decided.txt", Asked: `{"content":"milk, bread.","path":"/home/you/zz-decided.txt"}`, Outcome: "refused"},
	} {
		if _, err := db.AddDeed(d); err != nil {
			t.Fatal(err)
		}
	}

	steps, deeds := fetched(t, ts.URL+"/api/steps"), fetched(t, ts.URL+"/api/deeds?since=day")

	en := doingNowSays(t, "en-GB", now, steps, deeds)
	bg := doingNowSays(t, "bg-BG", now, steps, deeds)

	// The steps under an answer: the tool's Bulgarian words, the tool's
	// name for a step without them, and whatever else as it came.
	if got, want := strings.Join(en.Did, "|"), "Read notes.txt|Read notes.txt|not this expert's tool: x|"; got != want {
		t.Errorf("the steps under an answer on the English page: %q, want %q", got, want)
	}

	if got, want := strings.Join(bg.Did, "|"), "Прочитане на notes.txt|Четене на файл|not this expert's tool: x|"; got != want {
		t.Errorf("the steps under an answer on the Bulgarian page: %q, want %q", got, want)
	}

	for _, c := range []struct {
		what    string
		en, bg  string
		said    func(doingNowSaid) string
		because string
	}{
		{"a tool running", "Read zz-doing-now.txt", "Прочитане на zz-doing-now.txt",
			func(s doingNowSaid) string { return s.Steps["Read zz-doing-now.txt"] }, "its own Bulgarian words"},
		{"a tool with no Bulgarian card", "List zz-doing-now", "Съдържание на папка",
			func(s doingNowSaid) string { return s.Steps["List zz-doing-now"] }, "the tool's name, as before"},
		{"a waiting question", "Waiting for you: " + create, "Чака вашето решение: " + createBG,
			func(s doingNowSaid) string { return s.Steps["Waiting for you: "+create] }, "the approval card's words"},
		{"a refused question", "Refused: " + create + " (2)", "Отказано: " + createBG + " (2)",
			func(s doingNowSaid) string { return s.Steps["Refused: "+create+" (2)"] }, "the same card under Refused"},
		{"an approved question", "Approved: " + create + " (3)", "Одобрено: " + createBG + " (3)",
			func(s doingNowSaid) string { return s.Steps["Approved: "+create+" (3)"] }, "the same card under Approved"},
		{"a question that started by itself", "Started by itself: " + odd, "Започна автоматично: " + oddBG,
			func(s doingNowSaid) string { return s.Steps["Started by itself: "+odd] }, "several lines, and signs kept as written"},
		{"a question asked before the card was kept", "Refused: Write zz-doing-now-old.txt", "Отказано: Write zz-doing-now-old.txt",
			func(s doingNowSaid) string { return s.Steps["Refused: Write zz-doing-now-old.txt"] }, "shown as it came"},
		{"the step running now", "Waiting for you: " + create + " (now)", "Чака вашето решение: " + createBG + " (сега)",
			func(s doingNowSaid) string { return s.Now }, "the current step carries the card too"},
		{"the record of a question", create, createBG,
			func(s doingNowSaid) string { return s.Deeds[create] }, "the approval card's words"},
		{"the record's tooltip", create, createBG,
			func(s doingNowSaid) string { return s.Titles[create] }, "the same line in full"},
		{"an older record", "Read zz-old.txt", "Четене на файл · zz-old.txt",
			func(s doingNowSaid) string { return s.Deeds["Read zz-old.txt"] }, "the tool's name and what it was given, as before"},
		{"an older record of a decided question", "Create zz-decided.txt", "Запис на файл",
			func(s doingNowSaid) string { return s.Deeds["Create zz-decided.txt"] }, "the tool's name, not a piece of its arguments"},
		{"an older record with nothing given", "List zz", "Съдържание на папка",
			func(s doingNowSaid) string { return s.Deeds["List zz"] }, "the tool's name, as before"},
	} {
		if got := c.said(en); got != c.en {
			t.Errorf("%s on the English page: %q, want %q", c.what, got, c.en)
		}

		if got := c.said(bg); got != c.bg {
			t.Errorf("%s on the Bulgarian page: %q, want %q (%s)", c.what, got, c.bg, c.because)
		}
	}
}

type doingNowSaid struct {
	Now    string            `json:"now"`
	Steps  map[string]string `json:"steps"`
	Deeds  map[string]string `json:"deeds"`
	Titles map[string]string `json:"titles"`
	Did    []string          `json:"did"`
}

func fetched(t *testing.T, url string) []byte {
	t.Helper()

	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("%s answered %d (%v): %s", url, res.StatusCode, err, raw)
	}

	return raw
}

// doingNowSays runs i18n.js, status.js and deeds.js in node, with the
// browser's language set to language, on the server's answers: each step
// and each deed by its English line, as the page would say it.
func doingNowSays(t *testing.T, language string, now, steps, deeds []byte) doingNowSaid {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "status.js", "deeds.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "now.json"), now, 0o644)
	os.WriteFile(filepath.Join(dir, "steps.json"), steps, 0o644)
	os.WriteFile(filepath.Join(dir, "deeds.json"), deeds, 0o644)

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
const answer = (name) => { const body = JSON.parse(read(name)); return body.data !== undefined ? body.data : body; };
(0, eval)(read('i18n.js'));
(0, eval)(read('status.js'));
(0, eval)(read('deeds.js'));
const said = { now: '', steps: {}, deeds: {}, titles: {} };
const now = answer('now.json');
said.now = window.pnStepLine(now.note, now);
for (const step of answer('steps.json').steps) said.steps[step.note] = window.pnStepLine(step.note, step);
said.did = [{ tool: 'read_file', summary: 'Read notes.txt', summary_bg: 'Прочитане на notes.txt' },
    { tool: 'read_file', summary: 'Read notes.txt' }, { summary: 'not this expert\'s tool: x' }, {}].map(window.pnDidLine);
for (const deed of answer('deeds.json').deeds) {
    said.deeds[deed.summary] = window.pnDeedLine(deed);
    said.titles[deed.summary] = window.pnDeedLine(deed, true);
}
process.stdout.write(JSON.stringify(said));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said doingNowSaid
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}

/*
 * The top bar keeps every button on a phone.
 *
 * At 390px the last button of the bar, System, ended past the edge on the
 * Command centre: the rule that lets the turner shrink stood above the
 * turner's own "flex: none" and lost to it. The rules for a narrow bar must
 * come after the ones they adjust, name the bar, let the turner shrink and
 * keep the buttons from being squeezed. The harness measures the result at
 * 390px in both languages; this keeps the rules where they work.
 */
func TestTheNarrowTopBarShrinksTheTurnerNotTheButtons(t *testing.T) {
	css := readAsset(t, filepath.Join("css", "console.css"))

	base := strings.LastIndex(css, "\n.ring-turner {")
	if base < 0 {
		t.Fatal("console.css has no .ring-turner rule")
	}

	after := css[base:]

	for _, rule := range []string{
		".topbar .ring-turner { flex: 0 1 auto; }",
		".topbar .icon-buttons,\n    .topbar .icon-button { flex: none; }",
		".topbar .topbar-gap { display: none; }",
	} {
		if !strings.Contains(after, rule) {
			t.Errorf("after the turner's own rule nothing says %q, so a narrow bar pushes its last button off the edge", rule)
		}
	}

	// The same thing said before the turner's own rule is overruled by it.
	for _, dead := range []string{"\n    .ring-turner { flex: 0 1 auto; }", "\n    .ring-here { max-width: 120px; }"} {
		if strings.Contains(css[:base], dead) {
			t.Errorf("%q stands before the turner's own rule, which overrules it", strings.TrimSpace(dead))
		}
	}

	// The name of the panel is what shortens: it must be able to.
	here := css[strings.LastIndex(css, "\n.ring-here {"):]
	here = here[:strings.Index(here, "}")]

	for _, needs := range []string{"min-width: 0", "overflow: hidden", "text-overflow: ellipsis", "white-space: nowrap"} {
		if !strings.Contains(here, needs) {
			t.Errorf(".ring-here lacks %q, so its name cannot shorten", needs)
		}
	}
}

/*
 * Where a question to Claude went is said in Bulgarian in the record.
 *
 * Seen in the harness beside the cards this file is about: the record of
 * an approved question to Claude was Bulgarian down to its last line,
 * "Claude (Anthropic), under your own sign-in or key", which the page had
 * no words for. Checked against what the tool itself says, so a change of
 * wording there fails here.
 */
func TestWhereAQuestionToClaudeWentIsSaidInBulgarian(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	where := tools.AskClaude{}.Reaches(nil)
	if where == "" {
		t.Fatal("ask_claude no longer says where it reaches")
	}

	entry := "'" + where + "': '"

	at := strings.Index(js, entry)
	if at < 0 {
		t.Fatalf("i18n.js LEAVES has no Bulgarian for %q", where)
	}

	bg := js[at+len(entry):]
	bg = bg[:strings.Index(bg, "'")]

	if !tools.SaysBulgarian(bg) {
		t.Errorf("%q is said as %q on a Bulgarian page", where, bg)
	}
}
