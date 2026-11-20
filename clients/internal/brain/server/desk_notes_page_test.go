package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/pair"
)

/*
 * Notes 2, 3 and 6 of the review of the window's key (20.11.2026), as the
 * page says them: its scripts run in node, on the real server's answers.
 */

/*
 * (2) On a device elsewhere, a card another program asked for has no
 * buttons: who asked, and a line saying where it is answered, in the page's
 * language. A press from a page drawn before the card was marked is told
 * the same in words the page has, not in the server's English.
 */
func TestADeviceElsewhereIsShownWhereAnotherProgramsCardIsAnswered(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	s := plain(t, config.ReachNetwork)
	s.Handler()

	db := s.brain.DB

	target := filepath.Join(t.TempDir(), "asked.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "written"})

	asked, err := db.RecordInvocationAskedBy(0, "write_file", string(args), "Create "+target, "", "mutating", "Cursor 3.0")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.RecordInvocation(0, "write_file", string(args), "Create "+target+" (mine)", "mutating"); err != nil {
		t.Fatal(err)
	}

	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "phone", "192.168.1.9")

	carry := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }

	cards := ask(s, "GET", "/api/approvals", "192.168.1.9:4000", carry)
	refused := ask(s, "POST", "/api/approvals/"+itoa(asked)+"/approve", "192.168.1.9:4000", carry)
	window := ask(s, "GET", "/api/approvals", "127.0.0.1:5000", nil)

	if cards.Code != 200 || refused.Code != http.StatusForbidden || window.Code != 200 {
		t.Fatalf("the server's answers: %d %d %d", cards.Code, refused.Code, window.Code)
	}

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "desk.js", "programs.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "cards.json"), cards.Body.Bytes(), 0o644)
	os.WriteFile(filepath.Join(dir, "window.json"), window.Body.Bytes(), 0o644)
	os.WriteFile(filepath.Join(dir, "refused.json"), refused.Body.Bytes(), 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];
Object.defineProperty(globalThis, 'navigator', { value: { language: process.argv[3] }, configurable: true });
globalThis.window = globalThis;
window.dispatchEvent = () => true;
window.addEventListener = () => {};
window.location = { href: 'http://192.168.1.2:8896/' };
window.sessionStorage = { getItem: () => null };
globalThis.CustomEvent = function () {};
globalThis.document = { readyState: 'complete', querySelectorAll: () => [], querySelector: () => null,
    getElementById: () => null, documentElement: {} };
globalThis.setInterval = () => 0;
window.fetch = () => Promise.reject(new Error('no server here'));
const read = (name) => fs.readFileSync(path.join(dir, name), 'utf8');
eval(read('i18n.js'));
eval(read('desk.js'));
eval(read('programs.js'));
const P = window.pnPrograms;
const said = (list) => JSON.parse(read(list)).map((c) => {
    const o = P.approvalChoices(c, c.tool);
    return { mine: !c.asked_by, approve: o.approve, reject: o.reject, lasting: !!(o.forProgram || o.anyProgram), note: o.note, asker: o.askedBy ? o.askedBy.label : '' };
});
const refused = JSON.parse(read('refused.json'));
process.stdout.write(JSON.stringify({
    away: said('cards.json'),
    here: said('window.json'),
    atTheComputer: window.pnDesk.atTheComputer(403, refused),
    notTheWindow: window.pnDesk.refused(403, refused),
    words: window.pnT('desk.atTheComputer'),
}));
`), 0o644)

	type card struct {
		Mine, Approve, Reject, Lasting bool
		Note, Asker                    string
	}

	var english string

	for _, language := range []string{"en-GB", "bg-BG"} {
		out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
		if err != nil {
			t.Fatalf("the page's scripts failed: %v\n%s", err, out)
		}

		var got struct {
			Away, Here                  []card
			AtTheComputer, NotTheWindow bool
			Words                       string
		}

		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("the page's scripts said %s: %v", out, err)
		}

		if language == "en-GB" {
			english = got.Words
		}

		if got.Words == "" || strings.Contains(got.Words, "desk.") || strings.ContainsAny(got.Words, "–—") {
			t.Errorf("%s: the line where the buttons would be: %q", language, got.Words)
		}

		if language == "bg-BG" && (got.Words == english || regexp.MustCompile(`[a-zA-Z]{3,}`).MatchString(got.Words)) {
			t.Errorf("in Bulgarian the line is not Bulgarian: %q", got.Words)
		}

		if len(got.Away) != 2 || len(got.Here) != 2 {
			t.Fatalf("%s: two cards each way, got %s", language, out)
		}

		for _, c := range got.Away {
			switch {
			case c.Mine && (!c.Approve || !c.Reject || c.Note != ""):
				t.Errorf("%s: the assistant's own card lost its buttons on a paired device: %+v", language, c)
			case !c.Mine && (c.Approve || c.Reject || c.Lasting || c.Note != got.Words || !strings.Contains(c.Asker, "Cursor 3.0")):
				t.Errorf("%s: another program's card on a device elsewhere: %+v", language, c)
			}
		}

		for _, c := range got.Here {
			if !c.Approve || !c.Reject || c.Note != "" {
				t.Errorf("%s: in the window a card lost its buttons: %+v", language, c)
			}
		}

		if !got.AtTheComputer || got.NotTheWindow {
			t.Errorf("%s: the page cannot tell the refusal of a device elsewhere (%v) from the window's (%v)",
				language, got.AtTheComputer, got.NotTheWindow)
		}
	}

	for _, script := range []string{"console.js", "room.js"} {
		src := readAsset(t, filepath.Join("js", script))

		if !strings.Contains(src, "'desk.atTheComputer'") || !strings.Contains(src, "pnDesk.atTheComputer(") {
			t.Errorf("%s does not say where another program's card is answered when a device presses it", script)
		}
	}

	if src := readAsset(t, filepath.Join("js", "room.js")); !strings.Contains(src, "item.at_the_computer_only") {
		t.Error("room.js shows the buttons on a card a device elsewhere cannot answer")
	}

	if src := readAsset(t, filepath.Join("js", "console.js")); !strings.Contains(src, "offer.reject !== false") {
		t.Error("console.js shows Reject on a card a device elsewhere cannot answer")
	}
}

/*
 * (3) The devices page says its own refusal: pairing and what may reach the
 * assistant, not "answers on cards and these switches", in both languages.
 */
func TestTheDevicesPageSaysItsOwnRefusal(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	en, bg := strings2(t, js, "desk.notHereDevices")

	if !strings.Contains(en, "Pairing") || !strings.Contains(en, "own window") || strings.Contains(strings.ToLower(en), "card") {
		t.Errorf("the devices page's refusal in English: %q", en)
	}

	if !strings.Contains(bg, "Сдвояването") || !strings.Contains(bg, "прозорец") || strings.Contains(bg, "карт") ||
		regexp.MustCompile(`\b(?:[a-zA-Z]{3,})\b`).MatchString(strings.ReplaceAll(bg, "PN Scripts Assistant", "")) {
		t.Errorf("the devices page's refusal in Bulgarian: %q", bg)
	}

	for _, said := range []string{en, bg} {
		if strings.ContainsAny(said, "–—") {
			t.Errorf("a dash in the refusal: %q", said)
		}
	}

	devices := readAsset(t, filepath.Join("js", "devices.js"))

	if !strings.Contains(devices, "'desk.notHereDevices'") || strings.Contains(devices, "'desk.notHere'") {
		t.Error("devices.js does not say its own refusal, or still says the cards' one")
	}
}

/*
 * (6) The count beside "Waiting for you" follows the cards when they are
 * drawn. It was counted only when the status was polled, every five
 * seconds, so after the last card was answered it said 1 above an empty
 * list (seen once in a walk).
 */
func TestTheWaitingCountFollowsTheCardsAsTheyAreDrawn(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "dashboard.js"), []byte(readAsset(t, filepath.Join("js", "dashboard.js"))), 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const dir = process.argv[2];
function node(id) {
    return { id, hidden: false, textContent: '', children: [], style: {}, dataset: {},
        classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
        append() {}, appendChild(c) { return c; }, prepend() {}, remove() {}, setAttribute() {}, getAttribute: () => null,
        addEventListener() {}, querySelector: () => null, querySelectorAll: () => [], getContext: () => null,
        getBoundingClientRect: () => ({ width: 0, height: 0 }) };
}
const nodes = {};
globalThis.window = globalThis;
window.addEventListener = () => {};
window.dispatchEvent = () => true;
globalThis.requestAnimationFrame = () => 0;
globalThis.setInterval = () => 0;
globalThis.setTimeout = () => 0;
// The status poll never answers: whatever the count says, the cards said.
globalThis.fetch = () => new Promise(() => {});
globalThis.document = { readyState: 'complete', addEventListener() {}, getElementById: (id) => (nodes[id] = nodes[id] || node(id)),
    querySelector: () => null, querySelectorAll: () => [], createElement: (t) => node(t), documentElement: node('html'), body: node('body') };
eval(fs.readFileSync(dir + '/dashboard.js', 'utf8'));
const count = () => [String(nodes['waiting-count'].textContent), nodes['waiting-count'].hidden, String(nodes['bell-count'].textContent), nodes['bell-count'].hidden];
const out = {};
if (typeof window.pnCountWaiting !== 'function') { process.stdout.write(JSON.stringify({ missing: true })); process.exit(0); }
nodes.approvals = node('approvals');
nodes.lessons = node('lessons');
nodes.approvals.children = [{}];
window.pnCountWaiting(0);
out.one = count();
// The last card answered: the list is drawn again, empty.
nodes.approvals.children = [];
window.pnCountWaiting();
out.answered = count();
out.emptySaid = nodes['waiting-empty'].hidden === false;
// Two lessons waiting, one decided: the status console.js reads after it.
nodes.lessons.children = [{}, {}];
window.pnCountWaiting(2);
out.lessons = count();
nodes.lessons.children = [{}];
window.pnCountWaiting(1);
out.lessonDecided = count();
process.stdout.write(JSON.stringify(out));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's script failed: %v\n%s", err, out)
	}

	var got struct {
		Missing                               bool
		One, Answered, Lessons, LessonDecided []any
		EmptySaid                             bool
	}

	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("the page's script said %s: %v", out, err)
	}

	if got.Missing {
		t.Fatal("the count is drawn only by the status poll: nothing counts it again when the cards are drawn")
	}

	want := func(name string, c []any, n string, hidden bool) {
		if len(c) != 4 || c[0] != n || c[1] != hidden || c[2] != n || c[3] != hidden {
			t.Errorf("%s: the count and the bell say %v, want %s (hidden %v)", name, c, n, hidden)
		}
	}

	want("one card", got.One, "1", false)
	want("after the last card was answered", got.Answered, "0", true)
	want("two lessons", got.Lessons, "2", false)
	want("a lesson decided", got.LessonDecided, "1", false)

	if !got.EmptySaid {
		t.Error("\"nothing needs a decision\" is not said under an empty list")
	}

	console := readAsset(t, filepath.Join("js", "console.js"))

	sync := console[strings.Index(console, "function syncWaitingEmpty()"):]
	sync = sync[:strings.Index(sync, "\n}\n")]

	if !strings.Contains(sync, "window.pnCountWaiting()") || strings.Index(sync, "pnCountWaiting") > strings.Index(sync, "if (!empty) return;") {
		t.Error("console.js does not count the waiting cards again each time it draws them")
	}

	status := console[strings.Index(console, "async function refreshStatus()"):]
	status = status[:strings.Index(status, "\n}\n")]

	if !strings.Contains(status, "window.pnCountWaiting(s.memory.pending_lessons)") {
		t.Error("console.js does not hand the lessons waiting to the count when it reads the status after a decision")
	}
}
