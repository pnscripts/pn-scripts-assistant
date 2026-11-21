package connect

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcpimport"
)

const program = "/usr/bin/pn-scripts-assistant"

var at = time.Date(2026, 11, 21, 18, 30, 5, 0, time.UTC)

func here(t *testing.T) (mcpimport.Env, string) {
	t.Helper()

	home := t.TempDir()

	return mcpimport.Env{GOOS: "linux", Home: home}, home
}

func files(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}

	return names
}

/*
 * The plan says the file and the entry, word for word, and reading it
 * changes nothing; writing does exactly what the plan said, after copying
 * the file as it was beside it, and keeps everything else in the file as
 * it was and in its order.
 */
func TestClaudeCodeIsConnectedAsThePlanSaid(t *testing.T) {
	env, home := here(t)
	file := filepath.Join(home, ".claude.json")

	// As Claude Code writes it: its own state first, servers in the middle,
	// projects after, with numbers and nesting that must come through.
	before := `{
  "numStartups": 412,
  "theme": "dark",
  "mcpServers": {
    "zeta": { "command": "npx", "args": ["-y", "zeta-server"], "env": { "TOKEN": "s3cret<>&" } },
    "alpha": { "type": "http", "url": "https://example.test/mcp" }
  },
  "projects": { "/home/me/work": { "allowedTools": [], "cost": 0.1234567890123, "big": 9007199254740993 } },
  "userID": "abc"
}
`
	if err := os.WriteFile(file, []byte(before), 0o640); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanFor(mcpimport.ClaudeCode, env, program)
	if err != nil {
		t.Fatal(err)
	}

	wantEntry := `{
  "pn-scripts-assistant": {
    "type": "stdio",
    "command": "/usr/bin/pn-scripts-assistant",
    "args": [
      "mcp"
    ]
  }
}`

	if plan.File != file || plan.State != StateNotThere || plan.Entry != wantEntry || plan.Title != "Claude Code" || len(plan.Seen) != 64 || plan.Now != "" {
		t.Fatalf("the plan: %+v", plan)
	}

	// Planning read and wrote nothing.
	if now, _ := os.ReadFile(file); string(now) != before || len(files(t, home)) != 1 {
		t.Fatalf("planning changed something: %v", files(t, home))
	}

	done, err := Write(mcpimport.ClaudeCode, env, program, plan.Seen, at)
	if err != nil {
		t.Fatal(err)
	}

	backup := file + ".before-pn-scripts-assistant-20261121-183005"
	if done.File != file || done.Backup != backup {
		t.Fatalf("what was done: %+v", done)
	}

	// The copy is the file as it was, byte for byte, for its owner alone.
	if kept, _ := os.ReadFile(backup); string(kept) != before {
		t.Errorf("the copy is not the file as it was:\n%s", kept)
	}

	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(backup); info.Mode().Perm() != 0o600 {
			t.Errorf("the copy is readable by more than its owner: %v", info.Mode().Perm())
		}

		// The file keeps the rights it had.
		if info, _ := os.Stat(file); info.Mode().Perm() != 0o640 {
			t.Errorf("the file's rights changed: %v", info.Mode().Perm())
		}
	}

	after, _ := os.ReadFile(file)

	// Everything that was there is there, in its order, and the assistant
	// is the last of the servers.
	var order []string

	for _, m := range mustObject(t, after) {
		order = append(order, m.key)
	}

	if strings.Join(order, " ") != "numStartups theme mcpServers projects userID" {
		t.Errorf("the names of the file are now %v", order)
	}

	var got struct {
		NumStartups int               `json:"numStartups"`
		Servers     map[string]entry  `json:"mcpServers"`
		Projects    json.RawMessage   `json:"projects"`
		Raw         map[string]string `json:"-"`
	}

	if err := json.Unmarshal(after, &got); err != nil {
		t.Fatalf("the file is no longer JSON: %v\n%s", err, after)
	}

	mine := got.Servers[Name]
	if got.NumStartups != 412 || len(got.Servers) != 3 || mine.Command != program || strings.Join(mine.Args, " ") != "mcp" || mine.Type != "stdio" {
		t.Errorf("the servers now: %+v", got.Servers)
	}

	var servers []string
	for _, m := range mustObject(t, mustObject(t, after)[2].value) {
		servers = append(servers, m.key)
	}

	if strings.Join(servers, " ") != "zeta alpha pn-scripts-assistant" {
		t.Errorf("the servers are now in the order %v", servers)
	}

	// Numbers too large or too exact for a float come through as written,
	// and so does a secret with characters JSON may write two ways.
	for _, kept := range []string{"0.1234567890123", "9007199254740993", `"s3cret<>&"`, `"https://example.test/mcp"`} {
		if !bytes.Contains(after, []byte(kept)) {
			t.Errorf("%s did not come through:\n%s", kept, after)
		}
	}

	// Nothing else is left beside it: no temporary file.
	if names := files(t, home); len(names) != 2 {
		t.Errorf("left in the folder: %v", names)
	}

	// Now it is connected, and writing again is refused with nothing done.
	again, _ := PlanFor(mcpimport.ClaudeCode, env, program)
	if again.State != StateSame {
		t.Fatalf("after connecting the plan says %q", again.State)
	}

	if _, err := Write(mcpimport.ClaudeCode, env, program, again.Seen, at.Add(time.Second)); !errors.Is(err, ErrSame) || len(files(t, home)) != 2 {
		t.Errorf("connecting twice: %v, %v", err, files(t, home))
	}
}

func mustObject(t *testing.T, raw []byte) []member {
	t.Helper()

	o, err := object(raw)
	if err != nil {
		t.Fatalf("not an object: %v\n%s", err, raw)
	}

	return o
}

// Claude Desktop has no settings file until something is set: one is made,
// in a folder of its own, and there is nothing to copy first.
func TestClaudeDesktopWithNoSettingsYet(t *testing.T) {
	env, home := here(t)
	file := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")

	plan, err := PlanFor(mcpimport.ClaudeDesktop, env, program)
	if err != nil || plan.File != file || plan.State != StateNoFile || strings.Contains(plan.Entry, `"type"`) {
		t.Fatalf("the plan: %+v, %v", plan, err)
	}

	if _, err := os.Stat(filepath.Dir(file)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("planning made the folder")
	}

	done, err := Write(mcpimport.ClaudeDesktop, env, program, plan.Seen, at)
	if err != nil || done.Backup != "" {
		t.Fatalf("writing: %+v, %v", done, err)
	}

	raw, _ := os.ReadFile(file)

	want := "{\n  \"mcpServers\": {\n    \"pn-scripts-assistant\": {\n      \"command\": \"/usr/bin/pn-scripts-assistant\",\n      \"args\": [\n        \"mcp\"\n      ]\n    }\n  }\n}\n"
	if string(raw) != want {
		t.Errorf("the new file:\n%s", raw)
	}

	// Its own settings reader finds it there, as the assistant's own entry.
	found, err := mcpimport.Read(mcpimport.Place{Client: mcpimport.ClaudeDesktop, File: file}, raw, home)
	if err != nil {
		t.Fatalf("the file is not read back as settings: %v", err)
	}

	for _, f := range found {
		if f.Name == Name && f.Problem == "" {
			t.Errorf("the assistant would import itself as a server of its own: %+v", f)
		}
	}
}

/*
 * What is written is what was shown: a file that changed after the plan, by
 * a byte, is not written, nothing is copied, and the next plan shows it as
 * it stands.
 */
func TestAFileThatChangedSinceItWasShownIsNotWritten(t *testing.T) {
	env, home := here(t)
	file := filepath.Join(home, ".claude.json")

	os.WriteFile(file, []byte(`{"mcpServers":{}}`), 0o600)

	plan, _ := PlanFor(mcpimport.ClaudeCode, env, program)

	// Claude Code wrote its settings in between.
	os.WriteFile(file, []byte(`{"mcpServers":{},"numStartups":1}`), 0o600)

	if _, err := Write(mcpimport.ClaudeCode, env, program, plan.Seen, at); !errors.Is(err, ErrChanged) {
		t.Fatalf("a changed file: %v", err)
	}

	if raw, _ := os.ReadFile(file); string(raw) != `{"mcpServers":{},"numStartups":1}` || len(files(t, home)) != 1 {
		t.Errorf("something was written or copied: %s, %v", raw, files(t, home))
	}

	// A plan made for another program's path, or for the other client,
	// does not write this one.
	other, _ := PlanFor(mcpimport.ClaudeCode, env, "/opt/elsewhere/pn-scripts-assistant")
	if _, err := Write(mcpimport.ClaudeCode, env, program, other.Seen, at); !errors.Is(err, ErrChanged) {
		t.Errorf("a plan for another path: %v", err)
	}

	for _, seen := range []string{"", "0", strings.Repeat("0", 64)} {
		if _, err := Write(mcpimport.ClaudeCode, env, program, seen, at); !errors.Is(err, ErrChanged) {
			t.Errorf("a plan nobody was shown (%q): %v", seen, err)
		}
	}

	// A file that came to be after a plan for none.
	os.Remove(file)

	none, _ := PlanFor(mcpimport.ClaudeCode, env, program)

	os.WriteFile(file, nil, 0o600)

	if _, err := Write(mcpimport.ClaudeCode, env, program, none.Seen, at); !errors.Is(err, ErrChanged) {
		t.Errorf("a file that appeared: %v", err)
	}
}

// An entry of the assistant's name that says something else (an older
// place of the program) is shown, and replaced only by the plan that
// showed it.
func TestAnOlderEntryIsShownAndReplaced(t *testing.T) {
	env, home := here(t)
	file := filepath.Join(home, ".claude.json")

	os.WriteFile(file, []byte(`{"mcpServers":{"pn-scripts-assistant":{"type":"stdio","command":"/home/me/old/pn-scripts-assistant","args":["mcp","--api","http://127.0.0.1:9999"]},"other":{"command":"x"}}}`), 0o600)

	plan, _ := PlanFor(mcpimport.ClaudeCode, env, program)
	if plan.State != StateDifferent || !strings.Contains(plan.Now, "/home/me/old/pn-scripts-assistant") || !strings.Contains(plan.Now, "9999") {
		t.Fatalf("the plan: %+v", plan)
	}

	if _, err := Write(mcpimport.ClaudeCode, env, program, plan.Seen, at); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Servers map[string]entry `json:"mcpServers"`
	}

	raw, _ := os.ReadFile(file)
	json.Unmarshal(raw, &got)

	if len(got.Servers) != 2 || got.Servers[Name].Command != program || len(got.Servers[Name].Args) != 1 || got.Servers["other"].Command != "x" {
		t.Errorf("after replacing: %s", raw)
	}

	// The same entry written another way round is the same.
	os.WriteFile(file, []byte(`{"mcpServers":{"pn-scripts-assistant":{"args":["mcp"],  "command":"/usr/bin/pn-scripts-assistant","type":"stdio"}}}`), 0o600)

	if same, _ := PlanFor(mcpimport.ClaudeCode, env, program); same.State != StateSame {
		t.Errorf("the same entry in another order is %q", same.State)
	}
}

// What is not plainly a settings file is left alone, whatever was shown.
func TestWhatIsNotASettingsFileIsLeftAlone(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":               `{"mcpServers": {`,
		"a list":                 `[1,2,3]`,
		"two values":             `{} {}`,
		"servers not a object":   `{"mcpServers": ["a"]}`,
		"a comment":              "// mine\n{}",
		"servers written twice":  `{"mcpServers":{"a":{"command":"x"}},"mcpServers":{}}`,
		"a server written twice": `{"mcpServers":{"pn-scripts-assistant":{"command":"x"},"pn-scripts-assistant":{"command":"y"}}}`,
	} {
		env, home := here(t)
		file := filepath.Join(home, ".claude.json")

		os.WriteFile(file, []byte(content), 0o600)

		plan, err := PlanFor(mcpimport.ClaudeCode, env, program)
		if err != nil || plan.State != StateUnread {
			t.Errorf("%s: the plan says %q, %v", name, plan.State, err)
		}

		if _, err := Write(mcpimport.ClaudeCode, env, program, plan.Seen, at); !errors.Is(err, ErrUnread) {
			t.Errorf("%s: writing: %v", name, err)
		}

		if raw, _ := os.ReadFile(file); string(raw) != content || len(files(t, home)) != 1 {
			t.Errorf("%s: something was written or copied", name)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}

	// A link in its place is somebody's arrangement: not written through.
	env, home := here(t)
	real := filepath.Join(t.TempDir(), "real.json")

	os.WriteFile(real, []byte(`{}`), 0o600)
	os.Symlink(real, filepath.Join(home, ".claude.json"))

	plan, _ := PlanFor(mcpimport.ClaudeCode, env, program)
	if _, err := Write(mcpimport.ClaudeCode, env, program, plan.Seen, at); plan.State != StateUnread || !errors.Is(err, ErrUnread) {
		t.Errorf("a link: %q, %v", plan.State, err)
	}

	if raw, _ := os.ReadFile(real); string(raw) != `{}` {
		t.Error("written through a link")
	}
}

func TestOnlyWhatItKnowsAndOnlyByAFullPath(t *testing.T) {
	env, _ := here(t)

	for _, c := range []mcpimport.Client{mcpimport.Cursor, mcpimport.VSCode, "", "claude"} {
		if _, err := PlanFor(c, env, program); err == nil {
			t.Errorf("a plan for %q", c)
		}

		if _, err := Write(c, env, program, strings.Repeat("0", 64), at); err == nil {
			t.Errorf("writing for %q", c)
		}
	}

	for _, p := range []string{"", "pn-scripts-assistant", "./pn-scripts-assistant", "bin/pn-scripts-assistant"} {
		if _, err := PlanFor(mcpimport.ClaudeCode, env, p); err == nil {
			t.Errorf("a plan naming the program as %q", p)
		}
	}

	if !Known(mcpimport.ClaudeCode) || !Known(mcpimport.ClaudeDesktop) || Known(mcpimport.Cursor) || len(Clients) != 2 {
		t.Error("the programs it connects to")
	}

	// Two copies made in the same second do not overwrite one another: the
	// second write is refused before anything changes.
	env2, home := here(t)
	file := filepath.Join(home, ".claude.json")

	os.WriteFile(file, []byte(`{}`), 0o600)
	os.WriteFile(file+".before-pn-scripts-assistant-20261121-183005", []byte("an older copy"), 0o600)

	plan, _ := PlanFor(mcpimport.ClaudeCode, env2, program)
	if _, err := Write(mcpimport.ClaudeCode, env2, program, plan.Seen, at); err == nil {
		t.Error("an older copy of the same second was overwritten")
	}

	if raw, _ := os.ReadFile(file); string(raw) != `{}` {
		t.Errorf("the file was changed without its copy: %s", raw)
	}

	if kept, _ := os.ReadFile(file + ".before-pn-scripts-assistant-20261121-183005"); string(kept) != "an older copy" {
		t.Error("the older copy was changed")
	}
}
