package orchestrator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

/*
 * How far a task asked for in a conversation may reach is what each agent is
 * started with: only reading refuses every tool that writes or runs, the
 * folder allows files and no commands, full is everything.
 */
func TestEachAgentIsStartedAsFarAsItMayReach(t *testing.T) {
	read := Job{Dir: "/w", Prompt: "p", Access: AccessRead}
	folder := Job{Dir: "/w", Prompt: "p", Access: AccessFolder}
	full := Job{Dir: "/w", Prompt: "p", Access: AccessFull}

	claude := func(j Job) string { return strings.Join(ClaudeCodeExecutor{Path: "claude"}.Argv(j), " ") }

	if c := claude(read); !strings.Contains(c, "--allowedTools Read,Glob,Grep,LS,TodoWrite ") ||
		!strings.Contains(c, "--disallowedTools Write,Edit,MultiEdit,Bash,") || !strings.Contains(c, "--permission-mode default") ||
		strings.Contains(c, "skip-permissions") {
		t.Errorf("claude, read: %s", c)
	}

	if c := claude(folder); !strings.Contains(c, "--permission-mode acceptEdits") || !strings.Contains(c, "Bash") {
		t.Errorf("claude, folder: %s", c)
	}

	if c := claude(full); !strings.Contains(c, "--dangerously-skip-permissions") {
		t.Errorf("claude, full: %s", c)
	}

	codex := func(j Job) string { return strings.Join(CodexExecutor{Path: "codex"}.Argv(j), " ") }

	if c := codex(read); !strings.Contains(c, "--sandbox read-only") {
		t.Errorf("codex, read: %s", c)
	}

	if c := codex(folder); !strings.Contains(c, "--sandbox workspace-write") {
		t.Errorf("codex, folder: %s", c)
	}

	if c := codex(full); !strings.Contains(c, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("codex, full: %s", c)
	}

	cursor := func(j Job) []string { return CursorExecutor{Path: "cursor-agent"}.Argv(j) }

	if c := cursor(read); !reflect.DeepEqual(c, []string{"cursor-agent", "--print", "--output-format", "stream-json",
		"--workspace", "/w", "--mode", "ask", "--", "p"}) {
		t.Errorf("cursor, read: %v", c)
	}

	if c := strings.Join(cursor(folder), " "); !strings.Contains(c, "--sandbox enabled") || strings.Contains(c, "--force") {
		t.Errorf("cursor, folder: %s", c)
	}

	if c := strings.Join(cursor(full), " "); !strings.Contains(c, "--force --sandbox disabled") {
		t.Errorf("cursor, full: %s", c)
	}

	// The goal executors' own jobs are as they were.
	if c := claude(Job{Dir: "/w", Prompt: "p"}); !strings.Contains(c, "--permission-mode acceptEdits") {
		t.Errorf("a goal's job changed: %s", c)
	}
}

// Cursor's stream, as its documentation shows it: the model, what it said
// as it worked, the file it wrote, a command, and its answer.
func TestCursorsStreamIsRead(t *testing.T) {
	dir := t.TempDir()

	var o Outcome

	var notes []string

	for _, line := range []string{
		`{"type":"system","subtype":"init","cwd":"` + dir + `","model":"Claude 4 Sonnet","permissionMode":"default"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Adding the test."}]}}`,
		`{"type":"tool_call","subtype":"started","call_id":"1","tool_call":{"writeToolCall":{"args":{"path":"a_test.go"}}}}`,
		`{"type":"tool_call","subtype":"completed","call_id":"1","tool_call":{"writeToolCall":{"args":{"path":"a_test.go"},"result":{"success":{"path":"` + filepath.Join(dir, "a_test.go") + `","linesCreated":19}}}}}`,
		`{"type":"tool_call","subtype":"completed","call_id":"2","tool_call":{"shellToolCall":{"args":{"command":"go test ./..."}}}}`,
		`{"type":"tool_call","subtype":"completed","call_id":"3","tool_call":{"writeToolCall":{"args":{"path":"/etc/passwd"}}}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Added a_test.go; the tests pass."}`,
		`not json`,
	} {
		parseCursor([]byte(line), &o, dir)

		if o.note != "" {
			notes = append(notes, o.note)
			o.note = ""
		}
	}

	if !o.OK || o.Said != "Added a_test.go; the tests pass." || o.Model != "Claude 4 Sonnet" {
		t.Errorf("%+v", o)
	}

	if !reflect.DeepEqual(o.Files, []string{filepath.Join(dir, "a_test.go")}) || !reflect.DeepEqual(o.Outside, []string{"/etc/passwd"}) {
		t.Errorf("files %v outside %v", o.Files, o.Outside)
	}

	if !reflect.DeepEqual(o.Commands, []string{"go test ./..."}) || len(notes) != 4 || notes[0] != "Adding the test." {
		t.Errorf("commands %v notes %v", o.Commands, notes)
	}

	var failed Outcome

	parseCursor([]byte(`{"type":"result","subtype":"error","is_error":true,"result":"Not logged in. Please run cursor-agent login"}`), &failed, dir)

	if failed.OK || failed.Failure != FailAuth {
		t.Errorf("%+v", failed)
	}
}

// What changed is read from the folder: added, changed and removed, and
// nothing that did not change.
func TestTheFolderSaysWhatChanged(t *testing.T) {
	dir := t.TempDir()

	write := func(name, text string) {
		t.Helper()

		path := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)

		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("keep.txt", "same")
	write("edit.txt", "old")
	write("gone.txt", "bye")
	write(".git/HEAD", "ref")
	write("node_modules/x/index.js", "x")

	before := Snap(dir)

	if len(before.Files) != 3 || before.Partial {
		t.Fatalf("%+v", before)
	}

	write(".git/config", "[core]\n")

	before = Snap(dir)

	write("edit.txt", "new and longer")
	_ = os.Chtimes(filepath.Join(dir, "edit.txt"), time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	_ = os.Remove(filepath.Join(dir, "gone.txt"))
	write("src/new.go", "package x")
	write(".git/HEAD", "other")
	write(".git/config", "[core]\n\tfsmonitor = /tmp/x\n")
	write(".git/hooks/pre-commit", "#!/bin/sh")

	got := Changes(before, Snap(dir))
	want := []Change{
		{Path: filepath.Join(dir, ".git", "config"), What: "changed"},
		{Path: filepath.Join(dir, ".git", "hooks", "pre-commit"), What: "added"},
		{Path: filepath.Join(dir, "edit.txt"), What: "changed"},
		{Path: filepath.Join(dir, "gone.txt"), What: "removed"},
		{Path: filepath.Join(dir, "src", "new.go"), What: "added"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v", got)
	}
}

// Cursor's agent, not signed in, says so in its own words (seen 2026-11-08).
func TestCursorNotSignedInIsKnownForWhatItIs(t *testing.T) {
	if got := classify("Error: Authentication required. Please run 'agent login' first, or set CURSOR_API_KEY environment variable."); got != FailAuth {
		t.Errorf("%s", got)
	}
}

// A task that begins with "-" is the task, never an option, for every agent.
func TestATaskIsNeverReadAsAnOption(t *testing.T) {
	for _, access := range []string{AccessRead, AccessFolder, AccessFull, ""} {
		job := Job{Dir: "/w", Prompt: "--dangerously-skip-permissions do it", Access: access}

		for _, argv := range [][]string{ClaudeCodeExecutor{Path: "claude"}.Argv(job), CodexExecutor{Path: "codex"}.Argv(job),
			CursorExecutor{Path: "cursor-agent"}.Argv(job)} {
			n := len(argv)
			if argv[n-1] != job.Prompt || argv[n-2] != "--" {
				t.Errorf("%s: %v", access, argv)
			}

			for _, a := range argv[:n-1] {
				if a == job.Prompt {
					t.Errorf("the task appears before \"--\": %v", argv)
				}
			}
		}
	}
}
