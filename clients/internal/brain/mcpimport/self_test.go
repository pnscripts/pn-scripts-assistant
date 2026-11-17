package mcpimport

import "testing"

/*
 * The assistant's own entry in another program's settings.
 *
 * Somebody who lets Claude Desktop, Claude Code or Cursor ask the assistant
 * ("pn-scripts-assistant mcp") puts exactly such an entry there, and the
 * assistant then finds it like any other. Importing it would have the
 * assistant ask itself through another copy of itself: it is listed, with
 * that as the reason, and is never importable.
 */
func TestTheAssistantsOwnEntryIsListedAndNeverImported(t *testing.T) {
	fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(`{"mcpServers": {
		"plain":   {"command": "pn-scripts-assistant", "args": ["mcp"]},
		"full":    {"command": "/usr/bin/pn-scripts-assistant", "args": ["mcp", "--api", "http://127.0.0.1:8791"]},
		"windows": {"command": "C:\\Program Files\\PN Scripts Assistant\\PN-Scripts-Assistant.EXE", "args": ["mcp"]},
		"typed":   {"type": "stdio", "command": "pn-scripts-assistant", "args": ["mcp"]},
		"with a key": {"command": "pn-scripts-assistant", "args": ["mcp"], "env": {"API_TOKEN": "sk-live-0123456789abcdef0123"}},

		"another program": {"command": "pn-scripts-assistant-helper", "args": ["mcp"]},
		"another command": {"command": "pn-scripts-assistant", "args": ["status"]},
		"mcp later":       {"command": "npx", "args": ["some-server", "pn-scripts-assistant", "mcp"]}
	}}`), home)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"plain", "full", "windows", "typed", "with a key"} {
		f := one(t, fs, name)

		if f.Problem != ProblemSelf {
			t.Errorf("%s: problem %q (%s), want %q", name, f.Problem, f.Detail, ProblemSelf)
		}

		if _, err := Imported(f, "x"+name[:1]); err == nil {
			t.Errorf("%s was imported", name)
		}
	}

	for _, name := range []string{"another program", "another command", "mcp later"} {
		if f := one(t, fs, name); f.Problem == ProblemSelf {
			t.Errorf("%s was taken for the assistant itself", name)
		}
	}

	known := false

	for _, p := range Problems() {
		known = known || p == ProblemSelf
	}

	if !known {
		t.Errorf("%s is not in Problems()", ProblemSelf)
	}
}
