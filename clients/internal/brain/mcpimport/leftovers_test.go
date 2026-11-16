package mcpimport

import (
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/mcp"
)

/*
 * Leftovers of the 1.1.0 security review.
 *
 * F11: a setting taken for a key only by the look of its value ("sk-...",
 * "Bearer ...") under a name that says nothing was left out of the
 * fingerprint like a real key, so it could be changed to anything that
 * still looked like one without a new approval.
 *
 * Q3: a key after "-p", and a flag named in camelCase (--accessToken=...),
 * were not seen. Q4: flags that only share a word with a credential
 * (--token-limit, --key-bindings, --pass-through) and a description that
 * mentions bearer tokens were refused.
 */
func TestAValueThatOnlyLooksLikeAKeyIsPartOfTheFingerprint(t *testing.T) {
	read := func(env string) Found {
		t.Helper()

		raw := `{"mcpServers": {"x": {"command": "npx", "args": ["-y", "server-x"], "env": ` + env + `}}}`

		fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
		if err != nil {
			t.Fatal(err)
		}

		return one(t, fs, "x")
	}

	a, b := read(`{"EXTRA_ARGS": "sk-first-value-zz"}`), read(`{"EXTRA_ARGS": "sk-second-value-zz"}`)

	if a.Problem != "" || len(a.Server.Secrets) != 1 || a.Server.Env["EXTRA_ARGS"] != "" {
		t.Fatalf("a value shaped like a key is not kept as a secret: %+v", a)
	}

	if a.Print == b.Print {
		t.Error("a setting named for nothing, hidden for the look of its value, changed without changing the fingerprint")
	}

	if again := read(`{"EXTRA_ARGS": "sk-first-value-zz"}`); again.Print != a.Print {
		t.Error("the same entry has two fingerprints")
	}

	// A key under a name that says it is one can be renewed as before.
	c, d := read(`{"GITHUB_TOKEN": "ghp_firstzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`), read(`{"GITHUB_TOKEN": "ghp_secondzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}`)
	if c.Print != d.Print {
		t.Error("a renewed key under a key's name changed the fingerprint")
	}

	// Nothing shown carries the value, the fingerprint included.
	shown, _ := json.Marshal(a)
	if strings.Contains(string(shown), "first-value") {
		t.Errorf("the value is in what is shown: %s", shown)
	}

	// Following an imported entry: the changed value is a changed entry,
	// which is asked about again, not a renewed key.
	was := imported(t, a, "xfirst")

	steps := Follow([]mcp.Server{was}, []Found{b}, map[string]bool{"/c.json": true},
		func(string) map[string]string { return a.Secrets })
	if len(steps) != 1 || steps[0].Kind != StepChanged {
		t.Errorf("steps %+v, want one %q", steps, StepChanged)
	}

	steps = Follow([]mcp.Server{imported(t, c, "xsecond")}, []Found{d}, map[string]bool{"/c.json": true},
		func(string) map[string]string { return c.Secrets })
	if len(steps) != 1 || steps[0].Kind != StepSecrets {
		t.Errorf("steps %+v, want one %q", steps, StepSecrets)
	}
}

func TestKeysAfterAShortFlagOrACamelCaseFlagAreSeen(t *testing.T) {
	raw := `{"mcpServers": {
  "short":      {"command": "npx", "args": ["-y", "server-x", "-p", "zz-plain-value-1"]},
  "camel":      {"command": "npx", "args": ["-y", "server-x", "--accessToken=zz-plain-value-2"]},
  "camelnext":  {"command": "npx", "args": ["-y", "server-x", "--apiKey", "zz-plain-value-3"]},
  "camelsec":   {"command": "node", "args": ["server.js", "--clientSecret", "zz-plain-value-4"]},
  "sshpass":    {"command": "sshpass", "args": ["-p", "zz-plain-value-5", "ssh", "host", "mcp-server"]},
  "upper":      {"command": "npx", "args": ["-y", "server-x", "--API_KEY=zz-plain-value-6"]},
  "bearer":     {"command": "npx", "args": ["-y", "server-x", "--header", "X-Auth: bearer zzPlainValue7zz"]}
}}`

	fs, err := Read(Place{Cursor, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"short", "camel", "camelnext", "camelsec", "sshpass", "upper", "bearer"} {
		f := one(t, fs, name)

		if f.Problem != ProblemCredential {
			t.Errorf("%s: problem %q (%s), want %q", name, f.Problem, f.Detail, ProblemCredential)
		}

		if _, err := Imported(f, "x"+name); err == nil {
			t.Errorf("%s was imported", name)
		}
	}

	shown, _ := json.Marshal(fs)
	if strings.Contains(string(shown), "zz-plain-value") || strings.Contains(string(shown), "zzPlainValue") {
		t.Errorf("a key is in what is shown: %s", shown)
	}
}

func TestFlagsThatOnlyShareAWordWithAKeyAreNotRefused(t *testing.T) {
	raw := `{"mcpServers": {
  "limit":      {"command": "npx", "args": ["-y", "server-x", "--token-limit", "4096"]},
  "limitcamel": {"command": "npx", "args": ["-y", "server-x", "--tokenLimit=4096", "--maxTokens", "512"]},
  "bindings":   {"command": "npx", "args": ["-y", "server-x", "--key-bindings", "vim"]},
  "through":    {"command": "npx", "args": ["-y", "server-x", "--pass-through", "stdio"]},
  "described":  {"command": "npx", "args": ["-y", "server-x", "--description", "Signs in with bearer tokens and keeps nothing"]},
  "switch":     {"command": "npx", "args": ["-y", "server-x", "--auth", "true", "--no-auth-cache", "--secret-length", "32"]},
  "package":    {"command": "npx", "args": ["-y", "-p", "@example/server-x", "server-x"]},
  "port":       {"command": "node", "args": ["server.js", "-p", "8080"]},
  "project":    {"command": "node", "args": ["server.js", "-p", "./tsconfig.json"]},
  "python":     {"command": "uvx", "args": ["-p", "3.12", "server-y"]}
}}`

	fs, err := Read(Place{Cursor, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"limit", "limitcamel", "bindings", "through", "described", "switch", "package", "port", "project", "python"} {
		f := one(t, fs, name)

		if f.Problem != "" {
			t.Errorf("%s was refused: %s (%s)", name, f.Problem, f.Detail)
		}

		if _, err := Imported(f, "x"+name); err != nil {
			t.Errorf("%s cannot be imported: %v", name, err)
		}
	}
}
