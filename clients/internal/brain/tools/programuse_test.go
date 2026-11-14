package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/programs"
)

func swapFinding(t *testing.T, desktop map[string]string, path map[string]string) {
	t.Helper()

	d, l := findDesktopEntry, lookProgram
	t.Cleanup(func() { findDesktopEntry, lookProgram = d, l })

	findDesktopEntry = func(name string) (string, bool) { e, ok := desktop[name]; return e, ok }
	lookProgram = func(names ...string) (string, bool) { p, ok := path[names[0]]; return p, ok }
}

func TestOpeningAnAppSaysWhichProgramItStarts(t *testing.T) {
	swapFinding(t, map[string]string{"firefox": "firefox_firefox.desktop"}, map[string]string{"gedit": "/usr/bin/gedit"})

	u, ok := ProgramUseOf(context.Background(), OpenApp{}, json.RawMessage(`{"name":"firefox"}`))
	if !ok || u.Verb != programs.VerbStart || u.Desktop != "firefox_firefox.desktop" || u.Name != "firefox" {
		t.Errorf("menu entry: %+v", u)
	}

	u, _ = ProgramUseOf(context.Background(), OpenApp{}, json.RawMessage(`{"name":"Gedit"}`))
	if u.Path != "/usr/bin/gedit" || u.Desktop != "" {
		t.Errorf("command: %+v", u)
	}

	// Gone from the machine: only the name is left, so a removed program
	// can still be recognised and refused with its date.
	u, _ = ProgramUseOf(context.Background(), OpenApp{}, json.RawMessage(`{"name":"krita"}`))
	if u.Path != "" || u.Desktop != "" || u.Name != "krita" {
		t.Errorf("unknown: %+v", u)
	}
}

func TestACommandSaysWhichProgramItRuns(t *testing.T) {
	swapFinding(t, nil, map[string]string{"git": "/usr/bin/git", "flatpak": "/usr/bin/flatpak", "snap": "/usr/bin/snap", "gtk-launch": "/usr/bin/gtk-launch"})

	cases := []struct {
		argv []string
		want programs.Use
	}{
		{[]string{"git", "status"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/git", Name: "git"}},
		{[]string{"env", "LANG=C", "git", "log"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/git", Name: "git"}},
		{[]string{"/opt/krita/bin/krita", "--export"}, programs.Use{Verb: programs.VerbChange, Path: "/opt/krita/bin/krita", Name: "krita"}},
		{[]string{"flatpak", "run", "--branch=stable", "org.gimp.GIMP", "-i"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.gimp.GIMP", Name: "flatpak"}},
		{[]string{"snap", "run", "chromium.chromedriver"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/snap", Snap: "chromium", Name: "snap"}},
		{[]string{"gtk-launch", "org.kde.krita.desktop"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/gtk-launch", Desktop: "org.kde.krita.desktop", Name: "gtk-launch"}},
		{[]string{"nothing-here"}, programs.Use{Verb: programs.VerbChange, Name: "nothing-here"}},
		{nil, programs.Use{Verb: programs.VerbChange}},
	}

	for _, c := range cases {
		raw, _ := json.Marshal(map[string]any{"argv": c.argv})

		if got, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); got != c.want {
			t.Errorf("%v: %+v, want %+v", c.argv, got, c.want)
		}
	}
}

func TestTypingAndClickingSayWhichWindowsProgram(t *testing.T) {
	w := windowProgram
	t.Cleanup(func() { windowProgram = w })

	var asked []any

	windowProgram = func(_ context.Context, atPoint bool, x, y int) (string, string) {
		asked = append(asked, atPoint, x, y)
		if atPoint {
			return "/snap/firefox/5091/usr/lib/firefox/firefox", ""
		}

		return "/usr/libexec/flatpak-bwrap", "org.gimp.GIMP"
	}

	u, _ := ProgramUseOf(context.Background(), TypeText{}, json.RawMessage(`{"text":"hello"}`))
	if u.Verb != programs.VerbInput || u.Flatpak != "org.gimp.GIMP" {
		t.Errorf("typing: %+v", u)
	}

	u, _ = ProgramUseOf(context.Background(), Click{}, json.RawMessage(`{"x":40,"y":50,"what":"OK"}`))
	if u.Verb != programs.VerbInput || u.Path != "/snap/firefox/5091/usr/lib/firefox/firefox" {
		t.Errorf("click: %+v", u)
	}

	if len(asked) != 6 || asked[0] != false || asked[3] != true || asked[4] != 40 || asked[5] != 50 {
		t.Errorf("asked %v", asked)
	}

	// Scrolling changes nothing and is not bound to a program.
	if _, ok := ProgramUseOf(context.Background(), Scroll{}, nil); ok {
		t.Error("scrolling was bound to a program")
	}
}

func TestAProcessIsReadForItsProgramAndFlatpak(t *testing.T) {
	proc := t.TempDir()
	dir := filepath.Join(proc, "4242")

	if err := os.MkdirAll(filepath.Join(dir, "root"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("/app/bin/gimp-2.10", filepath.Join(dir, "exe")); err != nil {
		t.Fatal(err)
	}

	info := "[Instance]\nname=not-this\n\n[Application]\nname=org.gimp.GIMP\nruntime=runtime/org.gnome.Platform\n"
	if err := os.WriteFile(filepath.Join(dir, "root", ".flatpak-info"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}

	if path, flatpak := processProgram(proc, 4242); path != "/app/bin/gimp-2.10" || flatpak != "org.gimp.GIMP" {
		t.Errorf("read %q %q", path, flatpak)
	}

	if path, flatpak := processProgram(proc, 1); path != "" || flatpak != "" {
		t.Errorf("a process that is not there read %q %q", path, flatpak)
	}
}

func TestTheWindowUnderAPointIsFoundFromTheTop(t *testing.T) {
	ids := stackingIDs("_NET_CLIENT_LIST_STACKING(WINDOW): window id # 0x4400004, 0x4600003, junk, 0x1a00003")
	if len(ids) != 3 || ids[0] != 0x4400004 || ids[2] != 0x1a00003 {
		t.Errorf("ids %v", ids)
	}

	if stackingIDs("_NET_CLIENT_LIST_STACKING:  not found.") != nil {
		t.Error("no list read as windows")
	}

	geo := "WINDOW=1\nX=100\nY=50\nWIDTH=200\nHEIGHT=100\nSCREEN=0\n"

	for _, c := range []struct {
		x, y int
		in   bool
	}{{100, 50, true}, {299, 149, true}, {300, 100, false}, {99, 60, false}, {150, 150, false}} {
		if geometryHolds(geo, c.x, c.y) != c.in {
			t.Errorf("%d,%d: holds %v", c.x, c.y, !c.in)
		}
	}

	if geometryHolds("X=0\nY=0\n", 0, 0) {
		t.Error("a window with no size held a point")
	}
}

// "Open Demo Notes" finds demo-notes.desktop by its file name with the
// spaces written as hyphens, and an entry whose file is named otherwise by
// the name the menu shows.
func TestAMenuProgramIsFoundByTheNameAPersonUses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	apps := filepath.Join(home, ".local", "share", "applications")
	os.MkdirAll(apps, 0o755)

	os.WriteFile(filepath.Join(apps, "zz-demo-notes.desktop"), []byte("[Desktop Entry]\nType=Application\nName=Demo Notes\nExec=true\n"), 0o644)
	os.WriteFile(filepath.Join(apps, "org.example.Qq7.desktop"), []byte("[Desktop Action x]\nName=Not This\n[Desktop Entry]\nName=Quiet Quill Seven\nExec=true\n"), 0o644)

	for name, want := range map[string]string{
		"zz-demo-notes":     "zz-demo-notes.desktop",
		"ZZ Demo Notes":     "zz-demo-notes.desktop",
		"quiet quill seven": "org.example.Qq7.desktop",
	} {
		if got, ok := desktopEntryFor(name); !ok || got != want {
			t.Errorf("%q found %q %v, want %q", name, got, ok, want)
		}
	}

	if got, ok := desktopEntryFor("not this zz9"); ok {
		t.Errorf("an action's name was taken for the entry's: %q", got)
	}

	if _, ok := desktopEntryFor("  "); ok {
		t.Error("an empty name found an entry")
	}
}
