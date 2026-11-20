package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
)

/*
 * The page's two scripts, run as the window runs them, on the real server.
 *
 * A window opened by its link takes the key, keeps it for the tab, and what
 * it posts afterwards is let through; the same page opened without the link
 * (a browser somebody pointed at the address, a program that fetched the
 * page) is refused and can tell why. The requests are made by node's own
 * fetch with nothing added by the test.
 */
func TestThePageOpenedByItsLinkDecidesAndAnyOtherPageDoesNot(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	s := plain(t, config.ReachHere)

	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)

	ts := server.URL

	dir := t.TempDir()

	for _, name := range []string{"desk.js", "desk-enter.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const [dir, base, link] = process.argv.slice(2);

// A tab: its own session storage, its own address, the real fetch.
function tab(address) {
    const kept = new Map();
    const seen = [];
    const win = {
        location: {
            href: address,
            pathname: new URL(address).pathname,
            replace(to) { win.location.href = new URL(to, win.location.href).href; win.location.pathname = new URL(win.location.href).pathname; win.replaced = true; },
        },
        sessionStorage: { getItem: (k) => (kept.has(k) ? kept.get(k) : null), setItem: (k, v) => kept.set(k, String(v)) },
        fetch(input, init) {
            const headers = new Headers((init && init.headers) || undefined);
            seen.push({ to: String(input), method: (init && init.method) || 'GET', key: headers.get('X-PN-Desk') });
            return fetch(new URL(String(input), win.location.href), init);
        },
        seen,
        kept,
    };
    return win;
}

function load(win, script) {
    const hidden = {};
    const document = { getElementById: (id) => (hidden[id] = hidden[id] || { hidden: id === 'desk-failed' }) };
    win.elements = hidden;
    return new Function('window', 'document', 'fetch', 'return ' + fs.readFileSync(path.join(dir, script), 'utf8').replace(/^\/\*[\s\S]*?\*\/\s*/, '').replace(/;\s*$/, ''))(win, document, (...a) => win.fetch(...a));
}

async function post(win, to, body) {
    const res = await win.fetch(to, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    let said = null;
    try { said = await res.json(); } catch (e) { said = null; }
    return { status: res.status, said };
}

(async () => {
    const out = {};

    // The window, opened by its link.
    const own = tab(link);
    await load(own, 'desk-enter.js');
    out.replaced = own.replaced === true && own.location.pathname === '/';
    out.kept = own.kept.get('pn-desk') || '';
    load(own, 'desk.js');
    out.here = own.pnDesk.here();
    const before = own.seen.length;
    out.allowed = await post(own, '/api/outside/allow', { subject: 'programs', allowed: true });
    await own.fetch('/api/outside');
    out.sent = own.seen.slice(before);
    out.refusedOwn = own.pnDesk.refused(out.allowed.status, out.allowed.said);

    // The rule itself: never to another address, never on a reading.
    const c = own.pnDesk.carry;
    const keyOn = (init) => (init && init.headers ? new Headers(init.headers).get('X-PN-Desk') : null);
    out.elsewhere = keyOn(c('k', base + '/', 'http://127.0.0.1:1/api/approvals/1/approve', { method: 'POST' }));
    out.otherHost = keyOn(c('k', base + '/', 'https://example.org/x', { method: 'POST' }));
    out.reading = keyOn(c('k', base + '/', '/api/approvals', undefined));
    out.posting = keyOn(c('k', base + '/', '/api/approvals/1/approve', { method: 'post' }));
    out.keepsHeaders = new Headers(c('k', base + '/', '/api/x', { method: 'POST', headers: { 'Content-Type': 'application/json' } }).headers).get('Content-Type');
    const asGiven = { method: 'POST' };
    out.noKey = c('', base + '/', '/api/approvals/1/approve', asGiven) === asGiven && !asGiven.headers ? 'left alone' : 'changed';

    // The same link a second time, in another tab: used up.
    const second = tab(link);
    await load(second, 'desk-enter.js');
    out.secondKept = second.kept.get('pn-desk') || '';
    out.secondSaid = second.elements['desk-failed'].hidden === false && second.elements['desk-opening'].hidden === true;
    out.secondStayed = second.replaced !== true;

    // A page nobody gave a link to.
    const other = tab(base + '/');
    load(other, 'desk.js');
    out.otherHere = other.pnDesk.here();
    out.otherAllowed = await post(other, '/api/outside/allow', { subject: 'ask', allowed: true });
    out.otherRefused = other.pnDesk.refused(out.otherAllowed.status, out.otherAllowed.said);
    out.otherCard = await post(other, '/api/approvals/1/approve', {});

    process.stdout.write(JSON.stringify(out));
})().catch((e) => { console.error(e); process.exit(1); });
`), 0o644)

	raw, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, ts, s.DeskLink(ts)).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("the page scripts failed: %s", exit.Stderr)
		}

		t.Fatal(err)
	}

	var got struct {
		Replaced bool   `json:"replaced"`
		Kept     string `json:"kept"`
		Here     bool   `json:"here"`
		Allowed  struct {
			Status int `json:"status"`
		} `json:"allowed"`
		Sent []struct {
			To, Method string
			Key        *string
		} `json:"sent"`
		RefusedOwn   bool    `json:"refusedOwn"`
		Elsewhere    *string `json:"elsewhere"`
		OtherHost    *string `json:"otherHost"`
		Reading      *string `json:"reading"`
		Posting      *string `json:"posting"`
		KeepsHeaders string  `json:"keepsHeaders"`
		NoKey        string  `json:"noKey"`
		SecondKept   string  `json:"secondKept"`
		SecondSaid   bool    `json:"secondSaid"`
		SecondStayed bool    `json:"secondStayed"`
		OtherHere    bool    `json:"otherHere"`
		OtherAllowed struct {
			Status int               `json:"status"`
			Said   map[string]string `json:"said"`
		} `json:"otherAllowed"`
		OtherRefused bool `json:"otherRefused"`
		OtherCard    struct {
			Status int `json:"status"`
		} `json:"otherCard"`
	}

	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the page scripts said %s: %v", raw, err)
	}

	if !got.Replaced || got.Kept != testDeskKey || !got.Here {
		t.Errorf("the window opened by its link did not take the key and go on to the first page: %s", raw)
	}

	if got.Allowed.Status != 200 || got.RefusedOwn {
		t.Errorf("the window's own switch was answered %d", got.Allowed.Status)
	}

	if len(got.Sent) != 2 || got.Sent[0].Key == nil || *got.Sent[0].Key != testDeskKey || got.Sent[1].Key != nil {
		t.Errorf("the key goes with what the page posts and with nothing it reads; sent: %s", raw)
	}

	if got.Elsewhere != nil || got.OtherHost != nil || got.Reading != nil {
		t.Errorf("the key was put on a request to another address or on a reading: %s", raw)
	}

	if got.Posting == nil || *got.Posting != "k" || got.KeepsHeaders != "application/json" || got.NoKey != "left alone" {
		t.Errorf("the rule changed a request it should have left, or left one it should carry: %s", raw)
	}

	if got.SecondKept != "" || !got.SecondSaid || !got.SecondStayed {
		t.Errorf("the link worked a second time, or the page did not say that it no longer works: %s", raw)
	}

	if got.OtherHere || got.OtherAllowed.Status != 403 || !got.OtherRefused || got.OtherCard.Status != 403 {
		t.Errorf("a page nobody gave the link to decided, or could not tell why it was refused: %s", raw)
	}

	if on, err := s.brain.DB.OutsideAllowed(); err != nil || len(on) != 1 {
		t.Errorf("exactly the window's switch should be on: %v (%v)", on, err)
	}
}

// The page loads the key's script before anything that posts (only pace.js,
// which holds the page's timers and posts nothing, comes before it), and
// has the words for a refusal in both languages.
func TestThePageCarriesTheKeyBeforeAnythingElse(t *testing.T) {
	page := readAsset(t, "index.html")

	first := strings.Index(page, `<script src="`)
	if first < 0 || !strings.HasPrefix(page[first:], `<script src="/js/pace.js"></script>`+"\n"+`<script src="/js/desk.js"></script>`) {
		t.Error("desk.js does not come straight after pace.js: a script before it would post without the key")
	}

	if pace := readAsset(t, filepath.Join("js", "pace.js")); strings.Contains(pace, "fetch(") || strings.Contains(pace, "XMLHttpRequest") {
		t.Error("pace.js, which runs before the key's script, makes requests of its own now")
	}

	for _, script := range []string{"console.js", "room.js", "programs.js", "devices.js"} {
		if src := readAsset(t, filepath.Join("js", script)); strings.Contains(src, "XMLHttpRequest") || strings.Contains(src, "sendBeacon") {
			t.Errorf("%s posts by something other than fetch, which would go without the key", script)
		}
	}

	en, bg := strings2(t, readAsset(t, filepath.Join("js", "i18n.js")), "desk.notHere")
	if !strings.Contains(en, "own window") || !strings.Contains(bg, "прозорец") {
		t.Errorf("the refusal is not said in both languages: %q / %q", en, bg)
	}

	for _, script := range []string{"console.js", "room.js", "programs.js", "devices.js"} {
		if !strings.Contains(readAsset(t, filepath.Join("js", script)), "'desk.notHere'") {
			t.Errorf("%s does not say why an answer was refused", script)
		}
	}

	for language, words := range deskWords {
		for _, said := range []string{words.Opening, words.Failed, words.What, words.Go} {
			if strings.TrimSpace(said) == "" || strings.ContainsAny(said, "–—") {
				t.Errorf("the page behind the link (%s) has an empty line or a dash: %q", language, said)
			}
		}
	}
}
