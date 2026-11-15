package mcpimport

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * What an imported server would be given is shown or refused, never
 * hidden.
 *
 * Found by the 1.1.0 security review. A setting that makes the program
 * load other code (NODE_OPTIONS=--require ..., LD_PRELOAD) was passed on
 * unseen, so "npx -y server-x" on the card was not what would run. A key
 * written into the command or the address (a database address with its
 * password, --api-key sk-..., ?token=...) was kept as plain words, shown
 * with every use and written unsealed. And a program on this machine was
 * described as staying on it, although nothing says an imported one does.
 */
func TestASettingThatLoadsOtherCodeIsRefused(t *testing.T) {
	raw := `{"mcpServers": {
  "node":    {"command": "npx", "args": ["-y", "server-x"], "env": {"NODE_OPTIONS": "--require /tmp/x.js"}},
  "preload": {"command": "npx", "args": ["-y", "server-x"], "env": {"LD_PRELOAD": "/tmp/x.so"}},
  "dyld":    {"command": "npx", "args": ["-y", "server-x"], "env": {"DYLD_INSERT_LIBRARIES": "/tmp/x.dylib"}},
  "python":  {"command": "uvx", "args": ["server-y"], "env": {"PYTHONSTARTUP": "/tmp/x.py"}},
  "bash":    {"command": "npx", "args": ["-y", "server-x"], "env": {"bash_env": "/tmp/x.sh"}},
  "java":    {"command": "java", "args": ["-jar", "s.jar"], "env": {"JAVA_TOOL_OPTIONS": "-javaagent:/tmp/x.jar"}},
  "pypath":  {"command": "uvx", "args": ["server-y"], "env": {"PYTHONPATH": "/tmp/x"}},
  "npmrc":   {"command": "npx", "args": ["-y", "server-x"], "env": {"npm_config_registry": "https://registry.example.com/"}},
  "index":   {"command": "uvx", "args": ["server-y"], "env": {"UV_INDEX_URL": "https://pypi.example.com/simple"}},
  "plain":   {"command": "npx", "args": ["-y", "server-x"], "env": {"LOG_LEVEL": "debug", "PATH": "/home/you/.nvm/bin:/usr/bin"}},
  "pathkey": {"command": "npx", "args": ["-y", "server-x"], "env": {"PATH": "AKIAZZZZZZZZZZZZZZZZ:/tmp/x"}}
}}`

	fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	for name, variable := range map[string]string{"node": "NODE_OPTIONS", "preload": "LD_PRELOAD", "dyld": "DYLD_INSERT_LIBRARIES",
		"python": "PYTHONSTARTUP", "bash": "bash_env", "java": "JAVA_TOOL_OPTIONS",
		"pypath": "PYTHONPATH", "npmrc": "npm_config_registry", "index": "UV_INDEX_URL"} {
		f := one(t, fs, name)

		if f.Problem != ProblemLoader || f.Detail != variable {
			t.Errorf("%s: problem %q (%s), want %q naming %s", name, f.Problem, f.Detail, ProblemLoader, variable)
		}

		if _, err := Imported(f, "x"+name); err == nil {
			t.Errorf("%s was imported", name)
		}
	}

	// Where programs are looked for is always shown, never taken for a
	// key by the look of its value and hidden.
	if f := one(t, fs, "pathkey"); f.Problem != "" || f.Server.Env["PATH"] != "AKIAZZZZZZZZZZZZZZZZ:/tmp/x" || len(f.Server.Secrets) != 0 {
		t.Errorf("a PATH shaped like a key was hidden: %+v", f)
	}

	// Ordinary settings are kept, to be shown with their values.
	plain := one(t, fs, "plain")
	if plain.Problem != "" || plain.Server.Env["LOG_LEVEL"] != "debug" || plain.Server.Env["PATH"] != "/home/you/.nvm/bin:/usr/bin" {
		t.Errorf("an ordinary setting was refused or lost: %+v", plain)
	}
}

func TestAKeyInTheCommandOrTheAddressIsRefusedAndNotShown(t *testing.T) {
	raw := `{"mcpServers": {
  "postgres": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://app:hunter2zz@db.example.com/shop"]},
  "flag":     {"command": "npx", "args": ["-y", "server-x", "--api-key", "zz-plain-value-1"]},
  "joined":   {"command": "npx", "args": ["-y", "server-x", "--token=zz-plain-value-2"]},
  "shape":    {"command": "npx", "args": ["-y", "server-x", "ghp_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"]},
  "query":    {"type": "http", "url": "https://mcp.example.com/mcp?token=zz-plain-value-3"},
  "userinfo": {"type": "http", "url": "https://app:hunter3zz@mcp.example.com/mcp"},
  "header":   {"command": "npx", "args": ["-y", "mcp-remote", "https://mcp.example.com/mcp", "--header", "Authorization: Bearer zz-plain-value-4"]},
  "folder":   {"command": "npx", "args": ["-y", "server-x", "--api-key", "zz-plain-value-5"], "cwd": "/home/you/server"},
  "shell":    {"command": "sh", "args": ["-c", "tool --api-key zz-plain-value-6"]},
  "oldway":   {"type": "sse", "url": "https://old.example.com/sse/zz-plain-value-7"},
  "fine":     {"command": "npx", "args": ["-y", "server-x", "--port", "8080", "--key-file", "/home/you/key.pem", "--max-tokens", "4096", "--author", "you", "--keyboard", "us"]},
  "fineurl":  {"type": "http", "url": "https://mcp.example.com/mcp?version=2"}
}}`

	fs, err := Read(Place{Cursor, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	// One with another reason first keeps that reason, and its key is
	// taken out of what is listed all the same.
	if f := one(t, fs, "folder"); f.Problem != ProblemFolder {
		t.Errorf("folder: problem %q, want %q", f.Problem, ProblemFolder)
	}

	for _, name := range []string{"postgres", "flag", "joined", "shape", "query", "userinfo", "header"} {
		f := one(t, fs, name)

		if f.Problem != ProblemCredential {
			t.Errorf("%s: problem %q (%s), want %q", name, f.Problem, f.Detail, ProblemCredential)
		}

		if _, err := Imported(f, "x"+name); err == nil {
			t.Errorf("%s was imported", name)
		}
	}

	// Nothing that is shown, or fingerprinted, carries the key.
	shown, _ := json.Marshal(fs)

	for _, key := range []string{"hunter2zz", "zz-plain-value-1", "zz-plain-value-2", "ghp_zzzz", "zz-plain-value-3", "hunter3zz", "zz-plain-value-4", "zz-plain-value-5",
		// Whatever cannot be used is listed by its program's name alone:
		// its card shows no more, and a key may sit anywhere in the rest.
		"zz-plain-value-6", "zz-plain-value-7"} {
		if strings.Contains(string(shown), key) {
			t.Errorf("%s is in what is shown: %s", key, shown)
		}
	}

	for _, name := range []string{"fine", "fineurl"} {
		if f := one(t, fs, name); f.Problem != "" {
			t.Errorf("%s was refused: %s (%s)", name, f.Problem, f.Detail)
		}
	}

	for _, p := range []string{ProblemLoader, ProblemCredential} {
		if !strings.Contains(strings.Join(Problems(), " "), p) {
			t.Errorf("%s is not in Problems()", p)
		}
	}
}

// Nothing says an imported program stays on this machine, so it is
// treated as one that reaches out: named so, and held back while privacy
// keeps everything here.
func TestAnImportedProgramCountsAsReachingOut(t *testing.T) {
	raw := `{"mcpServers": {"github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"]}}}`

	fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	s := imported(t, one(t, fs, "github"), "xgithub")

	if !s.Network || !s.Remote() {
		t.Errorf("an imported program is described as staying on this machine: %+v", s)
	}
}
