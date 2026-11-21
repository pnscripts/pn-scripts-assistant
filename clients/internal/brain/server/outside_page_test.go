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
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/progress"
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
			Means   string
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
		Chip, Said, Title, Trait, Went, Result, Asked, Decided string
	}

	// Every waiting question as its card says it, and what the page says
	// after a permission card was answered.
	Cards []struct {
		Tool    string
		Approve bool
		Lasting bool
		AskedBy *struct{ Label, Title string }
	}
	Permit    struct{ Yes, No, Gone, Unrecorded, Full, Other, Again string }
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
		len(en.Off.Rows) != 10 || en.Off.Rows[0].ID != "programs" || en.Off.Rows[0].Allowed || en.Off.Rows[0].Means != "" ||
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
    return { chip: a ? a.chip : '', said: a ? a.said : '', title: a ? a.title : '', trait: a ? a.trait : '', went: a ? a.went : '', result: P.outsideResult(d), asked: P.outsideAsked(d), decided: a ? (a.decided || '') : '' };
};
process.stdout.write(JSON.stringify(upper({
    off: P.outside(json('off.json')),
    on: P.outside(json('on.json')),
    card: P.approvalChoices(cards[0], 'write_file'),
    desk: P.approvalChoices(cards[1], 'write_file'),
    deeds: deeds.map(record),
    cards: cards.filter((c) => c && c.tool).map((c) => {
        const o = P.approvalChoices(c, c.tool);
        return { tool: window.pnToolName(c.tool), approve: o.approve, lasting: !!(o.forProgram || o.anyProgram), askedBy: o.askedBy };
    }),
    permit: {
        yes: P.permitDecided({ tool: 'permission_prompt', result: 'Allowed on its card: the program was told it may.' }, true),
        no: P.permitDecided({ tool: 'permission_prompt', result: 'Not allowed on its card: the program was told it may not.' }, false),
        gone: P.permitDecided({ tool: 'permission_prompt', result: 'Not answered: the program stopped waiting before anybody answered.' }, true, 'nobody waits'),
        other: P.permitDecided({ tool: 'write_file' }, true),
        // A second press on a card already allowed is refused by the server; the page still says what the first did.
        again: P.permitDecided({ tool: 'permission_prompt', result: 'Allowed on its card: the program was told it may.' }, false, 'that action was already done and cannot be decided again'),
        unrecorded: P.permitDecided({ tool: 'permission_prompt', result: 'Not allowed: the answer could not be written on the record, so the program was told no.' }, true, 'not recorded'),
        full: P.outsideResult({ tool: 'permission_prompt', asked_by: 'x', outcome: 'refused', result: 'Not answered: 3 permission questions were waiting already, and no more wait at once.' }),
    },
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

/*
 * The two switches of 1.2.0 step 4 and what they put on the record, as
 * the page says them in both languages: what allowing each one means
 * (read before it is ticked), a question and its answer, a search and
 * how much it found, each refusal in its own words, and the one line for
 * a program that asked too much.
 */
func TestThePageSaysAskingAndSearchingInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, b := newServer(t)

	model := &onThisComputer{said: "Maria, on weekdays."}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	remembered(t, b, model, "accountant", "Alex's accountant is Maria")

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

	question := `{"tool":"ask_assistant","arguments":{"question":"Who is the accountant?"},"client":{"name":"Cursor","version":"3.0"}}`
	search := `{"tool":"search_memory","arguments":{"query":"accountant"},"client":{"name":"Cursor","version":"3.0"}}`

	// Both refused, both allowed, both answered; then a model that fails.
	post(t, ts.URL+"/api/outside/ask", question)
	post(t, ts.URL+"/api/outside/ask", search)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"ask","allowed":true}`)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"memory","allowed":true}`)
	post(t, ts.URL+"/api/outside/ask", question)
	post(t, ts.URL+"/api/outside/ask", search)

	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, &onThisComputer{said: " "})

	post(t, ts.URL+"/api/outside/ask", question)

	// And the one line for a program that asked too much.
	tooMany, _ := otherprograms.TooMany()
	if err := b.AskedFromOutside("assistant_status", "loop 1", "", "refused", tooMany); err != nil {
		t.Fatal(err)
	}

	on := read("/api/outside")
	deeds := read("/api/deeds?since=all")

	en := outsidePageSays(t, "en-GB", off, on, []byte(`[{},{}]`), deeds)
	bg := outsidePageSays(t, "bg-BG", off, on, []byte(`[{},{}]`), deeds)

	// The switches, in the page's order, each off, the two new ones with
	// what allowing them means.
	if len(en.Off.Rows) != 10 || en.Off.Rows[1].ID != "ask" || en.Off.Rows[2].ID != "memory" ||
		en.Off.Rows[1].Label != "Questions to me, answered in words" || en.Off.Rows[2].Label != "Searching what I remember about you" ||
		en.Off.Rows[1].Allowed || en.Off.Rows[2].Allowed ||
		!strings.Contains(en.Off.Rows[1].Means, "I do nothing for the program that asks") ||
		!strings.Contains(en.Off.Rows[1].Means, "nothing about you unless the next switch is on too") ||
		!strings.Contains(en.Off.Rows[2].Means, "goes to that program and to the model it uses, which may be on the internet") ||
		!strings.Contains(en.Off.Rows[2].Means, "never the whole memory") {
		t.Errorf("in English, off: %+v", en.Off.Rows)
	}

	if bg.Off.Rows[1].Label != "Въпроси към мен, на които отговарям с думи" || bg.Off.Rows[2].Label != "Търсене в това, което помня за вас" ||
		!strings.Contains(bg.Off.Rows[1].Means, "Не върша нищо за програмата, която пита") ||
		!strings.Contains(bg.Off.Rows[2].Means, "може да е в интернет") {
		t.Errorf("in Bulgarian, off: %+v", bg.Off.Rows)
	}

	if !en.On.Rows[1].Allowed || !en.On.Rows[2].Allowed || en.On.Rows[0].Allowed || !strings.HasPrefix(en.On.Rows[2].State, "Allowed since ") {
		t.Errorf("in English, on: %+v", en.On.Rows)
	}

	// The last five questions, newest first.
	for i, want := range []struct{ what, outcome, whatBG, outcomeBG string }{
		{"whether I am running", "Not answered", "дали работя", "Без отговор"},
		{"a question to me", "Not answered", "въпрос към мен", "Без отговор"},
		{"what I remember", "Answered", "какво помня", "Отговорено"},
		{"a question to me", "Answered", "въпрос към мен", "Отговорено"},
		{"what I remember", "Not answered", "какво помня", "Без отговор"},
	} {
		if got := en.On.Recent[i]; got.What != want.what || got.Outcome != want.outcome {
			t.Errorf("in English, question %d: %+v", i, got)
		}

		if got := bg.On.Recent[i]; got.What != want.whatBG || got.Outcome != want.outcomeBG {
			t.Errorf("in Bulgarian, question %d: %+v", i, got)
		}
	}

	// The record, newest first: too many, no answer, a search, an answer,
	// a search refused, a question refused.
	if len(en.Deeds) != 6 {
		t.Fatalf("the record: %+v", en.Deeds)
	}

	for i, want := range []struct{ asked, result, askedBG, resultBG string }{
		{"", "Not answered: other programs asked more than 30 questions in a minute. The ones over that are refused and not listed one by one.",
			"", "Без отговор: другите програми зададоха повече от 30 въпроса за една минута. Въпросите над тази граница получават отказ и не се изброяват един по един."},
		{"The question: Who is the accountant?", "No answer: the model on this computer did not give one.",
			"Въпросът: Who is the accountant?", "Без отговор: моделът на този компютър не даде такъв."},
		{"Searched for: accountant", "Things I remember that I told it: 1.", "Търсено: accountant", "Неща от паметта ми, които ѝ казах: 1."},
		{"The question: Who is the accountant?", "My answer: Maria, on weekdays.", "Въпросът: Who is the accountant?", "Моят отговор: Maria, on weekdays."},
		{"", "Not answered: other programs may not search what I remember. You can allow it on the Programs page, under “What other programs may ask”.",
			"", "Без отговор: другите програми нямат право да търсят в това, което помня. Можете да го разрешите на страницата „Програми“, в „Какво могат да питат другите програми“."},
		{"", "Not answered: other programs may not ask me questions. You can allow it on the Programs page, under “What other programs may ask”.",
			"", "Без отговор: другите програми нямат право да ми задават въпроси. Можете да го разрешите на страницата „Програми“, в „Какво могат да питат другите програми“."},
	} {
		if got := en.Deeds[i]; got.Asked != want.asked || got.Result != want.result || got.Trait != "A question from another program" {
			t.Errorf("in English, record %d: %+v", i, got)
		}

		if got := bg.Deeds[i]; got.Asked != want.askedBG || got.Result != want.resultBG {
			t.Errorf("in Bulgarian, record %d: %+v", i, got)
		}
	}

	// What was found went to that program. An answer to a question is
	// made first and taken by the program after, which the record cannot
	// see: it says the answer was made for it. A refusal and a question
	// the model did not answer went nowhere.
	for i, went := range []string{"", "", "the answer went to that program", "the answer was made for that program to take", "", ""} {
		if en.Deeds[i].Went != went {
			t.Errorf("record %d, where the answer went: %q, want %q", i, en.Deeds[i].Went, went)
		}
	}

	if bg.Deeds[3].Went != "отговорът беше приготвен, за да го вземе тази програма" {
		t.Errorf("in Bulgarian, an answer made for a program: %q", bg.Deeds[3].Went)
	}

	// The page's words for the number are the record's number.
	if !strings.Contains(tooMany, "more than 30 questions in a minute") || otherprograms.MostAMinute != 30 {
		t.Errorf("the page says 30 a minute, the door answers %d: %q", otherprograms.MostAMinute, tooMany)
	}

	// No dash and, the question and the names apart, no English on the
	// Bulgarian page.
	latin := regexp.MustCompile(`[a-zA-Z]{3,}`)
	theirs := strings.NewReplacer("Who is the accountant?", "", "accountant", "", "Maria, on weekdays.", "", "Cursor", "", "loop", "", "Claude Code", "")

	var said []string
	for _, r := range bg.On.Rows {
		said = append(said, r.Label, r.State, r.Means)
	}

	for _, r := range bg.On.Recent {
		said = append(said, r.Who, r.What, r.Outcome)
	}

	for _, d := range bg.Deeds {
		said = append(said, d.Chip, d.Trait, d.Went, d.Result, d.Asked)
	}

	for _, s := range said {
		if latin.MatchString(theirs.Replace(s)) {
			t.Errorf("English left in Bulgarian: %q", s)
		}

		if strings.ContainsAny(s, "–—") {
			t.Errorf("a dash in what the page says: %q", s)
		}
	}

	_ = db
}

// A question given up because the person asked something is said so on
// the record in both languages, with the words the brain writes for it.
func TestThePageSaysAQuestionGaveWayToThePerson(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, b := newServer(t)

	model := &onThisComputer{said: "never said", hold: make(chan struct{})}
	b.Router = llm.NewRouter(llm.ModeOpen, llm.Local, model)

	if err := b.AllowOutside(otherprograms.Ask, true); err != nil {
		t.Fatal(err)
	}

	ticket, err := b.AskForOutside("Cursor 3.0", "A slow one?")
	if err != nil {
		t.Fatal(err)
	}

	progress.StartedATurn()

	b.OutsideAnswer(t.Context(), ticket, 5*time.Second)

	progress.FinishedATurn()

	// And one taken back by its switch while the model had it.
	if _, err := b.AskForOutside("Cursor 3.0", "Another slow one?"); err != nil {
		t.Fatal(err)
	}

	if err := b.AllowOutside(otherprograms.Ask, false); err != nil {
		t.Fatal(err)
	}

	for i := 0; len(theirs(t, db)) != 2; i++ {
		if i > 300 {
			t.Fatalf("the record: %+v", theirs(t, db))
		}

		time.Sleep(10 * time.Millisecond)
	}

	res, err := http.Get(ts.URL + "/api/deeds?since=all")
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()
	deeds, _ := io.ReadAll(res.Body)

	state := []byte(`{"subjects":[],"recent":[]}`)

	en := outsidePageSays(t, "en-GB", state, state, []byte(`[{},{}]`), deeds)
	bg := outsidePageSays(t, "bg-BG", state, state, []byte(`[{},{}]`), deeds)

	if len(en.Deeds) != 2 || en.Deeds[1].Result != "No answer: you asked me something, and that comes first." ||
		en.Deeds[1].Asked != "The question: A slow one?" || en.Deeds[1].Went != "" ||
		en.Deeds[0].Result != "No answer: this was switched off while I was answering." || en.Deeds[0].Went != "" {
		t.Errorf("in English: %+v", en.Deeds)
	}

	if len(bg.Deeds) != 2 || bg.Deeds[1].Result != "Без отговор: вие ме попитахте нещо, а това е с предимство." ||
		bg.Deeds[0].Result != "Без отговор: това беше изключено, докато отговарях." {
		t.Errorf("in Bulgarian: %+v", bg.Deeds)
	}
}

/*
 * Another program's permission questions (1.2.0 step 5) as the page says
 * them in both languages: the switch and what turning it on means, the
 * card of a question that waits, what the page says once it is answered,
 * the last questions, and each ending on the record with who decided.
 */
func TestThePageSaysAPermissionQuestionInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, _, _ := newServer(t)
	permitWaits(t, 700*time.Millisecond)

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

	// Refused while off; then allowed, refused and left unanswered.
	post(t, ts.URL+"/api/outside/ask", permitBash)
	post(t, ts.URL+"/api/outside/allow", `{"subject":"permissions","allowed":true}`)

	var cards []byte

	for _, decision := range []string{"approve", "deny", ""} {
		asked := askTheDoor(t, ts.URL, permitBash)
		card := permitCards(t, ts.URL, 1)[0]

		if decision == "" {
			cards = read("/api/approvals")
		} else {
			post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/"+decision, `{}`)
		}

		doorAnswered(t, asked)
	}

	post(t, ts.URL+"/api/outside/ask",
		`{"tool":"permission_prompt","arguments":{"tool_name":"AskUserQuestion","input":{}},"client":{"name":"claude-code","version":"2.1.303"}}`)

	on := read("/api/outside")
	deeds := read("/api/deeds?since=all")

	en := outsidePageSays(t, "en-GB", off, on, cards, deeds)
	bg := outsidePageSays(t, "bg-BG", off, on, cards, deeds)

	// The switch: the last of four, off, with what turning it on means.
	if len(en.Off.Rows) != 10 || en.Off.Rows[3].ID != "permissions" || en.Off.Rows[3].Allowed ||
		en.Off.Rows[3].Label != "Their own permission questions, put to you here" ||
		!strings.Contains(en.Off.Rows[3].Means, "I answer none of them myself") ||
		!strings.Contains(en.Off.Rows[3].Means, "a question nobody answers in ten minutes is a no") ||
		!strings.Contains(en.Off.Rows[3].Means, "at most three wait at once") ||
		!strings.Contains(en.Off.Rows[3].Means, "A yes on the card lets that program do it, once") ||
		!strings.Contains(en.Off.Rows[3].Means, "not what it will write into a file") || !en.On.Rows[3].Allowed {
		t.Errorf("the switch in English: %+v", en.Off.Rows)
	}

	if bg.Off.Rows[3].Label != "Техните въпроси за разрешение, зададени на вас тук" ||
		!strings.Contains(bg.Off.Rows[3].Means, "Аз не отговарям на нито един от тях") ||
		!strings.Contains(bg.Off.Rows[3].Means, "въпрос без отговор до десет минути означава „не“") ||
		!strings.Contains(bg.Off.Rows[3].Means, "едновременно чакат най-много три") ||
		!strings.Contains(bg.Off.Rows[3].Means, "Отговор „да“ на картата позволява на тази програма да го направи, еднократно") {
		t.Errorf("the switch in Bulgarian: %+v", bg.Off.Rows)
	}

	// What the switch says of itself is what the code does.
	if otherprograms.MostPermissionsWaiting != 3 || permitWaitDefault != 10*time.Minute {
		t.Errorf("the page says three at once and ten minutes; the code: %d and %s", otherprograms.MostPermissionsWaiting, permitWaitDefault)
	}

	// The card: named as what it is, who asked, yes or no and nothing
	// lasting.
	if len(en.Cards) != 1 || en.Cards[0].Tool != "Permission question of another program" || !en.Cards[0].Approve || en.Cards[0].Lasting ||
		en.Cards[0].AskedBy == nil || en.Cards[0].AskedBy.Label != "Asked by claude-code 2.1.303" ||
		!strings.Contains(en.Cards[0].AskedBy.Title, "a yes lets it do this once, and I do nothing myself") {
		t.Errorf("the card in English: %+v", en.Cards)
	}

	if len(bg.Cards) != 1 || bg.Cards[0].Tool != "Въпрос за разрешение от друга програма" || bg.Cards[0].Lasting ||
		bg.Cards[0].AskedBy == nil || bg.Cards[0].AskedBy.Label != "Поискано от claude-code 2.1.303" ||
		!strings.Contains(bg.Cards[0].AskedBy.Title, "с „да“ тя може да го направи еднократно, а аз не върша нищо") {
		t.Errorf("the card in Bulgarian: %+v", bg.Cards)
	}

	// After the card is answered.
	if en.Permit.Yes != "Allowed. The program was told it may, this once." || en.Permit.No != "Not allowed. The program was told it may not." ||
		!strings.HasPrefix(en.Permit.Gone, "The program that asked is no longer waiting") || en.Permit.Other != "" {
		t.Errorf("after the answer, in English: %+v", en.Permit)
	}

	if en.Permit.Again != en.Permit.Yes || bg.Permit.Again != bg.Permit.Yes {
		t.Errorf("a second press on a card already allowed: %q / %q", en.Permit.Again, bg.Permit.Again)
	}

	// A yes that could not be recorded is not said as "nobody waits"; and
	// one more than may wait has its own line on the record.
	if !strings.HasPrefix(en.Permit.Unrecorded, "I could not write the answer on the record, so the program was told no") ||
		!strings.HasPrefix(bg.Permit.Unrecorded, "Не успях да запиша отговора") ||
		en.Permit.Full != "Not answered: three permission questions were waiting already, and no more wait at once." ||
		bg.Permit.Full != "Без отговор: вече чакаха три въпроса за разрешение, а повече не чакат едновременно." {
		t.Errorf("unrecorded and full: %+v / %+v", en.Permit, bg.Permit)
	}

	if bg.Permit.Yes != "Разрешено. Програмата получи отговор, че може, само този път." ||
		bg.Permit.No != "Не е разрешено. Програмата получи отговор, че не може." ||
		!strings.HasPrefix(bg.Permit.Gone, "Програмата, която попита, вече не чака") {
		t.Errorf("after the answer, in Bulgarian: %+v", bg.Permit)
	}

	// The last questions (five), newest first: not passed on, unanswered,
	// refused, allowed, refused with the switch off.
	if len(en.On.Recent) != 5 {
		t.Fatalf("the last questions: %+v", en.On.Recent)
	}

	for i, outcome := range []string{"Not allowed", "Not allowed", "Not allowed", "Allowed", "Not allowed"} {
		if r := en.On.Recent[i]; r.Outcome != outcome || r.What != "whether it may do something" || r.Who != "claude-code 2.1.303" {
			t.Errorf("last question %d in English: %+v", i, r)
		}
	}

	if r := bg.On.Recent[3]; r.Outcome != "Разрешено" || r.What != "дали може да направи нещо" {
		t.Errorf("the allowed one in Bulgarian: %+v", r)
	}

	// The record, newest first, each ending in its own words and with
	// who decided.
	want := []struct{ en, bg, decidedEN, decidedBG string }{
		{"Not answered: I pass on a program’s permission questions only, not the questions it wants to ask you itself.",
			"Без отговор: предавам само въпросите за разрешение на една програма, не и въпросите, които тя иска да ви зададе сама.", "", ""},
		{"Not allowed: nobody answered in time, and a question without an answer is a no.",
			"Не е разрешено: никой не отговори навреме, а въпрос без отговор означава „не“.",
			"Decided by nobody: not allowed", "Никой не реши: не е разрешено"},
		{"Not allowed on its card: the program was told it may not.", "Не е разрешено на картата: програмата получи отговор, че не може.",
			"Decided on its card, at this computer", "Решено на картата, на този компютър"},
		{"Allowed on its card: the program was told it may.", "Разрешено на картата: програмата получи отговор, че може.",
			"Decided on its card, at this computer", "Решено на картата, на този компютър"},
		{"Not answered: other programs may not put their permission questions to you. You can allow it on the Programs page, under “What other programs may ask”.",
			"Без отговор: другите програми нямат право да ви задават своите въпроси за разрешение. Можете да го разрешите на страницата „Програми“, в „Какво могат да питат другите програми“.", "", ""},
	}

	if len(en.Deeds) != len(want) || len(bg.Deeds) != len(want) {
		t.Fatalf("the record: %d lines in English, %d in Bulgarian", len(en.Deeds), len(bg.Deeds))
	}

	for i, w := range want {
		if en.Deeds[i].Result != w.en || en.Deeds[i].Decided != w.decidedEN || en.Deeds[i].Trait != "A question from another program" {
			t.Errorf("line %d in English: %+v", i, en.Deeds[i])
		}

		if bg.Deeds[i].Result != w.bg || bg.Deeds[i].Decided != w.decidedBG || bg.Deeds[i].Trait != "Въпрос от друга програма" {
			t.Errorf("line %d in Bulgarian: %+v", i, bg.Deeds[i])
		}
	}

	// What was asked, whole, under the page's word for it.
	if en.Deeds[3].Asked != `What I was given: {"tool_name":"Bash","input":{"command":"rm -rf node_modules"}}` ||
		bg.Deeds[3].Asked != `Какво получих: {"tool_name":"Bash","input":{"command":"rm -rf node_modules"}}` {
		t.Errorf("what was asked: %q / %q", en.Deeds[3].Asked, bg.Deeds[3].Asked)
	}

	if en.Deeds[3].Went != "the answer went to that program" {
		t.Errorf("where the yes went: %q", en.Deeds[3].Went)
	}
}
