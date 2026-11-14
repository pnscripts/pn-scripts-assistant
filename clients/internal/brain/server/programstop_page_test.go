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
)

type stopSaid struct {
	Running struct {
		Stopped bool   `json:"stopped"`
		Said    string `json:"said"`
		Button  string `json:"button"`
	} `json:"running"`
	Stopped struct {
		Stopped bool   `json:"stopped"`
		Said    string `json:"said"`
		Button  string `json:"button"`
	} `json:"stopped"`
	MayStopped string `json:"mayStopped"`
	Offers     map[string]struct {
		Approve    bool `json:"approve"`
		ForProgram *struct {
			Label string `json:"label"`
			Title string `json:"title"`
		} `json:"forProgram"`
		AnyProgram *struct {
			Label     string `json:"label"`
			Title     string `json:"title"`
			Secondary bool   `json:"secondary"`
		} `json:"anyProgram"`
		Note string `json:"note"`
	} `json:"offers"`
}

/*
 * The switch and the approval card's answers, worked out by programs.js in
 * node on the real server's answers, in English and in Bulgarian: the
 * switch's sentence and button before and after a press, and what a
 * question about one program, about none, always asked, and while stopped
 * offers.
 */
func TestTheStopSwitchAndTheApprovalAnswersInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, b := newServer(t)

	ran := 0
	b.Agent.Registry.Register(startingDemo{ran: &ran})
	demoAsked(t, db, true)

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

	running := read("/api/programs")
	cards := read("/api/approvals")

	postPermit(t, ts.URL+"/api/programs/stop", nil)

	stopped := read("/api/programs")
	stoppedCards := read("/api/approvals")

	en := stopPageSays(t, "en-GB", running, stopped, cards, stoppedCards)
	bg := stopPageSays(t, "bg-BG", running, stopped, cards, stoppedCards)

	if en.Running.Stopped || en.Running.Button != "Stop using other programs" || !strings.Contains(en.Running.Said, "One click stops all of it") {
		t.Errorf("in English, running: %+v", en.Running)
	}

	if !en.Stopped.Stopped || en.Stopped.Button != "Use other programs again" ||
		!regexp.MustCompile(`^Stopped since \d{1,2} \w{3} \d{4}, \d{2}:\d{2}: `).MatchString(en.Stopped.Said) {
		t.Errorf("in English, stopped: %+v", en.Stopped)
	}

	if !regexp.MustCompile(`^Спряно от \d{2}\.\d{2}\.\d{4}, \d{2}:\d{2}: `).MatchString(bg.Stopped.Said) || bg.Stopped.Button != "Използвай отново други програми" {
		t.Errorf("in Bulgarian, stopped: %+v", bg.Stopped)
	}

	if !strings.Contains(en.MayStopped, "Other programs are stopped now") {
		t.Errorf("the card does not say it is stopped: %q", en.MayStopped)
	}

	// A question about Demo: Always for Demo, and the wider Always beside it.
	demo := en.Offers["demo"]
	if !demo.Approve || demo.ForProgram == nil || demo.ForProgram.Label != "Always for Demo" ||
		!strings.Contains(demo.ForProgram.Title, "only version 1.0") || !strings.Contains(demo.ForProgram.Title, "start it") ||
		demo.AnyProgram == nil || demo.AnyProgram.Label != "Always, any program" || !demo.AnyProgram.Secondary {
		t.Errorf("in English a question about Demo offers %+v %+v", demo.ForProgram, demo.AnyProgram)
	}

	if bg.Offers["demo"].ForProgram == nil || bg.Offers["demo"].ForProgram.Label != "Винаги за Demo" ||
		bg.Offers["demo"].AnyProgram.Label != "Винаги, за всяка програма" {
		t.Errorf("in Bulgarian a question about Demo offers %+v", bg.Offers["demo"])
	}

	// Typing into Demo: always asked, so only Demo's own Always.
	typing := en.Offers["typing"]
	if typing.ForProgram == nil || typing.AnyProgram != nil {
		t.Errorf("typing offers %+v %+v", typing.ForProgram, typing.AnyProgram)
	}

	// No program: the Always it always had.
	plain := en.Offers["plain"]
	if plain.ForProgram != nil || plain.AnyProgram == nil || plain.AnyProgram.Label != "Always allow" || plain.AnyProgram.Secondary {
		t.Errorf("a question about no program offers %+v %+v", plain.ForProgram, plain.AnyProgram)
	}

	// Asked for another reason too: nothing lasting.
	other := en.Offers["other"]
	if other.ForProgram != nil || other.AnyProgram != nil {
		t.Errorf("an always asked question offers %+v %+v", other.ForProgram, other.AnyProgram)
	}

	// Stopped: only Reject, and why.
	st := en.Offers["stopped"]
	if st.Approve || st.ForProgram != nil || st.AnyProgram != nil || !strings.Contains(st.Note, "You stopped me using other programs") {
		t.Errorf("a stopped question offers %+v", st)
	}

	if !strings.Contains(bg.Offers["stopped"].Note, "Спряхте ми използването на други програми") {
		t.Errorf("in Bulgarian, stopped: %q", bg.Offers["stopped"].Note)
	}

	// Bulgarian has no English sentence left (names apart).
	latin := regexp.MustCompile(`[a-zA-Z]{3,}`)
	for _, s := range []string{bg.Running.Said, bg.Running.Button, bg.Stopped.Said, bg.MayStopped, bg.Offers["demo"].ForProgram.Title, bg.Offers["demo"].AnyProgram.Title, bg.Offers["stopped"].Note} {
		if latin.MatchString(strings.NewReplacer("Demo", "", "zz_start_demo", "").Replace(s)) {
			t.Errorf("English left in Bulgarian: %q", s)
		}
	}

	if ran != 0 {
		t.Error("something ran")
	}
}

func stopPageSays(t *testing.T, language string, running, stopped, cards, stoppedCards []byte) stopSaid {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "programs.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	for name, raw := range map[string][]byte{"running.json": running, "stopped.json": stopped, "cards.json": cards, "stoppedCards.json": stoppedCards} {
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
const running = json('running.json'), stopped = json('stopped.json');
const card = json('cards.json')[0], stoppedCard = json('stoppedCards.json')[0];
const typing = Object.assign({}, card, { summary: 'Always asked (typing and clicking in another program): Demo: Type',
    program: Object.assign({}, card.program, { verb: 'control-input' }) });
const plain = { id: 9, tool: 'remember', summary: 'Remember this' };
const other = Object.assign({}, card, { summary: 'Always asked (installing software): Demo: Start demo',
    program: Object.assign({}, card.program, { always: false }) });
process.stdout.write(JSON.stringify({
    running: P.stopping(running),
    stopped: P.stopping(stopped),
    mayStopped: P.may({ kind: 'deb', id: 'demo', version: '1.0', present: true }, stopped).note,
    offers: {
        demo: P.approvalChoices(card, 'zz_start_demo'),
        typing: P.approvalChoices(typing, 'zz_start_demo'),
        plain: P.approvalChoices(plain, 'remember'),
        other: P.approvalChoices(other, 'zz_start_demo'),
        stopped: P.approvalChoices(stoppedCard, 'zz_start_demo'),
    },
}));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said stopSaid
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}
