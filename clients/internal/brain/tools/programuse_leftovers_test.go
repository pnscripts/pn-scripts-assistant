package tools

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * Leftover of the 1.1.0 security review: a "never" was stepped around by
 * the wrappers nobody had listed (sudo, doas, pkexec, xargs, busybox sh
 * -c, script -c, systemd-run, eval), and by anything after the first
 * command of a shell line or inside a one-line program (python3 -c).
 */
func TestMoreWrappersAreSeenThrough(t *testing.T) {
	swapFinding(t, nil, map[string]string{
		"firefox": "/usr/bin/firefox", "sudo": "/usr/bin/sudo", "xargs": "/usr/bin/xargs", "busybox": "/usr/bin/busybox",
		"script": "/usr/bin/script", "systemd-run": "/usr/bin/systemd-run", "sh": "/bin/sh", "doas": "/usr/bin/doas",
		"pkexec": "/usr/bin/pkexec", "env": "/usr/bin/env",
	})

	firefox := programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/firefox", Name: "firefox", Wrapped: true}

	for _, argv := range [][]string{
		{"sudo", "firefox"},
		{"sudo", "-u", "bob", "-E", "firefox", "--new-window"},
		{"sudo", "--user=bob", "-n", "--", "firefox"},
		{"sudo", "-g", "staff", "A=1", "firefox"},
		{"doas", "-u", "bob", "firefox"},
		{"pkexec", "--user", "bob", "firefox"},
		{"xargs", "firefox"},
		{"xargs", "-n", "1", "-I", "{}", "firefox", "{}"},
		{"xargs", "-0", "--max-args=1", "-P", "4", "firefox"},
		{"busybox", "sh", "-c", "firefox"},
		{"busybox", "nohup", "firefox"},
		{"script", "-c", "firefox", "/dev/null"},
		{"script", "-q", "-c", "firefox --new-window", "/dev/null"},
		{"script", "-qc", "firefox", "/dev/null"},
		{"script", "--command=firefox", "--quiet"},
		{"systemd-run", "firefox"},
		{"systemd-run", "--user", "--unit", "x", "-p", "MemoryMax=1G", "--setenv=A=1", "firefox"},
		{"systemd-run", "--scope", "-E", "A=1", "--", "firefox", "-P"},
		{"sh", "-c", "eval firefox"},
		{"sh", "-c", `eval "firefox --new-window"`},
		{"sh", "-c", "builtin eval exec firefox"},
		{"sudo", "env", "A=1", "busybox", "sh", "-c", "eval firefox"},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": argv})

		if got, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); got != firefox {
			t.Errorf("%s: %+v, want %+v", strings.Join(argv, " "), got, firefox)
		}
	}

	// A wrapper with nothing to run names nothing; one that reads its
	// command from somewhere else is itself.
	for _, c := range []struct {
		argv []string
		want programs.Use
	}{
		{[]string{"sudo", "-v"}, programs.Use{Verb: programs.VerbChange}},
		{[]string{"script", "/tmp/typescript"}, programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/script", Name: "script"}},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": c.argv})

		if got, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); got != c.want {
			t.Errorf("%s: %+v, want %+v", strings.Join(c.argv, " "), got, c.want)
		}
	}
}

func TestEveryCommandOfALineIsSaid(t *testing.T) {
	swapFinding(t, nil, map[string]string{
		"firefox": "/usr/bin/firefox", "gimp": "/usr/bin/gimp", "curl": "/usr/bin/curl", "sh": "/bin/sh", "echo": "/usr/bin/echo",
		"python3": "/usr/bin/python3", "flatpak": "/usr/bin/flatpak", "sudo": "/usr/bin/sudo", "node": "/usr/bin/node",
		"perl": "/usr/bin/perl", "os": "/usr/bin/os",
	})

	// A file named in code is looked at; here, the ones of this test.
	stat := statProgram
	t.Cleanup(func() { statProgram = stat })

	statProgram = func(path string) (os.FileInfo, error) {
		if path == "/usr/bin/gimp" {
			return os.Stat(os.Args[0])
		}

		return nil, os.ErrNotExist
	}

	named := func(argv ...string) []string {
		t.Helper()

		raw, _ := json.Marshal(map[string]any{"argv": argv})

		var out []string

		for _, u := range AlsoRunsOf(context.Background(), RunCommand{}, raw) {
			if !u.Wrapped || u.Verb != programs.VerbChange {
				t.Errorf("%v: %+v can allow, or is another use", argv, u)
			}

			name := u.Name
			if u.Flatpak != "" {
				name += "=" + u.Flatpak
			}

			out = append(out, name)
		}

		sort.Strings(out)

		return out
	}

	for _, c := range []struct {
		argv []string
		want string
	}{
		{[]string{"sh", "-c", "echo hi; firefox"}, "echo firefox"},
		{[]string{"sh", "-c", "echo hi && A=1 firefox --x || gimp"}, "echo firefox gimp"},
		{[]string{"bash", "-c", "echo $(firefox --version) | gimp -"}, "echo firefox gimp"},
		{[]string{"sh", "-c", "echo hi\nexec firefox"}, "echo firefox"},
		{[]string{"sh", "-c", "if true; then firefox; fi"}, "firefox true"},
		{[]string{"sh", "-c", "echo `firefox`; (gimp) & { curl x; }"}, "curl echo firefox gimp"},
		{[]string{"sh", "-c", "echo hi; sh -c 'echo there; firefox'"}, "echo firefox"},
		{[]string{"sh", "-c", "echo 'not; firefox' \"nor; gimp\""}, "echo"},
		{[]string{"sh", "-c", "echo \"$(firefox)\"; eval 'true; gimp'"}, "echo firefox gimp true"},
		{[]string{"sh", "-c", "echo hi; sudo nohup firefox"}, "echo firefox"},
		{[]string{"sh", "-c", "echo hi; flatpak run org.gimp.GIMP"}, "echo flatpak=org.gimp.GIMP"},
		{[]string{"sh", "-c", "echo hi > /tmp/firefox"}, "echo"},
		{[]string{"sudo", "sh", "-c", "echo; firefox"}, "echo firefox"},
		// A one-line program: every word of it that is a program here.
		{[]string{"python3", "-c", "import os; os.system('firefox --new-window')"}, "firefox os"},
		{[]string{"python3", "-c", "import subprocess; subprocess.run(['/usr/bin/gimp', '-i'])"}, "gimp"},
		{[]string{"node", "-e", "require('child_process').execSync(\"firefox\")"}, "firefox"},
		{[]string{"perl", "-e", "exec 'firefox'"}, "firefox"},
		// A plain command says nothing more than itself.
		{[]string{"firefox", "--new-window"}, ""},
		{[]string{"sh", "script.sh"}, ""},
		{[]string{"python3", "tool.py", "firefox"}, ""},
	} {
		if got := strings.Join(named(c.argv...), " "); got != c.want {
			t.Errorf("%s: %q, want %q", strings.Join(c.argv, " "), got, c.want)
		}
	}

	// As administrator too.
	raw, _ := json.Marshal(map[string]any{"argv": []string{"sh", "-c", "echo; firefox"}, "reason": "x"})
	if got := AlsoRunsOf(context.Background(), RunAsAdmin{}, raw); len(got) != 2 {
		t.Errorf("as administrator: %+v", got)
	}

	// Bounded: a line of a thousand commands is not a thousand lookups.
	long := strings.Repeat("firefox; gimp; curl; ", 400)
	raw, _ = json.Marshal(map[string]any{"argv": []string{"sh", "-c", long}})

	if got := AlsoRunsOf(context.Background(), RunCommand{}, raw); len(got) != 3 {
		t.Errorf("a long line of three programs: %d uses", len(got))
	}

	// A tool that uses no program says none.
	if got := AlsoRunsOf(context.Background(), Scroll{}, json.RawMessage(`{}`)); got != nil {
		t.Errorf("scroll: %+v", got)
	}
}
