package mcpimport

import (
	"encoding/json"
	"strings"
	"testing"
)

/*
 * Notes of the read after 1.2.0 (ASSISTANT-PROGRAM-USE-NOTES-1121, 5).
 *
 * A setting was hidden as a key whenever its name held a key's word
 * anywhere, and a hidden setting under such a name was left out of the
 * fingerprint. So a place, an address or a command under a name that only
 * mentions a key (AUTH_URL, TOKEN_ENDPOINT, KEY_FILE, API_KEY_HELPER,
 * TOKEN_COMMAND, GIT_CONFIG_KEY_0, SSLKEYLOGFILE, XAUTHORITY) could be
 * changed in the other program's settings with the approval kept and
 * nothing shown. And a key inside a value under a neutral name (an
 * address with ?api_key=, a connection string with Password=) was shown
 * on the page as a setting.
 */
func TestANameThatOnlyMentionsAKeyIsShownAndFollowed(t *testing.T) {
	for _, c := range []struct {
		name, value string
		secret      bool
	}{
		// Places, addresses and commands: shown.
		{"AUTH_URL", "https://login.example.test/authorize", false},
		{"TOKEN_ENDPOINT", "https://login.example.test/token", false},
		{"KEY_FILE", "/home/you/.config/server/key.pem", false},
		{"API_KEY_HELPER", "/home/you/bin/print-key", false},
		{"TOKEN_COMMAND", "pass show server", false},
		{"GIT_CONFIG_KEY_0", "core.sshCommand", false},
		{"SSLKEYLOGFILE", "/tmp/keys.log", false},
		{"XAUTHORITY", "/run/user/1000/xauth", false},
		{"SSH_AUTH_SOCK", "/run/user/1000/keyring/ssh", false},
		{"GOOGLE_APPLICATION_CREDENTIALS", "/home/you/creds.json", false},
		{"PASSWORD_STORE_DIR", "/home/you/.password-store", false},
		{"NO_AUTH", "1", false},

		// Keys by their name: hidden, as before.
		{"API_KEY", "x", true},
		{"OPENAI_API_KEY", "x", true},
		{"GITHUB_TOKEN", "x", true},
		{"AUTH_TOKEN", "x", true},
		{"ACCESS_TOKEN", "x", true},
		{"CLIENT_SECRET", "x", true},
		{"DB_PASS", "x", true},
		{"PASSWORD", "x", true},
		{"AUTH", "x", true},
		{"APIKEY", "x", true},
		{"BEARER", "x", true},
		{"AUTH_HEADER", "Authorization: Bearer x", true},

		// A key inside a value under a name that says nothing: hidden.
		{"SERVER_URL", "https://api.example.test/v1?api_key=abc123", true},
		{"ENDPOINT", "https://api.example.test/v1?token=abc123", true},
		{"CONNECTION", "Server=db;Database=app;User Id=me;Password=hunter2;", true},
		{"DSN", "host=db user=me password=hunter2 dbname=app", true},
		{"AUTH_URL", "https://me:hunter2@login.example.test/", true},
		{"TOKEN_ENDPOINT", "https://login.example.test/token?client_secret=abc", true},

		// Still plain.
		{"SERVER_URL", "https://api.example.test/v1?page=2", false},
		{"CONNECTION", "Server=db;Database=app;", false},
	} {
		if got := Secret(c.name, c.value); got != c.secret {
			t.Errorf("Secret(%s, %q) = %v, want %v", c.name, c.value, got, c.secret)
		}
	}
}

func TestAChangedPlaceUnderAKeysWordIsANewDefinition(t *testing.T) {
	read := func(env map[string]string) Found {
		t.Helper()

		e, _ := json.Marshal(env)
		raw := `{"mcpServers": {"x": {"command": "npx", "args": ["-y", "server-x@1.0.0"], "env": ` + string(e) + `}}}`

		fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
		if err != nil {
			t.Fatal(err)
		}

		return one(t, fs, "x")
	}

	for _, name := range []string{"AUTH_URL", "TOKEN_ENDPOINT", "KEY_FILE", "API_KEY_HELPER", "TOKEN_COMMAND",
		"GIT_CONFIG_KEY_0", "SSLKEYLOGFILE", "XAUTHORITY"} {
		a := read(map[string]string{name: "/first/place"})
		b := read(map[string]string{name: "/second/place"})

		if a.Print == b.Print {
			t.Errorf("%s changed and the approval would stay", name)
		}

		if a.Server.Env[name] != "/first/place" || len(a.Server.Secrets) != 0 {
			t.Errorf("%s is not shown as a setting: %+v", name, a.Server)
		}
	}

	// A key inside a value is hidden, and a changed one is still a change:
	// its name says nothing, so its value counts, as a digest.
	a := read(map[string]string{"SERVER_URL": "https://api.example.test/v1?api_key=first-zz"})
	b := read(map[string]string{"SERVER_URL": "https://api.example.test/v1?api_key=second-zz"})

	if len(a.Server.Secrets) != 1 || a.Server.Env["SERVER_URL"] != "" {
		t.Fatalf("a key inside an address is shown: %+v", a.Server)
	}

	if a.Print == b.Print {
		t.Error("a hidden address changed without changing the fingerprint")
	}

	shown, _ := json.Marshal(a)
	if strings.Contains(string(shown), "first-zz") {
		t.Errorf("the key is in what is shown: %s", shown)
	}

	// A renewed key under a key's own name still keeps the approval.
	c, d := read(map[string]string{"API_KEY": "first"}), read(map[string]string{"API_KEY": "second"})
	if c.Print != d.Print {
		t.Error("a renewed key changed the fingerprint")
	}
}
