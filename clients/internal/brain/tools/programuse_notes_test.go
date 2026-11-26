package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * Notes of the read after 1.2.0 (ASSISTANT-PROGRAM-USE-NOTES-1121).
 *
 * 1. open_app given a place ({"name":"/tmp/gimp"}) looked it up as a
 *    path and never said it was elsewhere, so a card that matches by file
 *    name (GIMP's, whose entry says "gimp") lent it its "always", and the
 *    question was headed with GIMP's name.
 * 2. When gtk-launch was missing or failed, open_app fell through to the
 *    first word as a command, which was never ruled on: "text editor",
 *    ruled as the Text Editor's menu entry, ran a program called "text".
 * 6. Wrappers a command was not read through: a "never" did not reach
 *    what su -c, runuser, taskset, chrt, flock, strace, watch, unshare,
 *    nsenter, chroot, dbus-run-session, xvfb-run, find -exec, a terminal's
 *    -e, cmd /c or powershell -Command run.
 */

// swapStarting records what open_app starts instead of starting it.
func swapStarting(t *testing.T, fail error) *[][]string {
	t.Helper()

	s := startApp
	t.Cleanup(func() { startApp = s })

	var started [][]string

	startApp = func(path string, args ...string) error {
		started = append(started, append([]string{path}, args...))

		return fail
	}

	return &started
}

func TestOpeningAPlaceIsNotTheProgramOfThatName(t *testing.T) {
	swapFinding(t, map[string]string{"gimp": "gimp.desktop"}, map[string]string{"gimp": "/usr/bin/gimp", "/tmp/gimp": "/tmp/gimp"})

	gimp := programs.Record{Program: programs.Program{Kind: programs.App, ID: "gimp.desktop", Name: "GIMP", Exec: "gimp"},
		RowID: 3, Present: true, Pin: "pin:aaaa"}
	permits := map[int64][]programs.Permit{3: {{Verb: programs.VerbStart, Answer: programs.Always, Version: "pin:aaaa"}}}

	for _, name := range []string{"/tmp/gimp", "./gimp", "../Downloads/gimp", `C:\Users\you\Downloads\gimp.exe`} {
		raw, _ := json.Marshal(map[string]string{"name": name})

		u, _ := ProgramUseOf(context.Background(), OpenApp{}, raw)
		if u.Desktop != "" || !u.Elsewhere || u.Path == "" {
			t.Errorf("%s: %+v, want a file elsewhere", name, u)
		}

		ruling := programs.Rule(programs.Match([]programs.Record{gimp}, u), permits, programs.VerbStart)
		if ruling.Answer == programs.Always {
			t.Errorf("%s borrowed GIMP's \"always\": %+v", name, ruling)
		}

		// Never started: open_app opens programs by name.
		started := swapStarting(t, nil)

		if _, err := (OpenApp{}).Execute(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "run_command") {
			t.Errorf("%s: opened, or refused without saying what to use: %v", name, err)
		}

		if len(*started) != 0 {
			t.Errorf("%s started %v", name, *started)
		}
	}

	// GIMP's "never" still reaches a file elsewhere that shares its name.
	never := map[int64][]programs.Permit{3: {{Verb: programs.VerbStart, Answer: programs.Never}}}
	u, _ := ProgramUseOf(context.Background(), OpenApp{}, json.RawMessage(`{"name":"/tmp/gimp"}`))

	if r := programs.Rule(programs.Match([]programs.Record{gimp}, u), never, programs.VerbStart); r.Answer != programs.Never {
		t.Errorf("GIMP's \"never\" does not reach /tmp/gimp: %+v", r)
	}

	// By name, as before.
	u, _ = ProgramUseOf(context.Background(), OpenApp{}, json.RawMessage(`{"name":"gimp"}`))
	if r := programs.Rule(programs.Match([]programs.Record{gimp}, u), permits, programs.VerbStart); r.Answer != programs.Always {
		t.Errorf("GIMP by name lost its \"always\": %+v %+v", u, r)
	}
}

func TestAMenuEntryThatCannotStartIsNotRunAsACommand(t *testing.T) {
	entries := map[string]string{"text editor": "org.gnome.TextEditor.desktop"}

	// No gtk-launch here: the first word is a program, and it is not run.
	swapFinding(t, entries, map[string]string{"text": "/usr/bin/text"})

	started := swapStarting(t, nil)

	if _, err := (OpenApp{}).Execute(context.Background(), json.RawMessage(`{"name":"text editor"}`)); err == nil {
		t.Error("opened with no way to start the menu entry")
	}

	if len(*started) != 0 {
		t.Errorf("ran %v, which was never ruled on", *started)
	}

	// gtk-launch is there and fails: still nothing else is run.
	swapFinding(t, entries, map[string]string{"text": "/usr/bin/text", "gtk-launch": "/usr/bin/gtk-launch"})

	started = swapStarting(t, errors.New("could not start"))

	if _, err := (OpenApp{}).Execute(context.Background(), json.RawMessage(`{"name":"text editor"}`)); err == nil {
		t.Error("said opened when gtk-launch failed")
	}

	if want := [][]string{{"/usr/bin/gtk-launch", "--", "org.gnome.TextEditor.desktop"}}; !reflect.DeepEqual(*started, want) {
		t.Errorf("started %v, want only %v", *started, want)
	}

	// gtk-launch starts it: the entry after "--", so a name like an
	// option is never read as one.
	started = swapStarting(t, nil)

	if out, err := (OpenApp{}).Execute(context.Background(), json.RawMessage(`{"name":"text editor"}`)); err != nil || out != "Opened text editor" {
		t.Errorf("%q, %v", out, err)
	}

	if len(*started) != 1 || (*started)[0][1] != "--" {
		t.Errorf("started %v", *started)
	}

	// No menu entry: the command, as before; what ProgramUse names.
	swapFinding(t, nil, map[string]string{"gedit": "/usr/bin/gedit"})

	started = swapStarting(t, nil)

	if _, err := (OpenApp{}).Execute(context.Background(), json.RawMessage(`{"name":"Gedit"}`)); err != nil {
		t.Fatal(err)
	}

	if want := [][]string{{"/usr/bin/gedit"}}; !reflect.DeepEqual(*started, want) {
		t.Errorf("started %v, want %v", *started, want)
	}
}

func powershellEncoded(s string) string {
	var b []byte

	for _, r := range utf16.Encode([]rune(s)) {
		b = append(b, byte(r), byte(r>>8))
	}

	return base64.StdEncoding.EncodeToString(b)
}

func TestTheWrappersOfTheReadAfter120AreSeenThrough(t *testing.T) {
	swapFinding(t, nil, map[string]string{"gimp": "/usr/bin/gimp", "rm": "/usr/bin/rm"})

	cases := []struct {
		argv []string
		name string
	}{
		{[]string{"su", "-c", "gimp --batch x", "you"}, "gimp"},
		{[]string{"su", "-", "you", "-c", "gimp"}, "gimp"},
		{[]string{"su", "--command=gimp -i"}, "gimp"},
		{[]string{"runuser", "-u", "you", "--", "gimp", "-i"}, "gimp"},
		{[]string{"runuser", "-l", "you", "-c", "gimp"}, "gimp"},
		{[]string{"taskset", "0x3", "gimp"}, "gimp"},
		{[]string{"taskset", "-c", "0-2", "gimp"}, "gimp"},
		{[]string{"chrt", "-f", "10", "gimp"}, "gimp"},
		{[]string{"chrt", "--idle", "0", "gimp"}, "gimp"},
		{[]string{"flock", "-w", "5", "/tmp/l", "gimp", "-i"}, "gimp"},
		{[]string{"flock", "/tmp/l", "-c", "gimp -i"}, "gimp"},
		{[]string{"strace", "-f", "-o", "/tmp/t", "-e", "trace=file", "gimp"}, "gimp"},
		{[]string{"watch", "-n", "2", "gimp", "-i"}, "gimp"},
		{[]string{"watch", "-x", "gimp", "-i"}, "gimp"},
		{[]string{"unshare", "-n", "gimp"}, "gimp"},
		{[]string{"unshare", "--net", "-S", "0", "--", "gimp"}, "gimp"},
		{[]string{"nsenter", "-t", "1", "-m", "gimp"}, "gimp"},
		{[]string{"chroot", "--userspec=you", "/", "gimp"}, "gimp"},
		{[]string{"dbus-run-session", "--", "gimp"}, "gimp"},
		{[]string{"dbus-run-session", "--config-file=/tmp/c", "gimp"}, "gimp"},
		{[]string{"xvfb-run", "-a", "-s", "-screen 0 800x600x24", "gimp"}, "gimp"},
		{[]string{"find", ".", "-name", "*.xcf", "-exec", "gimp", "{}", ";"}, "gimp"},
		{[]string{"find", "/", "-execdir", "rm", "{}", "+"}, "rm"},
		{[]string{"gnome-terminal", "--", "gimp", "-i"}, "gimp"},
		{[]string{"xterm", "-e", "gimp", "-i"}, "gimp"},
		{[]string{"xfce4-terminal", "-e", "gimp -i"}, "gimp"},
		{[]string{"konsole", "--noclose", "-e", "gimp"}, "gimp"},
		{[]string{"x-terminal-emulator", "-e", "gimp"}, "gimp"},
		{[]string{"alacritty", "--command", "gimp"}, "gimp"},
		{[]string{"cmd", "/c", "gimp", "--batch"}, "gimp"},
		{[]string{"cmd.exe", "/D", "/S", "/C", "gimp & rm x"}, "gimp"},
		{[]string{"powershell", "-NoProfile", "-Command", "gimp; rm x"}, "gimp"},
		{[]string{"pwsh", "-c", "gimp"}, "gimp"},
		{[]string{"powershell.exe", "-ExecutionPolicy", "Bypass", "-EncodedCommand", powershellEncoded("gimp -i")}, "gimp"},
	}

	for _, c := range cases {
		raw, _ := json.Marshal(map[string]any{"argv": c.argv})

		u, _ := ProgramUseOf(context.Background(), RunCommand{}, raw)
		if u.Name != c.name || !u.Wrapped {
			t.Errorf("%q: %+v, want %s, wrapped", c.argv, u, c.name)
		}
	}

	// Every command of a line handed over is reached, not only the first.
	for _, argv := range [][]string{
		{"su", "-c", "true; rm -rf x"},
		{"flock", "/tmp/l", "-c", "true && rm x"},
		{"watch", "true; rm x"},
		{"cmd", "/c", "echo hi & rm x"},
		{"powershell", "-Command", "Write-Host hi; rm x"},
		{"xfce4-terminal", "-e", "sh -c 'true; rm x'"},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": argv})

		found := false

		for _, u := range AlsoRunsOf(context.Background(), RunCommand{}, raw) {
			if u.Name == "rm" && u.Wrapped {
				found = true
			}
		}

		if !found {
			t.Errorf("%q: rm is not among what it also runs", argv)
		}
	}

	// A "never" on a program is reached through each of them.
	rm := programs.Record{Program: programs.Program{Kind: programs.App, ID: "rm.desktop", Name: "rm", Exec: "rm"},
		RowID: 9, Present: true}
	never := map[int64][]programs.Permit{9: {{Verb: programs.VerbChange, Answer: programs.Never}}}

	raw, _ := json.Marshal(map[string]any{"argv": []string{"taskset", "1", "rm", "-rf", "x"}})
	u, _ := ProgramUseOf(context.Background(), RunCommand{}, raw)

	if r := programs.Rule(programs.Match([]programs.Record{rm}, u), never, programs.VerbChange); r.Answer != programs.Never {
		t.Errorf("a never is not seen through taskset: %+v", r)
	}

	// Nothing to run: no program is named, and nothing is borrowed.
	for _, argv := range [][]string{{"taskset", "-p", "1", "1234"}, {"su"}, {"chroot", "/srv/x"}, {"flock", "9"}} {
		raw, _ := json.Marshal(map[string]any{"argv": argv})

		if u, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); u.Name != "" && u.Name != argv[0] {
			t.Errorf("%q named %+v", argv, u)
		}
	}
}

/*
 * Found by the review of the fix: read through, a wrapper was no longer
 * named at all, so a "never" on its own card (find, a terminal,
 * powershell, su), which refused the call when the wrapper was read as
 * the program, refused nothing. And strace's -A and -n take no value,
 * ltrace's -l does.
 */
func TestANeverOnTheWrapperItselfStillRefuses(t *testing.T) {
	swapFinding(t, nil, map[string]string{"rm": "/usr/bin/rm", "find": "/usr/bin/find", "gnome-terminal": "/usr/bin/gnome-terminal",
		"powershell": "/usr/bin/powershell", "su": "/usr/bin/su", "cmd": "/usr/bin/cmd", "taskset": "/usr/bin/taskset", "bash": "/usr/bin/bash"})

	card := func(name string) programs.Record {
		return programs.Record{Program: programs.Program{Kind: programs.App, ID: name + ".desktop", Name: name, Exec: name},
			RowID: 40, Present: true}
	}
	never := map[int64][]programs.Permit{40: {{Verb: programs.VerbChange, Answer: programs.Never}}}

	for wrapper, argv := range map[string][]string{
		"find":           {"find", ".", "-exec", "rm", "{}", ";"},
		"gnome-terminal": {"gnome-terminal", "--", "bash"},
		"powershell":     {"powershell", "-Command", "rm x"},
		"su":             {"su", "-c", "rm x"},
		"cmd":            {"cmd", "/c", "rm x"},
		"taskset":        {"taskset", "1", "rm", "x"},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": argv})

		refused := false

		uses := append([]programs.Use{}, AlsoRunsOf(context.Background(), RunCommand{}, raw)...)
		if u, ok := ProgramUseOf(context.Background(), RunCommand{}, raw); ok {
			uses = append(uses, u)
		}

		for _, u := range uses {
			if programs.Rule(programs.Match([]programs.Record{card(wrapper)}, u), never, programs.VerbChange).Answer == programs.Never {
				refused = true
			}

			if u.Name == wrapper && !u.Wrapped {
				t.Errorf("%q: the wrapper %s is named as one that may be allowed: %+v", argv, wrapper, u)
			}
		}

		if !refused {
			t.Errorf("%q: a never on %s does not refuse it: %+v", argv, wrapper, uses)
		}
	}

	// A plain command still says nothing more.
	raw, _ := json.Marshal(map[string]any{"argv": []string{"rm", "x"}})
	if more := AlsoRunsOf(context.Background(), RunCommand{}, raw); len(more) != 0 {
		t.Errorf("a plain command also runs %+v", more)
	}
}

func TestTracersOptionsAreReadAsTheyAre(t *testing.T) {
	swapFinding(t, nil, map[string]string{"rm": "/usr/bin/rm"})

	for _, argv := range [][]string{
		{"strace", "-A", "-o", "log", "rm", "x"},
		{"strace", "-n", "rm", "x"},
		{"strace", "--quiet", "rm", "x"},
		{"strace", "--decode-fds", "rm", "x"},
		{"ltrace", "-l", "libfoo.so", "rm", "x"},
		{"ltrace", "-A", "5", "-n", "2", "rm", "x"},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": argv})

		if u, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); u.Name != "rm" || !u.Wrapped {
			t.Errorf("%q: %+v, want rm", argv, u)
		}
	}
}
