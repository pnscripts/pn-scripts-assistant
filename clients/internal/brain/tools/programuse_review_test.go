package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * A command says which program it runs, however it is dressed.
 *
 * Found by the 1.1.0 security review. "env -i firefox", "nohup firefox",
 * "sh -c firefox" and "flatpak run --branch stable org.mozilla.firefox"
 * each named something else than Firefox (an option, the wrapper, the
 * branch), so a "never" on Firefox's card did not refuse them. And a file
 * that only shares a program's name, given by its place, was taken for the
 * program: it is now said to be elsewhere, so no "always" answers for it.
 */
func TestACommandIsSeenThroughWhatWrapsIt(t *testing.T) {
	swapFinding(t, nil, map[string]string{
		"firefox": "/usr/bin/firefox", "flatpak": "/usr/bin/flatpak", "snap": "/usr/bin/snap",
		"env": "/usr/bin/env", "sh": "/bin/sh", "nohup": "/usr/bin/nohup", "timeout": "/usr/bin/timeout",
	})

	firefox := programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/firefox", Name: "firefox"}
	flatpak := programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/flatpak", Flatpak: "org.mozilla.firefox", Name: "flatpak"}
	snap := programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/snap", Snap: "firefox", Name: "snap"}

	for _, c := range []struct {
		argv []string
		want programs.Use
	}{
		{[]string{"env", "-i", "firefox"}, firefox},
		{[]string{"env", "-u", "DISPLAY", "firefox"}, firefox},
		{[]string{"env", "--unset=DISPLAY", "-C", "/tmp", "A=1", "firefox", "--new-window"}, firefox},
		{[]string{"/usr/bin/env", "-", "A=1", "--", "firefox"}, firefox},
		{[]string{"env", "-S", "A=1 firefox --private-window"}, firefox},
		{[]string{"env", "--split-string=firefox -P x"}, firefox},
		{[]string{"nohup", "firefox"}, firefox},
		{[]string{"setsid", "-f", "firefox"}, firefox},
		{[]string{"nice", "-n", "10", "firefox"}, firefox},
		{[]string{"nice", "-10", "firefox"}, firefox},
		{[]string{"ionice", "-c", "3", "firefox"}, firefox},
		{[]string{"timeout", "-k", "5", "--signal=KILL", "30s", "firefox"}, firefox},
		{[]string{"stdbuf", "-oL", "-e", "0", "firefox"}, firefox},
		{[]string{"sh", "-c", "firefox"}, firefox},
		{[]string{"bash", "-lc", "A=1 firefox --new-window; echo done"}, firefox},
		{[]string{"/bin/sh", "-c", "exec 'firefox' && true"}, firefox},
		{[]string{"nohup", "env", "A=1", "sh", "-c", "nice firefox"}, firefox},
		{[]string{"flatpak", "run", "--branch", "stable", "org.mozilla.firefox"}, flatpak},
		{[]string{"flatpak", "--user", "run", "--command", "sh", "--env=A=1", "org.mozilla.firefox", "-c", "x"}, flatpak},
		{[]string{"env", "-i", "flatpak", "run", "--arch", "x86_64", "org.mozilla.firefox"}, flatpak},
		{[]string{"snap", "run", "--hook", "configure", "firefox"}, snap},
		{[]string{"snap", "run", "-r", "12", "firefox.geckodriver"}, snap},
		// A shell running a script is the shell; nothing is guessed.
		{[]string{"sh", "script.sh"}, programs.Use{Verb: programs.VerbChange, Path: "/bin/sh", Name: "sh"}},
		// Only what wraps is left: nothing to run.
		{[]string{"env", "-i"}, programs.Use{Verb: programs.VerbChange}},
		{[]string{"sh", "-c"}, programs.Use{Verb: programs.VerbChange, Path: "/bin/sh", Name: "sh"}},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": c.argv})

		if got, _ := ProgramUseOf(context.Background(), RunCommand{}, raw); got != c.want {
			t.Errorf("%s: %+v, want %+v", strings.Join(c.argv, " "), got, c.want)
		}
	}
}

func TestAFileThatOnlySharesAProgramsNameIsSaidToBeElsewhere(t *testing.T) {
	swapFinding(t, nil, map[string]string{"gimp": "/usr/bin/gimp", "tool": "/home/you/.local/bin/tool"})

	same := sameFile
	t.Cleanup(func() { sameFile = same })

	// A link to the program its name runs is that program.
	sameFile = func(a, b string) bool {
		return a == b || (a == "/usr/bin/gimp" && b == "/home/you/bin/gimp-link/gimp") || (b == "/usr/bin/gimp" && a == "/home/you/bin/gimp-link/gimp")
	}

	for _, c := range []struct {
		argv      []string
		dir       string
		path      string
		elsewhere bool
	}{
		{[]string{"gimp", "-i"}, "", "/usr/bin/gimp", false},
		{[]string{"/usr/bin/gimp"}, "", "/usr/bin/gimp", false},
		{[]string{"/opt/gimp-3/bin/gimp"}, "", "/opt/gimp-3/bin/gimp", false},
		{[]string{"/home/you/bin/gimp-link/gimp"}, "", "/home/you/bin/gimp-link/gimp", false},
		{[]string{"/home/you/.local/bin/tool"}, "", "/home/you/.local/bin/tool", false},
		{[]string{"/tmp/cloned-repo/gimp"}, "", "/tmp/cloned-repo/gimp", true},
		{[]string{"/usr/bin/../../tmp/cloned-repo/gimp"}, "", "/tmp/cloned-repo/gimp", true},
		{[]string{"./gimp"}, "/tmp/cloned-repo", "/tmp/cloned-repo/gimp", true},
		{[]string{"bin/gimp"}, "/tmp/cloned-repo", "/tmp/cloned-repo/bin/gimp", true},
		// A place given from the folder is never the program its name
		// runs, even when the folder is the system's.
		{[]string{"./gimp"}, "/usr/bin", "/usr/bin/gimp", true},
		{[]string{"env", "-i", "/tmp/cloned-repo/gimp"}, "", "/tmp/cloned-repo/gimp", true},
		{[]string{"sh", "-c", "/tmp/cloned-repo/gimp --batch"}, "", "/tmp/cloned-repo/gimp", true},
	} {
		raw, _ := json.Marshal(map[string]any{"argv": c.argv, "dir": c.dir})

		got, _ := ProgramUseOf(context.Background(), RunCommand{}, raw)

		if got.Path != c.path || got.Elsewhere != c.elsewhere || got.Name != "gimp" && got.Name != "tool" {
			t.Errorf("%v in %q: %+v, want the file %s, elsewhere %v", c.argv, c.dir, got, c.path, c.elsewhere)
		}
	}

	// The window of a file that shares a name is elsewhere too.
	was := windowProgram
	t.Cleanup(func() { windowProgram = was })

	for path, elsewhere := range map[string]bool{"/usr/bin/gimp": false, "/home/you/Downloads/gimp": true, "": false} {
		windowProgram = func(context.Context, bool, int, int) (string, string) { return path, "" }

		for _, tool := range []Tool{TypeText{}, Click{}} {
			if got, _ := ProgramUseOf(context.Background(), tool, json.RawMessage(`{}`)); got.Elsewhere != elsewhere || got.Path != path {
				t.Errorf("%s in the window of %q: %+v, want elsewhere %v", tool.Name(), path, got, elsewhere)
			}
		}
	}
}

// As administrator is a command like any other: the program's card and
// "Stop using other programs" decide it too.
func TestRunningAsAdministratorSaysWhichProgramItRuns(t *testing.T) {
	swapFinding(t, nil, map[string]string{"firefox": "/usr/bin/firefox"})

	if !UsesOtherPrograms(RunAsAdmin{}) {
		t.Fatal("run_as_admin is not stopped by \"Stop using other programs\"")
	}

	got, bound := ProgramUseOf(context.Background(), RunAsAdmin{}, json.RawMessage(`{"argv":["env","-i","firefox"]}`))

	if want := (programs.Use{Verb: programs.VerbChange, Path: "/usr/bin/firefox", Name: "firefox"}); !bound || got != want {
		t.Errorf("run_as_admin: %+v, want %+v", got, want)
	}
}

type startedJobs struct{ started []string }

func (j *startedJobs) Start(what string, _ func(context.Context) (string, error)) (int64, error) {
	j.started = append(j.started, what)

	return int64(len(j.started)), nil
}

func (*startedJobs) List() []BackgroundJob { return nil }
func (*startedJobs) Stop(int64) error      { return nil }

/*
 * Nothing that acts in another program is started in the background.
 *
 * Scrolling asks nobody, so do_in_background accepted it and ran it where
 * neither "Stop using other programs" nor a program's card is weighed.
 */
func TestNothingThatActsInAnotherProgramStartsInTheBackground(t *testing.T) {
	jobs := &startedJobs{}
	registry := NewRegistry(Scroll{}, ListWindows{})
	background := InBackground{Jobs: jobs, Registry: func() *Registry { return registry }}

	out, err := background.Execute(context.Background(), json.RawMessage(`{"tool":"scroll","arguments":{"direction":"down"}}`))
	if err == nil || !strings.Contains(err.Error(), "another program") || len(jobs.started) != 0 {
		t.Fatalf("scrolling was started in the background: %q, %v, %v", out, err, jobs.started)
	}

	// Something of the assistant's own still is.
	if _, err := background.Execute(context.Background(), json.RawMessage(`{"tool":"list_windows","arguments":{}}`)); err != nil || len(jobs.started) != 1 {
		t.Fatalf("a tool that only reads was not started: %v, %v", err, jobs.started)
	}
}
