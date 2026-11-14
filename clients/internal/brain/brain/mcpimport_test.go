package brain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/mcpimport"
	"pn-scripts-assistant/internal/brain/programs"
)

// writeSettings writes a settings file the way editors do: a new file
// renamed over the old one.
func writeSettings(t *testing.T, path, text string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path+".tmp", []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func importedServer(t *testing.T, b *Brain, id string) mcp.Server {
	t.Helper()

	s, ok := b.Integrations.Book.Get(id)
	if !ok {
		t.Fatalf("%s is not an integration", id)
	}

	return s
}

/*
 * The whole path: servers set up in Claude Desktop and Cursor are found by
 * themselves, one is imported at the desk, and from then on its entry is
 * followed with real file events: removed, back, changed, a half-written
 * file (nothing concluded), the file deleted.
 */
func TestServersInOtherProgramsAreFoundImportedAndFollowed(t *testing.T) {
	b := quietBrain(t)
	home := t.TempDir()

	env := mcpimport.Env{GOOS: "linux", Home: home}
	b.mcpImports.env = &env
	b.mcpImports.opts = programs.Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 200 * time.Millisecond}

	desktop := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	cursor := filepath.Join(home, ".cursor", "mcp.json")

	original := `{"mcpServers": {"filesystem": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem@2026.1.14", "` +
		home + `/Projects"], "env": {"FS_TOKEN": "k1"}}}}`

	writeSettings(t, desktop, original)
	writeSettings(t, cursor, `{"mcpServers": {"wrapped": {"command": "bash", "args": ["-c", "npx thing"]}}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.KeepMCPImports(ctx)
	defer b.StopMCPImports()

	waitForPrograms(t, "the first look", func() bool {
		found, _, state := b.MCPImports()
		return len(found) == 2 && state != nil
	})

	found, files, state := b.MCPImports()

	byName := map[string]MCPFound{}
	for _, f := range found {
		byName[f.Name] = f
	}

	fsys := byName["filesystem"]
	if fsys.Problem != "" || fsys.Title != "Claude Desktop" || fsys.Imported != "" || fsys.Leaves || !strings.Contains(fsys.Where, "npx") {
		t.Errorf("filesystem found as %+v", fsys)
	}

	if byName["wrapped"].Problem != mcpimport.ProblemShell {
		t.Errorf("a shell was not refused: %+v", byName["wrapped"])
	}

	states := map[string]string{}
	for _, f := range files {
		states[f.Client] = f.State
	}

	if states["claude-desktop"] != FileRead || states["cursor"] != FileRead || states["claude-code"] != FileNone || states["vscode"] != FileNone {
		t.Errorf("file states %+v", files)
	}

	// The home folder (Claude Code) and the two set up are watched; VS Code's folder is not there yet.
	if len(state) != 1 || state[0].Watched != 3 || len(state[0].Polled) != 1 {
		t.Errorf("how the files are kept current: %+v", state)
	}

	// Nothing is used by itself.
	if len(b.Integrations.Book.Servers()) != 0 {
		t.Fatal("a found server became an integration without anybody asking")
	}

	lines := strings.Join(b.foundNotImported(), "\n")
	if !strings.Contains(lines, "filesystem, in Claude Desktop's settings") || !strings.Contains(lines, "cannot be used as written there (shell)") {
		t.Errorf("the model is told %q", lines)
	}

	if _, err := b.ImportMCP(byName["wrapped"].Key); err == nil {
		t.Error("a shell was imported")
	}

	s, err := b.ImportMCP(fsys.Key)
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "filesystem" || s.Approved || s.Active || s.From == nil || s.From.Client != "claude-desktop" {
		t.Errorf("imported as %+v", s)
	}

	if b.Integrations.SecretsOf("filesystem")["FS_TOKEN"] != "k1" {
		t.Error("the entry's key was not kept as a secret")
	}

	if raw, _ := os.ReadFile(filepath.Join(b.Root, mcp.FolderName, "servers.json")); strings.Contains(string(raw), "k1") {
		t.Error("the key is in the readable servers file")
	}

	if _, err := b.ImportMCP(fsys.Key); err == nil {
		t.Error("the same entry was imported twice")
	}

	if found, _, _ := b.MCPImports(); found[0].Imported != "filesystem" && found[1].Imported != "filesystem" {
		t.Errorf("the page does not say it was imported: %+v", found)
	}

	if _, err := b.Integrations.Approve("filesystem", nil); err != nil {
		t.Fatal(err)
	}

	// Removed in Claude Desktop.
	writeSettings(t, desktop, `{"mcpServers": {}}`)
	waitForPrograms(t, "the removal", func() bool { return importedServer(t, b, "filesystem").Gone() })

	if _, err := b.Integrations.Activate(ctx, "filesystem"); err == nil {
		t.Error("a server removed in Claude Desktop could be started")
	}

	out, _ := mcp.ListIntegrations{Gateway: b.Integrations, Found: b.foundNotImported}.Execute(ctx, nil)
	if !strings.Contains(out, "no longer in Claude Desktop's settings since") {
		t.Errorf("the model is not told it went: %q", out)
	}

	// Back, unchanged: usable again with its approval.
	writeSettings(t, desktop, original)
	waitForPrograms(t, "the return", func() bool { return !importedServer(t, b, "filesystem").Gone() })

	if !importedServer(t, b, "filesystem").Approved {
		t.Error("a server back unchanged lost its approval")
	}

	// Half written: nothing is concluded.
	writeSettings(t, desktop, `{"mcpServers": {"filesystem": {"command": "np`)
	waitForPrograms(t, "the broken file", func() bool {
		_, files, _ := b.MCPImports()
		return files[0].State == FileBroken
	})

	if s := importedServer(t, b, "filesystem"); s.Gone() || !s.Approved {
		t.Errorf("a half-written file changed it: %+v", s)
	}

	// Changed: the new definition, asked about again.
	writeSettings(t, desktop, strings.Replace(original, "2026.1.14", "2026.2.1", 1))
	waitForPrograms(t, "the change", func() bool {
		return strings.Contains(strings.Join(importedServer(t, b, "filesystem").Command, " "), "2026.2.1")
	})

	if importedServer(t, b, "filesystem").Approved {
		t.Error("a changed server kept its approval")
	}

	// A new key only: kept, nothing else.
	if _, err := b.Integrations.Approve("filesystem", nil); err != nil {
		t.Fatal(err)
	}

	writeSettings(t, desktop, strings.Replace(strings.Replace(original, "2026.1.14", "2026.2.1", 1), "k1", "k2", 1))
	waitForPrograms(t, "the new key", func() bool { return b.Integrations.SecretsOf("filesystem")["FS_TOKEN"] == "k2" })

	if !importedServer(t, b, "filesystem").Approved {
		t.Error("a new key withdrew the approval")
	}

	// Claude Desktop's settings deleted (the program uninstalled): gone.
	if err := os.Remove(desktop); err != nil {
		t.Fatal(err)
	}

	waitForPrograms(t, "the deleted file", func() bool { return importedServer(t, b, "filesystem").Gone() })

	// The event log says what happened, with no key in it.
	events, _ := json.Marshal(b.Integrations.Events("filesystem", 30))
	for _, want := range []string{"imported", "gone from claude-desktop", "back in claude-desktop", "changed in claude-desktop"} {
		if !strings.Contains(string(events), want) {
			t.Errorf("no %q in the record: %s", want, events)
		}
	}

	if strings.Contains(string(events), "k1") || strings.Contains(string(events), "k2") {
		t.Error("a key is in the record")
	}
}

// An entry that went while the program was closed is never started on the
// way back up.
func TestAServerWhoseEntryWentIsNotResumed(t *testing.T) {
	b := quietBrain(t)
	home := t.TempDir()

	env := mcpimport.Env{GOOS: "linux", Home: home}
	b.mcpImports.env = &env
	b.mcpImports.opts = programs.Options{Debounce: 50 * time.Millisecond, Longest: time.Second, PollEvery: 200 * time.Millisecond}

	desktop := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")

	// It was imported, approved and running when the program closed; a
	// command that would fail loudly if it were ever started.
	s := mcp.Server{ID: "gone", Title: "gone", Transport: "stdio", Command: []string{filepath.Join(home, "no-such-program")},
		Approved: true, Active: true,
		From: &mcp.Origin{Client: "claude-desktop", File: desktop, Name: "gone", Print: "x"}}

	if err := b.Integrations.Book.Put(s); err != nil {
		t.Fatal(err)
	}

	writeSettings(t, desktop, `{"mcpServers": {}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.KeepMCPImports(ctx)
	defer b.StopMCPImports()

	waitForPrograms(t, "the first look", func() bool { return importedServer(t, b, "gone").Gone() })

	time.Sleep(100 * time.Millisecond)

	for _, e := range b.Integrations.Events("gone", 30) {
		if strings.Contains(e.Event, "activated") || strings.Contains(e.Event, "failed") {
			t.Errorf("a gone server was started on the way up: %+v", e)
		}
	}

	if got := importedServer(t, b, "gone"); !got.From.WasActive || got.Active {
		t.Errorf("gone as %+v %+v", got, got.From)
	}
}
