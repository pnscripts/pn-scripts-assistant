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

	"pn-scripts-assistant/internal/brain/store"
)

type outsidePage struct {
	Off, On struct {
		Fixed struct{ Label, State string }
		Rows  []struct {
			ID      string
			Label   string
			Allowed bool
			State   string
		}
		Recent []struct{ When, Who, What, Outcome string }
		None   string
	}
	Card struct {
		Approve    bool
		ForProgram any
		AnyProgram any
		AskedBy    *struct{ Label, Title string }
	}
	Desk struct {
		AskedBy any
	}
	Unnamed string
	Deeds   []struct {
		Chip, Said, Title, Trait, Went, Result string
	}
	Tool      struct{ Chip, Trait, Went string }
	Mine      struct{ Chip, Result string }
	Turned    []string
	Elsewhere string
}

/*
 * What the page says about other programs asking, worked out by
 * programs.js in node on the real server's answers, in English and in
 * Bulgarian: the switch off and on, the last questions, the card of a
 * waiting question another program asked for, and the record.
 */
func TestThePageSaysWhoAskedAndWhatMayBeAskedInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, _ := newServer(t)
	twoPrograms(t, db)

	read := func(path string) []byte {
		t.Helper()

		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}

		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)

		return raw
	}

	off := read("/api/outside")

	// Refused, allowed, answered, and one from a program with no name.
	post(t, ts.URL+"/api/outside/ask", `{"tool":"list_programs","client":{"name":"claude-code","version":"2.1.0"}}`)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"programs","allowed":true}`)
	post(t, ts.URL+"/api/outside/ask", `{"tool":"list_programs","arguments":{"kind":"app"},"client":{"name":"Cursor","version":"3.0"}}`)
	post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status"}`)

	// A waiting question another program asked for, one from the desk,
	// and a thing done at the desk.
	id, _ := db.RecordInvocation(0, "write_file", `{}`, "Always asked (another program asked for it): Write notes.txt", "mutating")
	db.InvocationAskedBy(id, "claude-code 2.1.0")
	db.RecordInvocation(0, "write_file", `{}`, "Write other.txt", "mutating")

	// And a tool of the assistant's own that another program asked for
	// and its person approved.
	db.AddDeed(store.Deed{Tool: "write_file", Summary: "Write notes.txt", Outcome: "done", Changes: true, AskedBy: "Cursor 3.0"})

	on := read("/api/outside")
	cards := read("/api/approvals")
	deeds := read("/api/deeds?since=all")

	en := outsidePageSays(t, "en-GB", off, on, cards, deeds)
	bg := outsidePageSays(t, "bg-BG", off, on, cards, deeds)

	// The switch, off: one fixed line and one switch, not allowed.
	if en.Off.Fixed.Label != "Whether I am running, and which version I am" || en.Off.Fixed.State != "Always answered" ||
		len(en.Off.Rows) != 1 || en.Off.Rows[0].ID != "programs" || en.Off.Rows[0].Allowed ||
		en.Off.Rows[0].Label != "Which programs are installed on this computer" ||
		en.Off.Rows[0].State != "Not allowed. A program that asks is told to ask you." ||
		len(en.Off.Recent) != 0 || en.Off.None != "No program has asked anything yet." {
		t.Errorf("in English, off: %+v", en.Off)
	}

	if bg.Off.Fixed.Label != "Дали работя и коя версия съм" || bg.Off.Fixed.State != "Винаги се отговаря" ||
		bg.Off.Rows[0].Label != "Кои програми са инсталирани на този компютър" ||
		bg.Off.Rows[0].State != "Не е разрешено. Програма, която попита, получава отговор да се обърне към вас." ||
		bg.Off.None != "Още никоя програма не е питала нищо." {
		t.Errorf("in Bulgarian, off: %+v", bg.Off)
	}

	// On: since when, and the last questions, newest first.
	if !en.On.Rows[0].Allowed || !regexp.MustCompile(`^Allowed since \d{1,2} \w{3} \d{4}, \d{2}:\d{2}\.$`).MatchString(en.On.Rows[0].State) {
		t.Errorf("in English, on: %+v", en.On.Rows)
	}

	if !regexp.MustCompile(`^Разрешено от \d{2}\.\d{2}\.\d{4}, \d{2}:\d{2}\.$`).MatchString(bg.On.Rows[0].State) {
		t.Errorf("in Bulgarian, on: %+v", bg.On.Rows)
	}

	if len(en.On.Recent) != 4 || en.On.None != "" {
		t.Fatalf("the last questions: %+v", en.On.Recent)
	}

	for i, want := range []struct{ who, what, outcome string }{
		{"Cursor 3.0", "asked for: Write a file", "Done"},
		{"A program that gave no name", "whether I am running", "Answered"},
		{"Cursor 3.0", "which programs are installed", "Answered"},
		{"claude-code 2.1.0", "which programs are installed", "Not answered"},
	} {
		got := en.On.Recent[i]
		if got.Who != want.who || got.What != want.what || got.Outcome != want.outcome || got.When == "" {
			t.Errorf("in English, question %d: %+v", i, got)
		}
	}

	for i, want := range []struct{ who, what, outcome string }{
		{"Cursor 3.0", "поиска: Запис на файл", "Изпълнено"},
		{"Програма, която не каза името си", "дали работя", "Отговорено"},
		{"Cursor 3.0", "кои програми са инсталирани", "Отговорено"},
		{"claude-code 2.1.0", "кои програми са инсталирани", "Без отговор"},
	} {
		got := bg.On.Recent[i]
		if got.Who != want.who || got.What != want.what || got.Outcome != want.outcome {
			t.Errorf("in Bulgarian, question %d: %+v", i, got)
		}
	}

	// The card: who asked, that the name is the program's own word, and
	// nothing lasting among the answers.
	if en.Card.AskedBy == nil || en.Card.AskedBy.Label != "Asked by claude-code 2.1.0" ||
		!strings.Contains(en.Card.AskedBy.Title, "the name the program gave itself") ||
		!strings.Contains(en.Card.AskedBy.Title, "nothing lasting is offered for what another program asks") ||
		!en.Card.Approve || en.Card.ForProgram != nil || en.Card.AnyProgram != nil {
		t.Errorf("in English, the card: %+v", en.Card)
	}

	if bg.Card.AskedBy == nil || bg.Card.AskedBy.Label != "Поискано от claude-code 2.1.0" ||
		!strings.Contains(bg.Card.AskedBy.Title, "името, което програмата сама си даде") ||
		!strings.Contains(bg.Card.AskedBy.Title, "не се предлага трайно разрешение") {
		t.Errorf("in Bulgarian, the card: %+v", bg.Card.AskedBy)
	}

	if en.Desk.AskedBy != nil || bg.Desk.AskedBy != nil {
		t.Errorf("a question from the desk says a program asked: %+v", en.Desk)
	}

	// The record: who, on the line and inside, and what it came to.
	if len(en.Deeds) != 4 {
		t.Fatalf("the record: %+v", en.Deeds)
	}

	// The approved tool is the newest line; the three questions follow.
	if d := en.Deeds[0]; d.Chip != "Asked by Cursor 3.0" || d.Trait != "" || d.Went != "" || d.Result != "" {
		t.Errorf("in English, a tool run for another program on the record: %+v", d)
	}

	en.Deeds, bg.Deeds = en.Deeds[1:], bg.Deeds[1:]

	if d := en.Deeds[1]; d.Chip != "Asked by Cursor 3.0" || d.Trait != "A question from another program" ||
		d.Result != "Programs I told it about: 1." {
		t.Errorf("in English, an answered question on the record: %+v", d)
	}

	if d := en.Deeds[2]; d.Chip != "Asked by claude-code 2.1.0" || !strings.HasPrefix(d.Result, "Not answered: other programs may not ask which programs are installed.") {
		t.Errorf("in English, a refused question on the record: %+v", d)
	}

	if d := en.Deeds[0]; d.Chip != "Asked by a program that gave no name" || d.Result != "" {
		t.Errorf("in English, a question from a program with no name: %+v", d)
	}

	if d := bg.Deeds[1]; d.Chip != "Поискано от Cursor 3.0" || d.Trait != "Въпрос от друга програма" || d.Result != "Програми, за които ѝ казах: 1." {
		t.Errorf("in Bulgarian, an answered question on the record: %+v", d)
	}

	if d := bg.Deeds[2]; !strings.HasPrefix(d.Result, "Без отговор: другите програми нямат право да питат кои програми са инсталирани.") {
		t.Errorf("in Bulgarian, a refused question on the record: %+v", d)
	}

	if bg.Deeds[0].Chip != "Поискано от програма, която не каза името си" {
		t.Errorf("in Bulgarian, no name: %+v", bg.Deeds[0])
	}

	// Inside the record: the name, and what a name is worth; a program
	// that gave none is said once.
	if en.Deeds[1].Said != "Cursor 3.0. This is the name the program gave itself. Nothing can check it, so it is shown and never decides anything." ||
		en.Deeds[0].Said != "A program that gave no name." || bg.Deeds[0].Said != "Програма, която не каза името си." ||
		!strings.HasPrefix(bg.Deeds[1].Said, "Cursor 3.0. Това е името, което програмата сама си даде.") {
		t.Errorf("who asked, inside the record: %q / %q / %q", en.Deeds[1].Said, en.Deeds[0].Said, bg.Deeds[1].Said)
	}

	// An answer given to another program went to it: the record does not
	// say it stayed on this machine. A refused question went nowhere.
	if en.Deeds[1].Went != "the answer went to that program" || bg.Deeds[1].Went != "отговорът отиде при тази програма" ||
		en.Deeds[2].Went != "" {
		t.Errorf("where an answer went: %q, %q, refused %q", en.Deeds[1].Went, bg.Deeds[1].Went, en.Deeds[2].Went)
	}

	// One of the assistant's own tools run because another program asked
	// keeps its own name, and says who asked.
	if en.Tool.Chip != "Asked by Cursor 3.0" || en.Tool.Trait != "" || en.Tool.Went != "" {
		t.Errorf("a tool run for another program: %+v", en.Tool)
	}

	// The change of the switch is a line of the record too, in both languages.
	if len(en.Turned) != 1 || en.Turned[0] != "What other programs may ask: Other programs may now ask which programs are installed | Changed at this computer" {
		t.Errorf("in English, the switch on the record: %q", en.Turned)
	}

	if len(bg.Turned) != 1 || bg.Turned[0] != "Какво могат да питат другите програми: Другите програми вече могат да питат кои програми са инсталирани | Променено на този компютър" {
		t.Errorf("in Bulgarian, the switch on the record: %q", bg.Turned)
	}

	// Only the switch says where it was changed.
	if en.Elsewhere != "" {
		t.Errorf("a deed that is not the switch says where it was changed: %q", en.Elsewhere)
	}

	// What was done at the desk says nothing about another program.
	if en.Mine.Chip != "" || en.Mine.Result != "" || bg.Mine.Chip != "" {
		t.Errorf("a deed from the desk: %+v", en.Mine)
	}

	// Bulgarian has no English sentence left (the names programs gave apart).
	latin := regexp.MustCompile(`[a-zA-Z]{3,}`)
	names := strings.NewReplacer("claude-code", "", "Cursor", "")

	said := []string{bg.Off.Fixed.Label, bg.Off.Fixed.State, bg.Off.Rows[0].Label, bg.Off.Rows[0].State, bg.Off.None, bg.On.Rows[0].State,
		bg.Card.AskedBy.Label, bg.Card.AskedBy.Title}

	for _, r := range bg.On.Recent {
		said = append(said, r.When, r.Who, r.What, r.Outcome)
	}

	for _, d := range bg.Deeds {
		said = append(said, d.Chip, d.Said, d.Title, d.Trait, d.Went, d.Result)
	}

	for _, s := range said {
		if latin.MatchString(names.Replace(s)) {
			t.Errorf("English left in Bulgarian: %q", s)
		}

		if strings.ContainsAny(s, "–—") {
			t.Errorf("a dash in what the page says: %q", s)
		}
	}
}

func outsidePageSays(t *testing.T, language string, off, on, cards, deeds []byte) outsidePage {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "programs.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	for name, raw := range map[string][]byte{"off.json": off, "on.json": on, "cards.json": cards, "deeds.json": deeds} {
		os.WriteFile(filepath.Join(dir, name), raw, 0o644)
	}

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
    getElementById: () => null, documentElement: {} };
globalThis.setInterval = () => 0;
globalThis.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
const json = (name) => { const b = JSON.parse(read(name)); return b && b.data !== undefined ? b.data : b; };
eval(read('i18n.js'));
eval(read('programs.js'));
const P = window.pnPrograms;
const cards = json('cards.json');
const all = json('deeds.json').deeds;
// What other programs asked; the change of the switch is a line of its own.
const deeds = all.filter((d) => d.asked_by);
const turned = all.filter((d) => d.tool === 'other_programs_switch').map((d) => window.pnToolName(d.tool) + ': ' + ((window.pnLang() === 'bg' && d.summary_bg) || d.summary) + ' | ' + P.turnedWhere(d));
const upper = (o) => o && typeof o === 'object' && !Array.isArray(o)
    ? Object.fromEntries(Object.entries(o).map(([k, v]) => [k[0].toUpperCase() + k.slice(1), upper(v)]))
    : (Array.isArray(o) ? o.map(upper) : o);
const record = (d) => {
    const a = P.askedBy(d);
    return { chip: a ? a.chip : '', said: a ? a.said : '', title: a ? a.title : '', trait: a ? a.trait : '', went: a ? a.went : '', result: P.outsideResult(d) };
};
process.stdout.write(JSON.stringify(upper({
    off: P.outside(json('off.json')),
    on: P.outside(json('on.json')),
    card: P.approvalChoices(cards[0], 'write_file'),
    desk: P.approvalChoices(cards[1], 'write_file'),
    deeds: deeds.map(record),
    turned,
    elsewhere: P.turnedWhere({ tool: 'write_file' }),
    mine: record({ tool: 'write_file', summary: 'Write notes.txt', outcome: 'done', result: 'wrote 3 lines' }),
    tool: record({ tool: 'write_file', summary: 'Write notes.txt', outcome: 'done', result: 'wrote 3 lines', changes_something: true, asked_by: 'Cursor 3.0' }),
})));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said outsidePage
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}
