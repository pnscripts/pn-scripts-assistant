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
	"pn-scripts-assistant/internal/brain/connect"
	"pn-scripts-assistant/internal/brain/otherprograms"
)

/*
 * What the page says about the things other programs hand over, the
 * actions they may run and connecting one: worked out by programs.js in
 * node on the real server's answers, in English and in Bulgarian.
 */

type handoverPage struct {
	Rows []struct {
		ID, Label, State, Means string
		Hands                   bool
	}
	Handing string
	Recent  []struct{ Who, What, Outcome string }
	Message struct {
		Approve, Reject bool
		Seen            struct{ Label, Text string }
		AskedBy         struct{ Label, Title string }
		Lasting         bool
	}
	Seen   string
	Deeds  []struct{ Tool, Name, Result, Asked, Chip string }
	Badge  string
	Action struct {
		Rows   []struct{ Name, Command, Since string }
		None   string
		Limits string
		Full   bool
	}
	NoActions string
	NotKept   map[string]string
	Connect   []struct {
		Client, Title, File, Entry, State, Button string
		Can, Connected                            bool
	}
	Stands     map[string]string
	NotWritten map[string]string
	Done       string
	DoneNew    string
}

func handoverPageSays(t *testing.T, language string, files map[string][]byte) handoverPage {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "programs.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	for name, raw := range files {
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
const upper = (o) => o && typeof o === 'object' && !Array.isArray(o)
    ? Object.fromEntries(Object.entries(o).map(([k, v]) => [k[0].toUpperCase() + k.slice(1), upper(v)]))
    : (Array.isArray(o) ? o.map(upper) : o);
const keep = (o) => o;
const outside = P.outside(json('outside.json'));
const cards = json('cards.json');
const message = cards.find((c) => c.tool === 'tell_user');
const choice = P.approvalChoices(message, 'tell_user');
const deeds = json('deeds.json').deeds.filter((d) => d.asked_by);
const codes = JSON.parse(read('codes.json'));
process.stdout.write(JSON.stringify(Object.assign(upper({
    rows: outside.rows,
    handing: outside.handing,
    recent: outside.recent,
    message: { approve: choice.approve, reject: choice.reject, seen: choice.seen, askedBy: choice.askedBy, lasting: !!(choice.forProgram || choice.anyProgram) },
    seen: P.permitDecided({ tool: 'tell_user', result: 'Seen.' }, true),
    deeds: deeds.map((d) => ({ tool: d.tool, name: window.pnToolName(d.tool), result: P.outsideResult(d), asked: P.outsideAsked(d), chip: P.askedBy(d).chip })),
    badge: window.pnWords ? '' : '',
    action: P.actions(json('actions.json')),
    noActions: P.actions({ actions: [], most: 20, seconds: 45 }).none,
    connect: P.connecting(json('connect.json')),
    done: P.connectDone({ file: '/home/me/.claude.json', backup: '/home/me/.claude.json.before' }),
    doneNew: P.connectDone({ file: '/home/me/.config/Claude/claude_desktop_config.json' }),
}), {
    // By the server's own codes: every one has words on the page.
    NotKept: Object.fromEntries(codes.notKept.map((c) => [c, P.actionNotKept(c)])),
    Stands: Object.fromEntries(codes.states.map((c) => [c, P.connecting({ programs: [{ state: c }] })[0].state])),
    NotWritten: Object.fromEntries(codes.notWritten.map((c) => [c, P.connectNotWritten(c)])),
})));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said handoverPage

	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("what the page said could not be read: %v\n%s", err, out)
	}

	return said
}

func TestThePageSaysWhatIsHandedOverInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	ts, _, b := newServer(t)

	restore := brain.HearInstead(func(string) {})
	defer restore()

	read := func(path string) []byte {
		t.Helper()

		// As the window reads: with its key, which two of these need.
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set(DeskHeader, testDeskKey)

		res, err := bare.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)

		return raw
	}

	if code, body := post(t, ts.URL+"/api/outside/actions", `{"name":"say-hello","command":"echo hello && exit 3"}`); code != 200 {
		t.Fatalf("writing the action: %d %s", code, body)
	}

	// One refused with its switch off, then each one taken.
	asked(t, ts.URL, "tell_user", `{"text":"Too early."}`)

	for _, h := range handOvers {
		if err := b.AllowOutside(h.subject, true); err != nil {
			t.Fatal(err)
		}

		if _, err := asked(t, ts.URL, h.tool, h.arguments); err != nil {
			t.Fatalf("%s: %v", h.tool, err)
		}
	}

	// A name that is not there, and the names there are.
	asked(t, ts.URL, "run_action", `{"name":"format-disk"}`)
	asked(t, ts.URL, "run_action", `{}`)

	// Claude Code has settings, with an older entry of the assistant.
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"pn-scripts-assistant":{"command":"/old/pn-scripts-assistant","args":["mcp"]}}}`), 0o600)

	codes, _ := json.Marshal(map[string][]string{
		"notKept":    {brain.ActionBadName, brain.ActionBadCommand, brain.ActionTaken, brain.ActionsFull},
		"states":     {connect.StateNoFile, connect.StateNotThere, connect.StateSame, connect.StateDifferent, connect.StateUnread},
		"notWritten": {"changed", "unread", "same"},
	})

	files := map[string][]byte{
		"outside.json": read("/api/outside"), "cards.json": read("/api/approvals"), "deeds.json": read("/api/deeds?since=all"),
		"actions.json": read("/api/outside/actions"), "connect.json": read("/api/outside/connect"), "codes.json": codes,
	}

	en := handoverPageSays(t, "en-GB", files)
	bg := handoverPageSays(t, "bg-BG", files)

	// The switches: four for what is asked, then six for what is handed
	// over under a heading of their own, each with what turning it on means.
	if len(en.Rows) != 10 || en.Handing != "What they may hand me" || bg.Handing != "Какво могат да ми предават" {
		t.Fatalf("the switches: %d, %q / %q", len(en.Rows), en.Handing, bg.Handing)
	}

	subjects := otherprograms.Subjects()

	for i, r := range en.Rows {
		if r.ID != string(subjects[i]) || r.Hands != otherprograms.HandsOver(subjects[i]) || r.Label == "" || bg.Rows[i].Label == "" || r.Label == bg.Rows[i].Label {
			t.Errorf("switch %d: %+v / %+v", i, r, bg.Rows[i])
		}

		if r.Hands && (r.Means == "" || bg.Rows[i].Means == "" || !strings.HasPrefix(r.State, "Allowed since ") || !strings.HasPrefix(bg.Rows[i].State, "Разрешено от ")) {
			t.Errorf("the switch %s does not say what it means, or that it is on: %+v / %+v", r.ID, r, bg.Rows[i])
		}
	}

	for id, means := range map[string][2]string{
		"remember":  {"only your yes on its card in this window", "едва след вашето „да“ на картата му в този прозорец"},
		"reminders": {"beginning “From another program” and the name it gave", "започва с „От друга програма“"},
		"todos":     {"never on your own lists", "никога във вашите списъци"},
		"tell":      {"until you press Seen", "докато натиснете „Видях“"},
		"speak":     {"beginning “Another program” and the name it gave", "започвам с „Друга програма“"},
		"actions":   {"It can run nothing you did not write", "не може да изпълни нищо, което не сте написали"},
	} {
		for i, r := range en.Rows {
			if r.ID == id && (!strings.Contains(r.Means, means[0]) || !strings.Contains(bg.Rows[i].Means, means[1])) {
				t.Errorf("what %s means: %q / %q", id, r.Means, bg.Rows[i].Means)
			}
		}
	}

	// The last five, newest first: the names asked for, the action that is
	// not there, the action run, the line said, the message.
	want := [][4]string{
		{"one of your actions", "Run", "едно от вашите действия", "Изпълнено"},
		{"one of your actions", "Not taken", "едно от вашите действия", "Не е прието"},
		{"one of your actions", "Not taken", "едно от вашите действия", "Не е прието"},
		{"a line to say aloud", "To be said", "изречение за казване на глас", "Ще бъде казано"},
		{"a message for you", "Shown", "съобщение за вас", "Показано"},
	}

	if len(en.Recent) != 5 {
		t.Fatalf("the last ones: %+v", en.Recent)
	}

	for i, w := range want {
		if en.Recent[i].What != w[0] || en.Recent[i].Outcome != w[1] || bg.Recent[i].What != w[2] || bg.Recent[i].Outcome != w[3] || en.Recent[i].Who != "a test" {
			t.Errorf("line %d of the last ones: %+v / %+v", i, en.Recent[i], bg.Recent[i])
		}
	}

	// A message's card: one button, Seen, who left it, nothing lasting.
	if !en.Message.Approve || en.Message.Reject || en.Message.Lasting || en.Message.Seen.Label != "Seen" || bg.Message.Seen.Label != "Видях" ||
		en.Message.AskedBy.Label != "Message from a test" || bg.Message.AskedBy.Label != "Съобщение от a test" ||
		en.Message.Seen.Text != "The build is done." || bg.Message.Seen.Text != "The build is done." ||
		!strings.Contains(en.Message.AskedBy.Title, "not a question") || !strings.Contains(bg.Message.AskedBy.Title, "а не въпрос") ||
		en.Seen != "The message is marked as seen." || bg.Seen != "Съобщението е отбелязано като видяно." {
		t.Errorf("a message's card: %+v / %+v, %q / %q", en.Message, bg.Message, en.Seen, bg.Seen)
	}

	// The record, each line in the page's language: what was handed over
	// and what it came to.
	wantDeeds := map[string][4]string{
		"remember_fact": {"To remember: Alex deploys on Fridays.", "Kept for you to accept or discard. Nothing is remembered until you accept it.",
			"За запомняне: Alex deploys on Fridays.", "Запазено, за да го приемете или отхвърлите. Нищо не се запомня, докато не го приемете."},
		"add_reminder": {"The reminder: call the bank", "Reminder set for 20", "Напомнянето: call the bank", "Напомнянето е зададено за "},
		"add_todo":     {"The line: renew the domain", "Put on the list kept for other programs.", "Редът: renew the domain", "Добавено в списъка за другите програми."},
		"say_to_user":  {"The line to say: The build is done.", "To be said aloud at the next gap.", "Изречението за казване: The build is done.", "Ще бъде казано на глас при следващата пауза."},
	}

	seen := map[string]bool{}

	for i, d := range en.Deeds {
		w, known := wantDeeds[d.Tool]
		if !known || seen[d.Tool] {
			continue
		}

		seen[d.Tool] = true

		if d.Asked != w[0] || !strings.HasPrefix(d.Result, w[1]) || bg.Deeds[i].Asked != w[2] || !strings.HasPrefix(bg.Deeds[i].Result, w[3]) ||
			d.Chip != "Asked by a test" || d.Name == d.Tool || bg.Deeds[i].Name == d.Name {
			t.Errorf("%s on the record: %+v / %+v", d.Tool, d, bg.Deeds[i])
		}
	}

	if len(seen) != len(wantDeeds) {
		t.Errorf("on the record: %v of %d kinds", seen, len(wantDeeds))
	}

	var results, resultsBG []string

	for i, d := range en.Deeds {
		results = append(results, d.Tool+": "+d.Result)
		resultsBG = append(resultsBG, d.Tool+": "+bg.Deeds[i].Result)
	}

	all, allBG := strings.Join(results, "\n"), strings.Join(resultsBG, "\n")

	for _, pair := range [][2]string{
		{"tell_user: Shown on a card in my window until it is seen.", "tell_user: Показано на карта в моя прозорец, докато бъде видяно."},
		{"tell_user: Not shown: other programs may not leave you messages in my window.", "tell_user: Не е показано: другите програми нямат право да ви оставят съобщения в моя прозорец."},
		{"run_action: Ended with exit code 3 after ", "run_action: Завърши с код на изход 3 след "},
		{"run_action: Not run: there is no action with that name.", "run_action: Не е изпълнено: няма действие с това име."},
		{"run_action: Names of your actions I told it: 1.", "run_action: Имена на ваши действия, които ѝ казах: 1."},
	} {
		if !strings.Contains(all, pair[0]) || !strings.Contains(allBG, pair[1]) {
			t.Errorf("the record does not say %q / %q:\n%s\n%s", pair[0], pair[1], all, allBG)
		}
	}

	// The day of a reminder as each language writes it.
	if !regexp.MustCompile(`зададено за \d{2}\.\d{2}\.\d{4} \d{2}:\d{2}\.`).MatchString(allBG) || !regexp.MustCompile(`set for \d{4}-\d{2}-\d{2} \d{2}:\d{2}\.`).MatchString(all) {
		t.Errorf("when the reminder is for: %s / %s", all, allBG)
	}

	// A decimal comma in Bulgarian.
	if !regexp.MustCompile(`след \d+,\d сек\.`).MatchString(allBG) || !regexp.MustCompile(`after \d+\.\d s\.`).MatchString(all) {
		t.Errorf("how long the action took: %s / %s", all, allBG)
	}

	// The actions card.
	if len(en.Action.Rows) != 1 || en.Action.Rows[0].Name != "say-hello" || en.Action.Rows[0].Command != "echo hello && exit 3" ||
		!strings.HasPrefix(en.Action.Rows[0].Since, "Written ") || !strings.HasPrefix(bg.Action.Rows[0].Since, "Написано ") ||
		en.Action.None != "" || en.Action.Full || !strings.Contains(en.Action.Limits, "has 45 seconds") || !strings.Contains(en.Action.Limits, "At most 20 actions") ||
		!strings.Contains(bg.Action.Limits, "има 45 секунди") || en.NoActions != "No actions are written yet." || bg.NoActions != "Още няма написани действия." {
		t.Errorf("the actions: %+v / %+v", en.Action, bg.Action)
	}

	// Connecting: an older entry is said to be one and offered to be
	// replaced; no file is said to be made.
	if len(en.Connect) != 2 || en.Connect[0].Title != "Claude Code" || !en.Connect[0].Can || en.Connect[0].Connected ||
		en.Connect[0].Button != "Show what would replace it" || bg.Connect[0].Button != "Покажи с какво ще бъде заменен" ||
		!strings.Contains(en.Connect[0].State, "says something else") || en.Connect[1].Title != "Claude Desktop" ||
		en.Connect[1].Button != "Show what would be written" || !strings.Contains(en.Connect[1].State, "one would be made") ||
		!strings.Contains(bg.Connect[1].State, "ще бъде създаден") || !strings.Contains(en.Connect[0].Entry, `"mcp"`) || en.Connect[0].File == "" {
		t.Errorf("connecting: %+v / %+v", en.Connect, bg.Connect)
	}

	if !strings.Contains(en.Done, "/home/me/.claude.json.before") || !strings.Contains(bg.Done, "е запазен като /home/me/.claude.json.before") ||
		!strings.Contains(en.DoneNew, "which was made for it") || !strings.Contains(bg.DoneNew, "беше създаден") {
		t.Errorf("after connecting: %q / %q / %q / %q", en.Done, bg.Done, en.DoneNew, bg.DoneNew)
	}

	// Every code of the server has its own words in both languages.
	for name, pair := range map[string][2]map[string]string{
		"an action not kept": {en.NotKept, bg.NotKept}, "how the settings stand": {en.Stands, bg.Stands}, "nothing written": {en.NotWritten, bg.NotWritten},
	} {
		said := map[string]bool{}

		for code, words := range pair[0] {
			if words == "" || pair[1][code] == "" || words == pair[1][code] || said[words] {
				t.Errorf("%s, code %q: %q / %q", name, code, words, pair[1][code])
			}

			said[words] = true
		}
	}

	if en.NotKept[brain.ActionTaken] != "There is an action with that name already." || en.Stands[connect.StateSame] != "Connected." || bg.Stands[connect.StateSame] != "Свързана е." {
		t.Errorf("the words for the codes: %+v %+v", en.NotKept, en.Stands)
	}

	// No dash anywhere, and, the names and what programs wrote apart, no
	// English on the Bulgarian page.
	latin := regexp.MustCompile(`[a-zA-Z]{3,}`)
	theirs := strings.NewReplacer("Alex deploys on Fridays.", "", "call the bank", "", "renew the domain", "", "The build is done.", "", "Too early.", "",
		"a test", "", "say-hello", "", "echo hello && exit 3", "", "Claude Code", "", "Claude Desktop", "", "mcpServers", "", "JSON", "",
		"/home/me/.claude.json.before", "", "/home/me/.claude.json", "", "/home/me/.config/Claude/claude_desktop_config.json", "")

	var said []string

	said = append(said, bg.Handing, bg.Seen, bg.Message.Seen.Label, bg.Message.AskedBy.Label, bg.Message.AskedBy.Title, bg.NoActions, bg.Action.Limits, bg.Done, bg.DoneNew)

	for _, r := range bg.Rows {
		said = append(said, r.Label, r.State, r.Means)
	}

	for _, r := range bg.Recent {
		said = append(said, r.What, r.Outcome)
	}

	for _, d := range bg.Deeds {
		said = append(said, d.Name, d.Result, d.Asked, d.Chip)
	}

	for _, r := range bg.Action.Rows {
		said = append(said, r.Since)
	}

	for _, c := range bg.Connect {
		said = append(said, c.State, c.Button)
	}

	for _, m := range []map[string]string{bg.NotKept, bg.Stands, bg.NotWritten} {
		for _, words := range m {
			said = append(said, words)
		}
	}

	for _, s := range said {
		if latin.MatchString(theirs.Replace(s)) {
			t.Errorf("English left in Bulgarian: %q", s)
		}
	}

	for _, s := range append(said, en.Handing, en.Seen, en.Action.Limits, en.Done, en.DoneNew, all) {
		if strings.ContainsAny(s, "–—") {
			t.Errorf("a dash in what the page says: %q", s)
		}
	}
}

// The page names every switch and every tool the server has, and the card
// for what waits to be remembered says where a thing a program told came
// from.
func TestThePageKnowsEverySwitchAndToolOfTheDoor(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "programs.js"))
	words := readAsset(t, filepath.Join("js", "i18n.js"))
	console := readAsset(t, filepath.Join("js", "console.js"))

	for _, s := range otherprograms.Subjects() {
		if !regexp.MustCompile(`\b` + string(s) + `: 'out\.`).MatchString(js) {
			t.Errorf("programs.js has no words for the switch %s", s)
		}

		if otherprograms.HandsOver(s) && !regexp.MustCompile(`\b`+string(s)+`: true`).MatchString(js) {
			t.Errorf("programs.js does not put the switch %s under what is handed over", s)
		}
	}

	for _, tool := range otherprograms.Tools() {
		if !regexp.MustCompile(`\b` + tool + `: 'out\.q\.`).MatchString(js) {
			t.Errorf("programs.js has no words for %s", tool)
		}

		// The record names what was handed over by its own words.
		if subject, _ := otherprograms.SubjectOf(tool); otherprograms.HandsOver(subject) && !regexp.MustCompile(`\b`+tool+`: \['`).MatchString(words) {
			t.Errorf("i18n.js has no name for the tool %s", tool)
		}
	}

	if !strings.Contains(js, "const MESSAGE = '"+otherprograms.TellTool+"'") {
		t.Error("programs.js does not know the tool a message comes through")
	}

	if !strings.Contains(console, otherprograms.Told+": 'cc.conf.told'") || !strings.Contains(words, "'cc.conf.told': ['from another program', 'от друга програма']") {
		t.Error("the card of a thing to remember does not say it came from another program")
	}

	for _, tool := range []string{brain.ActionsTool, brain.ConnectTool} {
		if !regexp.MustCompile(`\b` + tool + `: \['`).MatchString(words) {
			t.Errorf("i18n.js has no name for %s on the record", tool)
		}
	}
}
