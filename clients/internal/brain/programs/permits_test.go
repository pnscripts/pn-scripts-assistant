package programs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func rec(row int64, kind Kind, id, name, version string, present bool) Record {
	r := Record{Program: Program{Kind: kind, ID: id, Name: name, Version: version}, RowID: row, Present: present}
	if !present {
		at := time.Date(2026, 11, 12, 14, 3, 0, 0, time.Local)
		r.RemovedAt = &at
	}

	return r
}

func inventory() []Record {
	firefoxApp := rec(1, App, "firefox_firefox.desktop", "Firefox", "", true)
	firefoxApp.Exec = "/snap/bin/firefox"

	gimpApp := rec(3, App, "org.gimp.GIMP.desktop", "GIMP", "", true)
	gimpApp.Exec = "/usr/bin/flatpak"

	kritaApp := rec(5, App, "org.kde.krita.desktop", "Krita", "", false)
	kritaApp.Exec = "krita"

	geditApp := rec(7, App, "org.gnome.gedit.desktop", "Text Editor", "", true)
	geditApp.Exec = "gedit"

	win := rec(9, Win, "Notepad++", "Notepad++", "8.6", true)
	win.Path = `C:\Program Files\Notepad++`

	return []Record{
		firefoxApp,
		rec(2, Snap, "firefox", "Firefox", "132.0", true),
		gimpApp,
		rec(4, Flatpak, "org.gimp.GIMP", "GIMP", "2.10.38", true),
		kritaApp,
		rec(6, Deb, "krita", "krita", "5.2", false),
		geditApp,
		rec(8, Deb, "gedit", "gedit", "46.2", true),
		win,
	}
}

func keys(rs []Record) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Key())
	}

	return strings.Join(out, " ")
}

func TestAUseIsMatchedToEveryRecordOfItsProgram(t *testing.T) {
	all := inventory()

	cases := []struct {
		name string
		use  Use
		want string
	}{
		{"a snap's menu entry leads to the snap", Use{Desktop: "firefox_firefox.desktop"}, "app:firefox_firefox.desktop snap:firefox"},
		{"the snap's program file", Use{Path: "/snap/bin/firefox"}, "app:firefox_firefox.desktop snap:firefox"},
		{"a running snap's file", Use{Path: "/snap/firefox/5091/usr/lib/firefox/firefox"}, "snap:firefox"},
		{"a Flatpak's menu entry by its desktop id", Use{Desktop: "org.gimp.GIMP"}, "app:org.gimp.GIMP.desktop"},
		{"a Flatpak by its application id", Use{Flatpak: "org.gimp.GIMP"}, "app:org.gimp.GIMP.desktop flatpak:org.gimp.GIMP"},
		{"a Flatpak's exported command", Use{Path: "/var/lib/flatpak/exports/bin/org.gimp.GIMP"}, "app:org.gimp.GIMP.desktop flatpak:org.gimp.GIMP"},
		{"a package by its owner and its menu entry by the file", Use{Path: "/usr/bin/gedit", Package: "gedit"}, "app:org.gnome.gedit.desktop deb:gedit"},
		{"a Windows program inside its install folder", Use{Path: `c:\program files\notepad++\notepad++.exe`}, "win:Notepad++"},
		{"not a neighbour folder", Use{Path: `C:\Program Files\Notepad++ Extra\x.exe`}, ""},
		{"a removed program by the name it was called", Use{Name: "Krita"}, "app:org.kde.krita.desktop deb:krita"},
		{"the name only when nothing else matched", Use{Path: "/usr/bin/gedit", Package: "gedit", Name: "krita"}, "app:org.gnome.gedit.desktop deb:gedit"},
		{"nothing known", Use{Path: "/opt/unknown/bin/x", Name: "unknown"}, ""},
	}

	for _, c := range cases {
		if got := keys(Match(all, c.use)); got != c.want {
			t.Errorf("%s: matched %q, want %q", c.name, got, c.want)
		}
	}
}

func TestARemovedProgramIsNeverUsedAndTheAnswerSaysSinceWhen(t *testing.T) {
	all := inventory()
	m := Match(all, Use{Name: "krita"})

	// Even an "always" given before it went is dormant.
	permits := map[int64][]Permit{5: {{Verb: VerbStart, Answer: Always}}}

	r := Rule(m, permits, VerbStart)
	if r.Answer != Never || r.Why != WhyGone {
		t.Fatalf("a removed program was not refused: %+v", r)
	}

	if !strings.Contains(r.Said, "Krita was removed from this computer on 12 Nov 2026 at 14:03") ||
		!strings.Contains(r.Said, "used again if it comes back") {
		t.Errorf("the refusal does not say when it went: %q", r.Said)
	}
}

func TestAProgramThatCameBackIsUsedAgainWithItsKeptDecisions(t *testing.T) {
	all := inventory()
	all[4].Present, all[4].RemovedAt, all[4].LastChange = true, nil, Returned
	m := Match(all, Use{Desktop: "org.kde.krita.desktop"})

	r := Rule(m, map[int64][]Permit{5: {{Verb: VerbStart, Answer: Always}}}, VerbStart)
	if r.Answer != Always || r.Why != WhyAllowed {
		t.Errorf("a program back again lost its decision: %+v", r)
	}
}

func TestNeverWinsOverAlwaysOnAnotherCardOfTheSameProgram(t *testing.T) {
	m := Match(inventory(), Use{Desktop: "firefox_firefox.desktop"})
	permits := map[int64][]Permit{
		1: {{Verb: VerbStart, Answer: Always}},
		2: {{Verb: VerbStart, Answer: Never, Version: "132.0"}},
	}

	r := Rule(m, permits, VerbStart)
	if r.Answer != Never || r.Why != WhyRefused || r.Program.Kind != Snap {
		t.Fatalf("never did not win: %+v", r)
	}

	if !strings.Contains(r.Said, "never start it") || !strings.Contains(r.Said, "Programs page") {
		t.Errorf("the refusal does not say what and where to change it: %q", r.Said)
	}
}

func TestAlwaysIsForOneVerbAndOneVersion(t *testing.T) {
	m := Match(inventory(), Use{Path: "/snap/bin/firefox"})
	permits := map[int64][]Permit{2: {{Verb: VerbStart, Answer: Always, Version: "132.0"}}}

	if r := Rule(m, permits, VerbStart); r.Answer != Always || r.Why != WhyAllowed {
		t.Errorf("always for this version did not allow: %+v", r)
	}

	if r := Rule(m, permits, VerbInput); r.Answer != AskEach || r.Why != WhyUndecided || r.Program == nil {
		t.Errorf("always for starting allowed typing: %+v", r)
	}

	// Updated since: asked again, said why, kept.
	all := inventory()
	all[1].Version = "133.0"

	r := Rule(Match(all, Use{Path: "/snap/bin/firefox"}), permits, VerbStart)
	if r.Answer != AskEach || r.Why != WhyUpdated || !strings.Contains(r.Said, "was updated since") {
		t.Errorf("always for an older version still allowed: %+v", r)
	}

	// A menu entry has no version: "always" given there stays awake.
	r = Rule(Match(all, Use{Path: "/snap/bin/firefox"}), map[int64][]Permit{1: {{Verb: VerbStart, Answer: Always}}}, VerbStart)
	if r.Answer != Always {
		t.Errorf("always on a menu entry slept: %+v", r)
	}
}

func TestReadingStaysAllowedAcrossAnUpdate(t *testing.T) {
	p := Permit{Verb: VerbRead, Answer: Always, Version: "1"}
	if p.Asleep("2") {
		t.Error("reading slept after an update")
	}

	if !(Permit{Verb: VerbInput, Answer: Always, Version: "1"}).Asleep("2") {
		t.Error("typing stayed allowed after an update")
	}
}

func TestNothingKnownIsLeftToTheGeneralSetting(t *testing.T) {
	r := Rule(nil, nil, VerbStart)
	if r.Answer != AskEach || r.Why != WhyUndecided || r.Program != nil || r.Said != "" {
		t.Errorf("an unknown program was decided: %+v", r)
	}
}

func TestTheMostRecentlyRemovedRecordSaysWhen(t *testing.T) {
	a := rec(1, App, "x.desktop", "X", "", false)
	b := rec(2, Deb, "x", "x", "1", false)
	later := b.RemovedAt.Add(time.Hour)
	b.RemovedAt = &later

	r := Rule([]Record{a, b}, nil, VerbStart)
	if r.Program.RowID != 2 || !strings.Contains(r.Said, "15:03") {
		t.Errorf("the wrong removal was said: %+v", r)
	}
}

func TestVerbsAndAnswersAreKnown(t *testing.T) {
	for _, v := range InUse {
		if !KnownVerb(v) || VerbWords(v) == string(v) {
			t.Errorf("verb %q in use is not known or not said", v)
		}
	}

	if KnownVerb("format-disk") || KnownAnswer("maybe") || !KnownAnswer(Never) {
		t.Error("an unknown verb or answer was accepted")
	}
}

func TestPathsAreReadForSnapsAndFlatpaks(t *testing.T) {
	cases := map[string][2]string{
		"/snap/bin/code":                  {"code", ""},
		"/snap/bin/chromium.chromedriver": {"chromium", ""},
		"/snap/core22/1234/usr/bin/x":     {"core22", ""},
		"/snap/bin":                       {"", ""},
		"/home/you/.local/share/flatpak/exports/bin/org.x.Y":       {"", "org.x.Y"},
		"/var/lib/flatpak/app/org.x.Y/x86_64/stable/a/files/bin/y": {"", "org.x.Y"},
		"/usr/bin/snapshot": {"", ""},
	}

	for path, want := range cases {
		if s, f := snapOfPath(path), flatpakOfPath(path); s != want[0] || f != want[1] {
			t.Errorf("%s: snap %q flatpak %q, want %v", path, s, f, want)
		}
	}
}

func TestOnlyAProgramKnownByNameAloneIsRefusedAsRemoved(t *testing.T) {
	all := inventory()

	if r := Weigh(all, nil, Use{Verb: VerbStart, Name: "krita"}); r.Why != WhyGone {
		t.Errorf("a removed program named alone was not refused: %+v", r)
	}

	// Its file is here again (reinstalled another way, or not read yet).
	if r := Weigh(all, nil, Use{Verb: VerbChange, Path: "/opt/krita/krita", Name: "krita"}); r.Answer != AskEach || r.Why != WhyUndecided {
		t.Errorf("a program whose file is here was refused as removed: %+v", r)
	}

	if r := Weigh(all, nil, Use{Verb: VerbStart, Desktop: "org.kde.krita.desktop"}); r.Why == WhyGone {
		t.Errorf("a program whose menu entry is here was refused as removed: %+v", r)
	}

	if r := Weigh(all, nil, Use{Verb: VerbInput}); r.Answer != AskEach || r.Program != nil {
		t.Errorf("an unknown window was decided: %+v", r)
	}
}

func TestAPackageIsAskedOnlyWhenAPackageHasADecision(t *testing.T) {
	all := inventory()
	u := Use{Verb: VerbChange, Path: "/usr/bin/gedit"}

	if NeedsPackage(all, nil, u) {
		t.Error("asked dpkg with no decision on any package")
	}

	if !NeedsPackage(all, map[int64][]Permit{8: {{Verb: VerbChange, Answer: Never}}}, u) {
		t.Error("did not ask with a decision on a package")
	}

	if NeedsPackage(all, map[int64][]Permit{1: {{Verb: VerbChange, Answer: Never}}}, u) {
		t.Error("asked dpkg for a decision on a menu entry")
	}
}

func TestThePackageOfAFileIsReadFromDpkgQuery(t *testing.T) {
	var asked [][]string

	s := Scanner{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		asked = append(asked, append([]string{name}, args...))

		switch args[1] {
		case "/usr/bin/gedit":
			return []byte("diversion by x from: /usr/bin/gedit\ngedit:amd64, gedit-common: /usr/bin/gedit\n"), nil
		default:
			return nil, errors.New("dpkg-query: no path found matching pattern")
		}
	}}

	if got := s.PackageOf(context.Background(), "/usr/bin/gedit"); got != "gedit" {
		t.Errorf("owner %q", got)
	}

	if got := s.PackageOf(context.Background(), "/opt/x/y"); got != "" {
		t.Errorf("a file nobody owns: %q", got)
	}

	if got := s.PackageOf(context.Background(), "relative"); got != "" || len(asked) != 2 {
		t.Errorf("a relative path was asked about: %q %v", got, asked)
	}
}
