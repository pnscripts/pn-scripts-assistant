package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The Programs page says the inventory in the page's language.
 *
 * A scratch store gets a history a person could have had (programs there at
 * the first look, one installed since, one updated, one removed and back,
 * one removed today, one removed a month ago), the server answers, and the
 * page's own script runs on that answer in node with the browser in English
 * and in Bulgarian. What a person would read is checked, not the drawing.
 */
func TestTheProgramsPageSaysTheInventoryInBothLanguages(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	ts, db, _ := newServer(t)

	now := time.Now().UTC()
	month := now.Add(-30 * 24 * time.Hour)
	week := now.Add(-7 * 24 * time.Hour)
	yesterday := now.Add(-24 * time.Hour)

	look := func(kind programs.Kind, seen []programs.Program, at time.Time, first bool) {
		t.Helper()

		known, err := db.Programs(kind)
		if err != nil {
			t.Fatal(err)
		}

		save, changes := programs.Reconcile(kind, known, seen, at)
		if first {
			save, _ = programs.FirstLook(save, changes)
		}

		if err := db.SaveLook(kind, save, at); err != nil {
			t.Fatal(err)
		}
	}

	deb := func(id, version string) programs.Program {
		return programs.Program{Kind: programs.Deb, ID: id, Name: id, Version: version}
	}

	app := func(id, name string) programs.Program {
		return programs.Program{Kind: programs.App, ID: id + ".desktop", Name: name, Path: "/usr/share/applications/" + id + ".desktop", Exec: id}
	}

	// A month ago: the first look. Then, over time, everything else.
	look(programs.Deb, []programs.Program{deb("gimp", "2.10"), deb("vlc", "3.0"), deb("oldtool", "1.0")}, month, true)
	look(programs.App, []programs.Program{app("gimp", "GNU Image Manipulation Program")}, month, true)
	look(programs.Deb, []programs.Program{deb("gimp", "2.10"), deb("vlc", "3.0")}, month.Add(time.Hour), false)
	look(programs.Deb, []programs.Program{deb("gimp", "3.0"), deb("krita", "5.2")}, week, false)
	look(programs.Deb, []programs.Program{deb("gimp", "3.0"), deb("krita", "5.2"), deb("vlc", "3.0")}, yesterday, false)
	look(programs.App, []programs.Program{app("gimp", "GNU Image Manipulation Program"), app("inkscape", "Inkscape")}, yesterday, false)
	look(programs.Deb, []programs.Program{deb("gimp", "3.0"), deb("vlc", "3.0")}, now, false)

	res, err := http.Get(ts.URL + "/api/programs")
	if err != nil {
		t.Fatal(err)
	}

	answer, _ := io.ReadAll(res.Body)
	res.Body.Close()

	en := programsPageSays(t, "en-GB", answer)
	bg := programsPageSays(t, "bg-BG", answer)

	if en.Summary != "Programs here: 4 (in the menu 2, packages 2, snaps 0, Flatpaks 0)." {
		t.Errorf("in English the summary is %q", en.Summary)
	}

	// What changed in the last two weeks, newest first; the removal a
	// month ago is too old to be news.
	said := map[string]string{}
	var order []string

	for _, c := range en.Changes {
		said[c.Name] = c.Said
		order = append(order, c.Name)
	}

	if strings.Join(order, ",") != "krita,vlc,Inkscape,gimp" && strings.Join(order, ",") != "krita,Inkscape,vlc,gimp" {
		t.Errorf("in English the changes are %v", order)
	}

	for name, want := range map[string]string{
		"krita":    "Package · Removed on ",
		"vlc":      "Package · Came back on ",
		"Inkscape": "Menu program · Installed on ",
		"gimp":     "Package · Updated from 2.10 to 3.0 on ",
	} {
		if !strings.HasPrefix(said[name], want) {
			t.Errorf("in English %s says %q, want it to start %q", name, said[name], want)
		}
	}

	if _, old := said["oldtool"]; old {
		t.Error("a removal a month ago is listed as news")
	}

	// The removed list keeps both removals, with their dates.
	if len(en.Removed) != 2 || en.Removed[0].Name != "krita" || en.Removed[1].Name != "oldtool" {
		t.Errorf("in English the removed list is %+v", en.Removed)
	}

	// Searching narrows by name or command, and a search that finds
	// nothing says so instead of an empty box.
	if len(en.Search) != 1 || en.Search[0].Name != "Inkscape" || en.Search[0].Said != "inkscape" {
		t.Errorf("searching the menu for \"inks\": %+v", en.Search)
	}

	if !strings.Contains(en.NoMatch, "zzz") {
		t.Errorf("a search with no match says %q", en.NoMatch)
	}

	// The first-contact card of a program installed since, and of one
	// already there at the first look.
	card := map[string]string{}
	for _, r := range en.Card {
		card[r[0]] = r[1]
	}

	if card["Kind"] != "Menu program" || card["Starts"] != "inkscape" || !strings.HasPrefix(card["Where"], "/usr/share/applications/") ||
		card["First seen"] == "" || strings.HasPrefix(card["First seen"], "Already here") {
		t.Errorf("in English Inkscape's card is %v", en.Card)
	}

	if _, twice := card["Last change"]; twice {
		t.Errorf("an install is said twice on the card: %v", en.Card)
	}

	initial := map[string]string{}
	for _, r := range en.InitialCard {
		initial[r[0]] = r[1]
	}

	if !strings.HasPrefix(initial["First seen"], "Already here when I first looked, on ") ||
		!strings.HasPrefix(initial["Last change"], "Updated from 2.10 to 3.0") {
		t.Errorf("in English gimp's card is %v", en.InitialCard)
	}

	if !strings.Contains(en.CanDo, "ask you first") || !strings.Contains(en.CanDoGone, "removed on") {
		t.Errorf("in English what it can do: %q / %q", en.CanDo, en.CanDoGone)
	}

	// Every reason the server gives for polling a folder has words here.
	reasons := programs.Reasons()
	sort.Strings(en.Reasons)

	if strings.Join(en.Reasons, "|") != strings.Join(reasons, "|") {
		t.Errorf("programs.js knows the reasons %q, the server gives %q", en.Reasons, reasons)
	}

	// Large counts are grouped the way each language writes them.
	if en.Many != "Programs here: 12,345 (in the menu 0, packages 12,345, snaps 0, Flatpaks 0)." {
		t.Errorf("in English a large count reads %q", en.Many)
	}

	if !regexp.MustCompile(`^Програми тук: 12[\s\x{00a0}\x{202f}]345 \(`).MatchString(bg.Many) {
		t.Errorf("in Bulgarian a large count reads %q", bg.Many)
	}

	// A long list shows 200 and says so; it is never cut silently.
	if en.Long.Rows != 200 || en.Long.Total != 12345 ||
		en.Long.Note != "Showing the first 200 of 12,345. Type a name to narrow the list." {
		t.Errorf("in English a long list is %+v", en.Long)
	}

	// How it stays current: watched folders added up, folders that are not
	// there yet counted (most computers have no Flatpak), any other reason
	// named with its folder.
	if len(en.Keeping) != 3 || !strings.HasPrefix(en.Keeping[0], "Places watched: 5.") ||
		!strings.Contains(en.Keeping[1], "/usr/share/applications is looked at every 10 minutes instead (the system refused a file watch)") ||
		!strings.HasPrefix(en.Keeping[2], "Folders where programs could appear but which are not there yet") ||
		!strings.Contains(en.Keeping[2], "): 2. Each is looked at every 10 minutes.") {
		t.Errorf("in English how it stays current is %q", en.Keeping)
	}

	// Bulgarian: every sentence the page makes is Bulgarian; the only
	// Latin letters are names (the programs', their ids and paths, the
	// "snap" and "Flatpak" formats) and what was searched for.
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)
	latin := regexp.MustCompile(`[a-zA-Z]{3,}`)

	strip := func(s string) string {
		for _, name := range []string{"Flatpak", "snap", "GNU Image Manipulation Program", "Inkscape", "inkscape.desktop", "inkscape", "krita", "vlc", "gimp", "zzz"} {
			s = strings.ReplaceAll(s, name, "")
		}

		return regexp.MustCompile(`/\S+`).ReplaceAllString(s, "")
	}

	texts := []string{bg.Summary, bg.Windows, bg.NoMatch, bg.CanDo, bg.CanDoGone, bg.Long.Note}
	texts = append(texts, bg.Why...)
	texts = append(texts, bg.Keeping...)

	for _, c := range append(append([]programsRow{}, bg.Changes...), bg.Removed...) {
		texts = append(texts, c.Said)
	}

	for _, r := range append(append([][2]string{}, bg.Card...), bg.InitialCard...) {
		texts = append(texts, r[0], r[1])
	}

	for _, s := range texts {
		if !cyrillic.MatchString(s) && latin.MatchString(strip(s)) {
			t.Errorf("in Bulgarian the page says %q", s)

			continue
		}

		if latin.MatchString(strip(s)) {
			t.Errorf("English left in Bulgarian: %q", s)
		}
	}

	if bg.Summary != "Програми тук: 4 (в менюто 2, пакети 2, snap пакети 0, Flatpak пакети 0)." {
		t.Errorf("in Bulgarian the summary is %q", bg.Summary)
	}

	// A Windows computer: its own sources, in its own words, and only
	// those offered in the list; an older server's answer is Linux's.
	if en.Windows != "Programs here: 3 (in the menu 1, installed programs 1, app packages 1)." ||
		bg.Windows != "Програми тук: 3 (в менюто 1, инсталирани програми 1, пакети на приложения 1)." {
		t.Errorf("a Windows summary: %q / %q", en.Windows, bg.Windows)
	}

	if strings.Join(en.WinChoices, ",") != "app,win,msix,removed" || strings.Join(en.OldChoices, ",") != "app,deb,snap,flatpak,removed" {
		t.Errorf("the choices: %v / %v", en.WinChoices, en.OldChoices)
	}

	if len(en.WinCard) < 2 || en.WinCard[0][1] != "Installed program" || bg.WinCard[0][1] != "Инсталирана програма" {
		t.Errorf("an installed program's card: %v / %v", en.WinCard, bg.WinCard)
	}

	if len(en.WinList) != 1 || en.WinList[0].Name != "Windows Calculator" || en.WinList[0].Said != "11.2405.2.0" {
		t.Errorf("the app packages: %+v", en.WinList)
	}

	// Bulgarian dates are day.month.year.
	if !regexp.MustCompile(`\d{2}\.\d{2}\.\d{4}, \d{2}:\d{2}`).MatchString(bg.Changes[0].Said) {
		t.Errorf("in Bulgarian a date reads %q", bg.Changes[0].Said)
	}
}

// And the page has the view, the menu entry and the script, loaded before
// connection.js like every page script.
func TestTheProgramsPageIsOnThePage(t *testing.T) {
	page := readAsset(t, "index.html")

	for _, want := range []string{
		`<button type="button" class="nav-item" data-view="programs">`,
		`<section class="view" data-view="programs" hidden>`,
		`id="programs-summary"`, `id="programs-changes"`, `id="programs-list"`, `id="programs-card"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html has no %s", want)
		}
	}

	at := strings.Index(page, `<script src="/js/programs.js"></script>`)
	if at < 0 || at > strings.Index(page, `<script src="/js/connection.js"></script>`) {
		t.Error("programs.js is not loaded before connection.js")
	}
}

type programsRow struct {
	Name string `json:"name"`
	Said string `json:"said"`
}

type programsSaid struct {
	Summary     string        `json:"summary"`
	Changes     []programsRow `json:"changes"`
	Removed     []programsRow `json:"removed"`
	Search      []programsRow `json:"search"`
	NoMatch     string        `json:"noMatch"`
	Card        [][2]string   `json:"card"`
	InitialCard [][2]string   `json:"initialCard"`
	CanDo       string        `json:"canDo"`
	CanDoGone   string        `json:"canDoGone"`
	Reasons     []string      `json:"reasons"`
	Why         []string      `json:"why"`
	Keeping     []string      `json:"keeping"`
	Many        string        `json:"many"`
	Windows     string        `json:"windows"`
	WinChoices  []string      `json:"winChoices"`
	WinCard     [][2]string   `json:"winCard"`
	WinList     []programsRow `json:"winList"`
	OldChoices  []string      `json:"oldChoices"`
	Long        struct {
		Rows  int    `json:"rows"`
		Total int    `json:"total"`
		Note  string `json:"note"`
	} `json:"long"`
}

// programsPageSays runs i18n.js and programs.js in node, with the browser's
// language set to language, on the server's answer.
func programsPageSays(t *testing.T, language string, answer []byte) programsSaid {
	t.Helper()

	dir := t.TempDir()

	for _, name := range []string{"i18n.js", "programs.js"} {
		os.WriteFile(filepath.Join(dir, name), []byte(readAsset(t, filepath.Join("js", name))), 0o644)
	}

	os.WriteFile(filepath.Join(dir, "answer.json"), answer, 0o644)

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
eval(read('i18n.js'));
eval(read('programs.js'));
let body = JSON.parse(read('answer.json'));
if (body.data) body = body.data;
const P = window.pnPrograms;
const rows = (r) => r.map((x) => ({ name: x.name, said: x.said }));
const find = (id) => body.programs.find((p) => p.id === id);
// A Windows computer's answer.
const win = { kinds: ['app', 'win', 'msix'], first_looks: {}, programs: [
    { kind: 'app', id: 'Firefox.lnk', name: 'Firefox', present: true, exec: 'C:\\Program Files\\Mozilla Firefox\\firefox.exe' },
    { kind: 'win', id: '7-Zip', name: '7-Zip 24.09 (x64)', version: '24.09', path: 'C:\\Program Files\\7-Zip\\', present: true, initial: true },
    { kind: 'win', id: 'Discord', name: 'Discord', version: '1.0', present: false, removed_at: new Date().toISOString(), last_change: 'removed' },
    { kind: 'msix', id: 'Microsoft.WindowsCalculator_8wekyb3d8bbwe', name: 'Windows Calculator', version: '11.2405.2.0', present: true }] };
process.stdout.write(JSON.stringify({
    summary: P.summary(body),
    changes: rows(P.changes(body).rows),
    removed: rows(P.list(body, 'removed', '').rows),
    search: rows(P.list(body, 'app', 'inks').rows),
    noMatch: P.list(body, 'deb', 'zzz').note,
    card: P.found(find('inkscape.desktop'), body),
    initialCard: P.found(find('gimp'), body),
    canDo: P.canDo(find('gimp')),
    canDoGone: P.canDo(find('krita')),
    reasons: P.reasons,
    why: P.reasons.map((reason) => P.why({ reason, why: 'the server says' })),
    many: P.summary({ programs: Array.from({ length: 12345 }, (_, i) => ({ kind: 'deb', id: 'p' + i, present: true })) }),
    long: (() => { const l = P.list({ programs: Array.from({ length: 12345 }, (_, i) => ({ kind: 'deb', id: 'p' + i, present: true })) }, 'deb', '');
        return { rows: l.rows.length, total: l.total, note: l.note }; })(),
    windows: P.summary(win),
    winChoices: P.choices(win),
    winCard: P.found(win.programs[1], win),
    winList: rows(P.list(win, 'msix', '').rows),
    oldChoices: P.choices({ programs: [] }),
    keeping: P.keeping({ sources: [{ kind: 'app', watched: 3, polled: [
        { dir: '/usr/local/share/applications', reason: 'missing', why: 'not there' },
        { dir: '/var/lib/flatpak/exports/share/applications', reason: 'missing', why: 'not there' },
        { dir: '/usr/share/applications', reason: 'limit', why: 'the system refused a file watch' }] },
        { kind: 'snap', watched: 2 }] }),
}));
`), 0o644)

	out, err := exec.Command("node", filepath.Join(dir, "run.js"), dir, language).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's scripts failed: %v\n%s", err, out)
	}

	var said programsSaid
	if err := json.Unmarshal(out, &said); err != nil {
		t.Fatalf("the page's scripts said %s: %v", out, err)
	}

	return said
}
