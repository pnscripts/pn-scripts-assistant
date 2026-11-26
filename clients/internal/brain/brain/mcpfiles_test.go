//go:build linux

package brain

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/mcpimport"
)

/*
 * Notes of the read after 1.2.0 (ASSISTANT-PROGRAM-USE-NOTES-1121, 4).
 *
 * The watched settings files were looked at with os.Stat and then read
 * with os.ReadFile: two looks, links followed, so what was read need not
 * be what was measured. A settings file that is a pipe stopped the look
 * for good while it held the lock (opening a pipe waits for a writer),
 * and one that is a link to /dev/zero was read without end. A file is
 * now opened once, measured as opened, read only when it is a regular
 * file, and never past mcpimport.MostBytes.
 */
func TestASettingsFileIsReadOnceAndOnlyWhenItIsAFile(t *testing.T) {
	home := t.TempDir()

	env := mcpimport.Env{GOOS: "linux", Home: home}
	places := mcpimport.Places(env)

	byClient := map[mcpimport.Client]string{}
	for _, p := range places {
		byClient[p.Client] = p.File

		if err := os.MkdirAll(filepath.Dir(p.File), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Claude Desktop: a pipe nobody writes to.
	if err := syscall.Mkfifo(byClient[mcpimport.ClaudeDesktop], 0o600); err != nil {
		t.Skip("no pipes here:", err)
	}

	// Claude Code: a link to a device that never ends.
	if err := os.Symlink("/dev/zero", byClient[mcpimport.ClaudeCode]); err != nil {
		t.Fatal(err)
	}

	// Cursor: a link to a real settings file, as people keep them in a
	// folder of their own: read.
	real := filepath.Join(home, "dotfiles", "mcp.json")
	writeSettings(t, real, `{"mcpServers": {"filesystem": {"command": "npx", "args": ["-y", "server-filesystem@2026.1.14"]}}}`)

	if err := os.Symlink(real, byClient[mcpimport.Cursor]); err != nil {
		t.Fatal(err)
	}

	// VS Code: larger than the most read (sparse: costs no disk).
	big, err := os.Create(byClient[mcpimport.VSCode])
	if err != nil {
		t.Fatal(err)
	}

	if err := big.Truncate(mcpimport.MostBytes + 1); err != nil {
		t.Fatal(err)
	}

	big.Close()

	b := quietBrain(t)
	b.mcpImports.env = &env

	type result struct {
		found []mcpimport.Found
		files []MCPFile
		read  map[string]bool
	}

	done := make(chan result, 1)

	go func() {
		found, files, read := b.readMCPSettings()
		done <- result{found, files, read}
	}()

	var r result

	select {
	case r = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the look never ended: a settings file that is a pipe or a device held it")
	}

	state := map[string]string{}
	for _, f := range r.files {
		state[f.Client] = f.State
	}

	want := map[string]string{
		string(mcpimport.ClaudeDesktop): FileError,
		string(mcpimport.ClaudeCode):    FileError,
		string(mcpimport.Cursor):        FileRead,
		string(mcpimport.VSCode):        FileBig,
	}

	for client, w := range want {
		if state[client] != w {
			t.Errorf("%s: %q, want %q", client, state[client], w)
		}
	}

	if len(r.found) != 1 || r.found[0].Name != "filesystem" {
		t.Errorf("found %+v", r.found)
	}

	// Neither the pipe nor the device counts as read to the end: an entry
	// imported from them is not taken as gone.
	if r.read[byClient[mcpimport.ClaudeDesktop]] || r.read[byClient[mcpimport.ClaudeCode]] || !r.read[byClient[mcpimport.Cursor]] {
		t.Errorf("read to the end: %v", r.read)
	}
}

// A file that grows past the most read after it was measured is still
// read no further than that.
func TestASettingsFileIsNeverReadPastTheMost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Truncate(mcpimport.MostBytes + 4096); err != nil {
		t.Fatal(err)
	}

	f.Close()

	raw, state := readSettingsFile(path)
	if state != FileBig || raw != nil {
		t.Errorf("%q, %d bytes", state, len(raw))
	}

	if raw, state := readSettingsFile(filepath.Join(dir, "none.json")); state != FileNone || raw != nil {
		t.Errorf("a missing file: %q", state)
	}

	if raw, state := readSettingsFile(dir); state != FileError || raw != nil {
		t.Errorf("a folder: %q", state)
	}

	writeSettings(t, path, `{"mcpServers": {}}`)

	if raw, state := readSettingsFile(path); state != FileRead || string(raw) != `{"mcpServers": {}}` {
		t.Errorf("a plain file: %q %q", state, raw)
	}
}
