package programs

import (
	"strings"
	"testing"
)

func TestAMenuEntryIsReadAsTheProgramItStarts(t *testing.T) {
	e := ParseDesktop("org.gimp.GIMP.desktop", strings.NewReader(`# a comment
[Desktop Entry]
Type=Application
Name=GNU Image Manipulation Program
Name[bg]=Програма за обработка на изображения GNU
GenericName=Image Editor
Exec=env GDK_BACKEND=x11 gimp-2.10 %U
TryExec=gimp-2.10
Icon=gimp

[Desktop Action new]
Name=New window
Exec=something-else --new
`))

	if !e.Listed() {
		t.Fatalf("an ordinary application is listed: %+v", e)
	}

	p := e.Program
	if p.Kind != App || p.ID != "org.gimp.GIMP.desktop" {
		t.Errorf("kind/id %q %q", p.Kind, p.ID)
	}

	if p.Name != "GNU Image Manipulation Program" {
		t.Errorf("name %q: the untranslated Name, never a localised one or an action's", p.Name)
	}

	if p.Exec != "gimp-2.10" {
		t.Errorf("exec %q: the program after env and its settings, without field codes", p.Exec)
	}

	if e.TryExec != "gimp-2.10" {
		t.Errorf("tryexec %q", e.TryExec)
	}
}

func TestQuotedProgramsAndFieldCodesAreUnderstood(t *testing.T) {
	cases := map[string]string{
		`Exec="/opt/My App/bin/app" --flag %f`:              "/opt/My App/bin/app",
		`Exec=/snap/bin/firefox %u`:                         "/snap/bin/firefox",
		`Exec=env A=1 B=2 /usr/bin/thing`:                   "/usr/bin/thing",
		`Exec=/usr/bin/flatpak run --branch=stable org.x.Y`: "/usr/bin/flatpak",
		`Exec="quote\\"d" %F`:                               `quote"d`,
	}

	for line, want := range cases {
		e := ParseDesktop("x.desktop", strings.NewReader("[Desktop Entry]\nType=Application\nName=X\n"+line+"\n"))
		if e.Program.Exec != want {
			t.Errorf("%s: exec %q, want %q", line, e.Program.Exec, want)
		}
	}
}

func TestEntriesThatAreNotProgramsInTheMenuAreNotListed(t *testing.T) {
	cases := map[string]string{
		"hidden":       "[Desktop Entry]\nType=Application\nName=X\nExec=x\nHidden=true\n",
		"no display":   "[Desktop Entry]\nType=Application\nName=X\nExec=x\nNoDisplay=true\n",
		"a link":       "[Desktop Entry]\nType=Link\nName=X\nURL=x\n",
		"no name":      "[Desktop Entry]\nType=Application\nExec=x\n",
		"no exec":      "[Desktop Entry]\nType=Application\nName=X\n",
		"no group":     "Type=Application\nName=X\nExec=x\n",
		"other group":  "[Something]\nType=Application\nName=X\nExec=x\n",
		"empty":        "",
		"binary noise": "\x00\x01\x02[Desktop Entry\n",
	}

	for name, text := range cases {
		if e := ParseDesktop("x.desktop", strings.NewReader(text)); e.Listed() {
			t.Errorf("%s: listed %+v", name, e)
		}
	}

	// Hidden is remembered even though it is not listed: it masks the same
	// entry further down the search path.
	if e := ParseDesktop("x.desktop", strings.NewReader(cases["hidden"])); !e.Hidden {
		t.Error("Hidden=true must be kept")
	}

	// D-Bus activated programs have no Exec and are still programs.
	e := ParseDesktop("org.gnome.Calculator.desktop", strings.NewReader(
		"[Desktop Entry]\nType=Application\nName=Calculator\nDBusActivatable=true\n"))
	if !e.Listed() {
		t.Error("a D-Bus activatable entry without Exec is a program")
	}
}

func TestAHugeLineDoesNotStopTheRest(t *testing.T) {
	text := "[Desktop Entry]\nComment=" + strings.Repeat("a", 300_000) + "\nType=Application\nName=Big\nExec=big\n"

	e := ParseDesktop("big.desktop", strings.NewReader(text))
	if !e.Listed() || e.Program.Name != "Big" {
		t.Fatalf("a long line must be skipped, not end the reading: %+v", e)
	}
}
