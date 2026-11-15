package programs

import (
	"testing"
	"time"
)

/*
 * A card's "Always" is for the program on the card, not for anything that
 * shares its name.
 *
 * Found by the 1.1.0 security review. A menu entry that says "Exec=gimp"
 * was matched to any file called gimp, wherever it lay, so with "Always:
 * run it as a command" on GIMP's card a file called gimp in a downloaded
 * folder ran without a question, and its approval card would have said
 * "GIMP". A menu entry that starts its program through another one
 * (flatpak, python3, sh, env) was matched to every command that other
 * program runs. And a folder anywhere whose path merely contained
 * "flatpak/exports/bin" counted as the Flatpak of that name.
 */

func gimpWithAlways() ([]Record, map[int64][]Permit) {
	bare := rec(21, App, "gimp.desktop", "GIMP", "", true)
	bare.Exec = "gimp"

	full := rec(22, App, "inkscape.desktop", "Inkscape", "", true)
	full.Exec = "/usr/bin/inkscape"

	at := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

	return []Record{bare, full}, map[int64][]Permit{
		21: {{Verb: VerbChange, Answer: Always, At: at}, {Verb: VerbInput, Answer: Always, At: at}},
		22: {{Verb: VerbChange, Answer: Always, At: at}},
	}
}

func TestAlwaysIsNotGivenToASameNamedFileElsewhere(t *testing.T) {
	all, permits := gimpWithAlways()

	for _, c := range []struct {
		what string
		use  Use
	}{
		{"the program its name runs", Use{Verb: VerbChange, Path: "/usr/bin/gimp", Name: "gimp"}},
		{"a window of the program its name runs", Use{Verb: VerbInput, Path: "/usr/bin/gimp"}},
		{"the file the menu entry names in full", Use{Verb: VerbChange, Path: "/usr/bin/inkscape", Name: "inkscape"}},
	} {
		if r := Weigh(all, permits, c.use); r.Answer != Always || r.Program == nil {
			t.Errorf("%s lost its Always: %+v", c.what, r)
		}
	}

	for _, c := range []struct {
		what string
		use  Use
	}{
		{"a file called gimp in a downloaded folder", Use{Verb: VerbChange, Path: "/tmp/cloned-repo/gimp", Name: "gimp", Elsewhere: true}},
		{"a window of a file called gimp elsewhere", Use{Verb: VerbInput, Path: "/home/you/Downloads/gimp", Elsewhere: true}},
		{"a file called inkscape in another folder", Use{Verb: VerbChange, Path: "/tmp/x/inkscape", Name: "inkscape", Elsewhere: true}},
		{"a name alone, with no file found for it", Use{Verb: VerbChange, Name: "gimp"}},
		{"a name alone, in other letters", Use{Verb: VerbChange, Name: "GIMP"}},
	} {
		r := Weigh(all, permits, c.use)

		if r.Answer != AskEach || r.Why != WhyUndecided {
			t.Errorf("%s was answered by GIMP's card: %+v", c.what, r)
		}

		// The approval card must not call it by the real program's name.
		if r.Program != nil {
			t.Errorf("%s would be shown as %s", c.what, r.Program.Name)
		}
	}
}

func TestNeverStillReachesASameNamedFileElsewhere(t *testing.T) {
	all, _ := gimpWithAlways()
	never := map[int64][]Permit{21: {{Verb: VerbChange, Answer: Never}}}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/usr/bin/gimp", Name: "gimp"},
		{Verb: VerbChange, Path: "/tmp/cloned-repo/gimp", Name: "gimp", Elsewhere: true},
		{Verb: VerbChange, Name: "gimp"},
	} {
		if r := Weigh(all, never, use); r.Answer != Never || r.Why != WhyRefused {
			t.Errorf("a never did not refuse %+v: %+v", use, r)
		}
	}
}

func TestAProgramThatStartsOthersIsNotKnownByItsFile(t *testing.T) {
	gimp := rec(31, App, "org.gimp.GIMP.desktop", "GIMP", "", true)
	gimp.Exec = "/usr/bin/flatpak"

	script := rec(33, App, "tool.desktop", "Some Tool", "", true)
	script.Exec = "python3"

	shell := rec(34, App, "wrapped.desktop", "Wrapped", "", true)
	shell.Exec = "/bin/sh"

	all := []Record{gimp, rec(32, Flatpak, "org.gimp.GIMP", "GIMP", "2.10.38", true), script, shell}

	always := map[int64][]Permit{
		31: {{Verb: VerbChange, Answer: Always}},
		33: {{Verb: VerbChange, Answer: Always}},
		34: {{Verb: VerbChange, Answer: Always}},
	}

	if r := Weigh(all, always, Use{Verb: VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.gimp.GIMP", Name: "flatpak"}); r.Answer != Always {
		t.Errorf("GIMP run through flatpak lost its Always: %+v", r)
	}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.other.App", Name: "flatpak"},
		{Verb: VerbChange, Path: "/usr/bin/flatpak", Name: "flatpak"},
		{Verb: VerbChange, Path: "/usr/bin/python3", Name: "python3"},
		{Verb: VerbChange, Path: "/usr/bin/python3.12", Name: "python3.12"},
		{Verb: VerbChange, Path: "/bin/sh", Name: "sh"},
	} {
		if r := Weigh(all, always, use); r.Answer != AskEach || r.Program != nil {
			t.Errorf("%+v was answered by the card of a program that is only started through it: %+v", use, r)
		}
	}

	// And a never on such a card does not refuse everything else the
	// other program runs.
	never := map[int64][]Permit{31: {{Verb: VerbChange, Answer: Never}}, 33: {{Verb: VerbChange, Answer: Never}}}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.other.App", Name: "flatpak"},
		{Verb: VerbChange, Path: "/usr/bin/python3", Name: "python3"},
	} {
		if r := Weigh(all, never, use); r.Answer == Never {
			t.Errorf("%+v was refused by the card of another program: %+v", use, r)
		}
	}

	if r := Weigh(all, never, Use{Verb: VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.gimp.GIMP", Name: "flatpak"}); r.Answer != Never {
		t.Errorf("GIMP run through flatpak was not refused: %+v", r)
	}
}

func TestAFlatpakIsReadOnlyFromWhereFlatpaksAreInstalled(t *testing.T) {
	was := flatpakHomes
	t.Cleanup(func() { flatpakHomes = was })

	flatpakHomes = func() []string { return []string{"/var/lib/flatpak", "/home/you/.local/share/flatpak"} }

	for path, want := range map[string]string{
		"/var/lib/flatpak/exports/bin/org.x.Y":                              "org.x.Y",
		"/home/you/.local/share/flatpak/exports/bin/org.x.Y":                "org.x.Y",
		"/home/you/.local/share/flatpak/app/org.x.Y/x86_64/stable/a/bin/y":  "org.x.Y",
		"/tmp/cloned-repo/flatpak/exports/bin/org.x.Y":                      "",
		"/home/you/Downloads/var/lib/flatpak/exports/bin/org.x.Y":           "",
		"/home/other/.local/share/flatpak/exports/bin/org.x.Y":              "",
		"/tmp/x/flatpak/app/org.x.Y/x86_64/stable/a/files/bin/y":            "",
		"/var/lib/flatpak/exports/bin/../../../../../tmp/x/exports/bin/o.Y": "",
	} {
		if got := flatpakOfPath(path); got != want {
			t.Errorf("%s: read as the Flatpak %q, want %q", path, got, want)
		}
	}
}

/*
 * A program reached through something that wraps it earns no "always".
 *
 * Found by the second pass of the 1.1.0 security review: once a command
 * was read through env or a shell to name its program, that program's
 * "Always" ran the whole line (sh -c "gimp; anything else", env
 * LD_PRELOAD=... gimp). A wrapped use can be refused by the program's
 * card and is never allowed by it, whichever clue names the program.
 */
func TestAWrappedUseIsRefusedByACardAndNeverAllowedByIt(t *testing.T) {
	gimp := rec(41, App, "gimp.desktop", "GIMP", "", true)
	gimp.Exec = "gimp"

	entry := rec(43, App, "org.gimp.GIMP.desktop", "GIMP", "", true)
	entry.Exec = "/usr/bin/flatpak"

	fox := rec(45, App, "firefox_firefox.desktop", "Firefox", "", true)
	fox.Exec = "/snap/bin/firefox"

	all := []Record{gimp, rec(42, Deb, "gimp", "gimp", "2.10", true), entry,
		rec(44, Flatpak, "org.gimp.GIMP", "GIMP", "2.10.38", true), fox, rec(46, Snap, "firefox", "Firefox", "132.0", true)}

	always, never := map[int64][]Permit{}, map[int64][]Permit{}

	for _, r := range all {
		always[r.RowID] = []Permit{{Verb: VerbChange, Answer: Always, Version: r.Version}}
		never[r.RowID] = []Permit{{Verb: VerbChange, Answer: Never}}
	}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/usr/bin/gimp", Name: "gimp", Wrapped: true},
		{Verb: VerbChange, Path: "/usr/bin/gimp", Package: "gimp", Name: "gimp", Wrapped: true},
		{Verb: VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.gimp.GIMP", Name: "flatpak", Wrapped: true},
		{Verb: VerbChange, Path: "/usr/bin/snap", Snap: "firefox", Name: "snap", Wrapped: true},
		{Verb: VerbChange, Path: "/snap/bin/firefox", Name: "firefox", Wrapped: true},
		{Verb: VerbChange, Desktop: "gimp.desktop", Name: "gtk-launch", Wrapped: true},
	} {
		if r := Weigh(all, always, use); r.Answer != AskEach || r.Program != nil {
			t.Errorf("wrapped, %+v was allowed or named by a card: %+v", use, r)
		}

		if r := Weigh(all, never, use); r.Answer != Never {
			t.Errorf("wrapped, %+v was not refused by a never: %+v", use, r)
		}

		// Not wrapped, the same clues are the program itself.
		use.Wrapped = false

		if r := Weigh(all, always, use); r.Answer != Always {
			t.Errorf("not wrapped, %+v lost its Always: %+v", use, r)
		}
	}
}

/*
 * A menu entry that names its program file in full is that file, wherever
 * the bare name leads: Code's entry says /usr/share/code/code while
 * "code" on the path is a small starter script. The tool calls that file
 * elsewhere (it is not what the name runs), and it is still exactly the
 * card's program.
 */
func TestTheFileAMenuEntryNamesInFullIsItsProgram(t *testing.T) {
	code := rec(51, App, "code.desktop", "Code", "", true)
	code.Exec = "/usr/share/code/code"

	always := map[int64][]Permit{51: {{Verb: VerbChange, Answer: Always}, {Verb: VerbInput, Answer: Always}}}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/usr/share/code/code", Name: "code", Elsewhere: true},
		{Verb: VerbInput, Path: "/usr/share/code/code", Elsewhere: true},
	} {
		if r := Weigh([]Record{code}, always, use); r.Answer != Always || r.Program == nil {
			t.Errorf("%+v is the file the menu entry names and lost its Always: %+v", use, r)
		}
	}

	// A menu entry with a place given from some folder ("bin/tool") is
	// not a file anybody can point at: a command with the same words is
	// whatever lies in the folder it runs in.
	odd := rec(52, App, "tool.desktop", "Tool", "", true)
	odd.Exec = "bin/tool"

	if r := Weigh([]Record{odd}, map[int64][]Permit{52: {{Verb: VerbChange, Answer: Always}}},
		Use{Verb: VerbChange, Path: "bin/tool", Name: "tool", Elsewhere: true}); r.Answer != AskEach || r.Program != nil {
		t.Errorf("a place given from a folder was answered by a card: %+v", r)
	}

	for _, use := range []Use{
		{Verb: VerbChange, Path: "/tmp/x/code", Name: "code", Elsewhere: true},
		{Verb: VerbChange, Path: "/usr/share/code/code", Name: "code", Elsewhere: true, Wrapped: true},
	} {
		if r := Weigh([]Record{code}, always, use); r.Answer != AskEach || r.Program != nil {
			t.Errorf("%+v was answered by Code's card: %+v", use, r)
		}
	}
}
