package mcpimport

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/mcp"
)

const home = "/home/you"

func one(t *testing.T, fs []Found, name string) Found {
	t.Helper()

	for _, f := range fs {
		if f.Name == name {
			return f
		}
	}

	t.Fatalf("%s not found in %+v", name, fs)

	return Found{}
}

func TestThePlacesAreWhereEachProgramKeepsItsSettings(t *testing.T) {
	linux := Places(Env{GOOS: "linux", Home: "/home/you"})
	want := []Place{
		{ClaudeDesktop, "/home/you/.config/Claude/claude_desktop_config.json"},
		{ClaudeCode, "/home/you/.claude.json"},
		{Cursor, "/home/you/.cursor/mcp.json"},
		{VSCode, "/home/you/.config/Code/User/mcp.json"},
	}

	if !reflect.DeepEqual(linux, want) {
		t.Errorf("linux %+v", linux)
	}

	xdg := Places(Env{GOOS: "linux", Home: "/home/you", ConfigHome: "/elsewhere", ClaudeConfigDir: "/cc"})
	if xdg[0].File != "/elsewhere/Claude/claude_desktop_config.json" || xdg[1].File != "/cc/.claude.json" ||
		xdg[3].File != "/elsewhere/Code/User/mcp.json" {
		t.Errorf("XDG_CONFIG_HOME and CLAUDE_CONFIG_DIR not followed: %+v", xdg)
	}

	win := Places(Env{GOOS: "windows", Home: `C:\Users\you`, AppData: `C:\Users\you\AppData\Roaming`})
	if win[0].File != filepath.Join(`C:\Users\you\AppData\Roaming`, "Claude", "claude_desktop_config.json") ||
		win[3].File != filepath.Join(`C:\Users\you\AppData\Roaming`, "Code", "User", "mcp.json") {
		t.Errorf("windows %+v", win)
	}

	mac := Places(Env{GOOS: "darwin", Home: "/Users/you"})
	if mac[0].File != "/Users/you/Library/Application Support/Claude/claude_desktop_config.json" {
		t.Errorf("mac %+v", mac)
	}

	if Places(Env{GOOS: "linux"}) != nil {
		t.Error("no home, no places")
	}
}

func TestClaudeDesktopServersAreRead(t *testing.T) {
	raw := `{
  "mcpServers": {
    "filesystem": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/you/Desktop"]},
    "brave-search": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-brave-search"],
                     "env": {"BRAVE_API_KEY": "abc123", "LOG_LEVEL": "info"}}
  },
  "globalShortcut": "Ctrl+Space"
}`

	fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
	if err != nil || len(fs) != 2 {
		t.Fatalf("%v %+v", err, fs)
	}

	files := one(t, fs, "filesystem")
	if files.Problem != "" || files.Server.Transport != "stdio" ||
		!reflect.DeepEqual(files.Server.Command, []string{"npx", "-y", "@modelcontextprotocol/server-filesystem", "/home/you/Desktop"}) {
		t.Errorf("filesystem %+v", files)
	}

	brave := one(t, fs, "brave-search")
	if !reflect.DeepEqual(brave.Server.Secrets, []string{"BRAVE_API_KEY"}) || brave.Secrets["BRAVE_API_KEY"] != "abc123" {
		t.Errorf("the key is not a secret: %+v", brave)
	}

	if brave.Server.Env["LOG_LEVEL"] != "info" || brave.Server.Env["BRAVE_API_KEY"] != "" {
		t.Errorf("settings %+v", brave.Server.Env)
	}

	// Nothing that is shown carries the key.
	shown, _ := json.Marshal(brave)
	if strings.Contains(string(shown), "abc123") {
		t.Errorf("the key is in what is shown: %s", shown)
	}
}

func TestClaudeCodeReadsTheUserAndEveryProjectList(t *testing.T) {
	raw := `{
  "numStartups": 12,
  "oauthAccount": {"emailAddress": "someone@example.org"},
  "mcpServers": {"everything": {"type": "stdio", "command": "npx", "args": ["-y", "@modelcontextprotocol/server-everything"]}},
  "projects": {
    "/home/you/site": {"mcpServers": {"stripe": {"type": "http", "url": "https://mcp.stripe.example/mcp", "headers": {"Authorization": "Bearer sk_live_x"}}}},
    "/home/you/empty": {"allowedTools": []}
  }
}`

	fs, err := Read(Place{ClaudeCode, "/home/you/.claude.json"}, []byte(raw), home)
	if err != nil || len(fs) != 2 {
		t.Fatalf("%v %+v", err, fs)
	}

	user := one(t, fs, "everything")
	if user.Scope != "" || user.Problem != "" {
		t.Errorf("user scope %+v", user)
	}

	stripe := one(t, fs, "stripe")
	if stripe.Scope != "/home/you/site" || stripe.Server.Transport != "http" || stripe.Server.URL != "https://mcp.stripe.example/mcp" {
		t.Errorf("local scope %+v", stripe)
	}

	if stripe.Server.Header != "Authorization" || !reflect.DeepEqual(stripe.Server.Secrets, []string{"HEADER_AUTHORIZATION"}) ||
		stripe.Secrets["HEADER_AUTHORIZATION"] != "Bearer sk_live_x" {
		t.Errorf("the header is not a secret: %+v", stripe)
	}

	if stripe.Key() == user.Key() {
		t.Error("two entries share a key")
	}
}

func TestCursorAndVSCodeAreRead(t *testing.T) {
	cursor := `{"mcpServers": {"gh": {"url": "https://api.example.com/mcp/"}, "local": {"type": "stdio", "command": "${userHome}/bin/server", "args": []}}}`

	fs, err := Read(Place{Cursor, "/home/you/.cursor/mcp.json"}, []byte(cursor), home)
	if err != nil || len(fs) != 2 {
		t.Fatalf("%v %+v", err, fs)
	}

	if gh := one(t, fs, "gh"); gh.Server.Transport != "http" || gh.Problem != "" {
		t.Errorf("an address with no type is http: %+v", gh)
	}

	if local := one(t, fs, "local"); local.Server.Command[0] != "/home/you/bin/server" || local.Problem != "" {
		t.Errorf("${userHome} not filled in: %+v", local)
	}

	// VS Code: comments, trailing commas, "servers", and inputs.
	vscode := `// my servers
{
  "inputs": [{"type": "promptString", "id": "token", "password": true}],
  "servers": {
    /* the one I use */
    "memory": {"type": "stdio", "command": "npx", "args": ["-y", "@modelcontextprotocol/server-memory",],},
    "github": {"type": "http", "url": "https://api.githubcopilot.example/mcp/", "headers": {"Authorization": "Bearer ${input:token}"}},
    "odd // name": {"type": "stdio", "command": "uvx", "args": ["thing", "a /* not a comment */ b"]},
  },
}`

	fs, err = Read(Place{VSCode, "/v.json"}, []byte(vscode), home)
	if err != nil || len(fs) != 3 {
		t.Fatalf("%v %+v", err, fs)
	}

	if m := one(t, fs, "memory"); m.Problem != "" || len(m.Server.Command) != 3 {
		t.Errorf("memory %+v", m)
	}

	if g := one(t, fs, "github"); g.Problem != ProblemInput || g.Detail != "${input:token}" {
		t.Errorf("a value VS Code asks for is not importable: %+v", g)
	}

	if o := one(t, fs, "odd // name"); o.Server.Command[2] != "a /* not a comment */ b" {
		t.Errorf("a comment inside a string was taken out: %+v", o.Server.Command)
	}
}

func TestWhatCannotRunHereIsListedWithTheReason(t *testing.T) {
	raw := `{"mcpServers": {
  "shell":   {"command": "bash", "args": ["-c", "npx thing"]},
  "winshell":{"command": "C:\\Windows\\System32\\cmd.exe", "args": ["/c", "npx", "thing"]},
  "envvar":  {"command": "npx", "args": ["${PROJECT_DIR:-.}/server"]},
  "envkey":  {"command": "npx", "args": ["x"], "env": {"API_KEY": "${env:API_KEY}"}},
  "envfile": {"type": "stdio", "command": "npx", "envFile": "${workspaceFolder}/.env"},
  "folder":  {"command": "node", "args": ["index.js"], "cwd": "/home/you/server"},
  "sse":     {"type": "sse", "url": "https://old.example.com/sse"},
  "ws":      {"type": "ws", "url": "wss://example.com/mcp"},
  "two":     {"type": "http", "url": "https://example.com/mcp", "headers": {"A": "1", "B": "2"}},
  "plain":   {"type": "http", "url": "http://example.com/mcp"},
  "nothing": {"disabled": true},
  "notanobject": 7
}}`

	fs, err := Read(Place{Cursor, "/c.json"}, []byte(raw), home)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"shell": ProblemShell, "winshell": ProblemShell, "envvar": ProblemVariable, "envkey": ProblemVariable,
		"envfile": ProblemEnvFile, "folder": ProblemFolder, "sse": ProblemSSE, "ws": ProblemTransport,
		"two": ProblemHeaders, "plain": ProblemInvalid, "nothing": ProblemNothing,
	}

	if len(fs) != len(want) {
		t.Fatalf("an entry that is not an object should be skipped, the rest kept: %+v", fs)
	}

	for name, problem := range want {
		if f := one(t, fs, name); f.Problem != problem {
			t.Errorf("%s: problem %q, want %q (%s)", name, f.Problem, problem, f.Detail)
		}

		if _, err := Imported(one(t, fs, name), "x"+strings.ReplaceAll(name, "_", "")); err == nil {
			t.Errorf("%s was imported", name)
		}
	}

	known := map[string]bool{}
	for _, p := range Problems() {
		known[p] = true
	}

	for name, problem := range want {
		if !known[problem] {
			t.Errorf("%s: %s is not in Problems()", name, problem)
		}
	}
}

func TestBrokenOrEmptySettingsSayNothing(t *testing.T) {
	if fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(`{"mcpServers": {"a": {"command": "x"}`), home); !errors.Is(err, ErrUnreadable) || fs != nil {
		t.Errorf("half a file: %v %+v", err, fs)
	}

	for _, raw := range []string{"", "  \n", "// only a comment\n", "{}", `{"mcpServers": null}`} {
		if fs, err := Read(Place{VSCode, "/v.json"}, []byte(raw), home); err != nil || len(fs) != 0 {
			t.Errorf("%q: %v %+v", raw, err, fs)
		}
	}
}

func TestSecretsAreToldBySomethingBesidesTheirName(t *testing.T) {
	for _, c := range []struct {
		name, value string
		secret      bool
	}{
		{"GITHUB_PERSONAL_ACCESS_TOKEN", "x", true},
		{"OPENAI_API_KEY", "x", true},
		{"DB_PASSWORD", "x", true},
		{"GITLAB_PAT", "x", true},
		{"DATABASE_URL", "postgres://me:hunter2@localhost/db", true},
		{"ANYTHING", "ghp_abcdef", true},
		{"ANYTHING", "sk-proj-abc", true},
		{"PATH", "/usr/bin", false},
		{"PASSTHROUGH_MODE", "on", false},
		{"LOG_LEVEL", "debug", false},
		{"ALLOWED_DIRS", "/home/you/Projects", false},
		{"DATABASE_URL", "postgres://localhost/db", false},
	} {
		if got := Secret(c.name, c.value); got != c.secret {
			t.Errorf("Secret(%s, %s) = %v", c.name, c.value, got)
		}
	}
}

func TestTheFingerprintIgnoresANewKeyButNotANewCommand(t *testing.T) {
	read := func(raw string) Found {
		fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(raw), home)
		if err != nil || len(fs) != 1 {
			t.Fatalf("%v %+v", err, fs)
		}

		return fs[0]
	}

	a := read(`{"mcpServers": {"s": {"command": "npx", "args": ["a@1"], "env": {"K_TOKEN": "one"}}}}`)
	b := read(`{"mcpServers": {"s": {"command": "npx", "args": ["a@1"], "env": {"K_TOKEN": "two"}}}}`)
	c := read(`{"mcpServers": {"s": {"command": "npx", "args": ["a@2"], "env": {"K_TOKEN": "one"}}}}`)
	d := read(`{"mcpServers": {"s": {"command": "npx", "args": ["a@1"], "env": {"K_TOKEN": "one", "MODE": "x"}}}}`)

	if a.Print != b.Print {
		t.Error("a new key changed the fingerprint")
	}

	if a.Print == c.Print || a.Print == d.Print {
		t.Error("a changed definition kept the fingerprint")
	}
}

func imported(t *testing.T, f Found, id string) mcp.Server {
	t.Helper()

	s, err := Imported(f, id)
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestFollowingAnImportedEntry(t *testing.T) {
	file := "/home/you/.config/Claude/claude_desktop_config.json"
	place := Place{ClaudeDesktop, file}

	read := func(raw string) []Found {
		fs, err := Read(place, []byte(raw), home)
		if err != nil {
			t.Fatal(err)
		}

		return fs
	}

	first := read(`{"mcpServers": {"files": {"command": "npx", "args": ["fs@1"], "env": {"X_TOKEN": "k1"}}}}`)
	s := imported(t, first[0], "files")
	s.Approved, s.Active = true, true

	kept := map[string]string{"X_TOKEN": "k1"}
	secretsOf := func(string) map[string]string { return kept }

	all := map[string]bool{file: true}

	// Unchanged: nothing to do.
	if steps := Follow([]mcp.Server{s}, first, all, secretsOf); len(steps) != 0 {
		t.Errorf("unchanged: %+v", steps)
	}

	// The file could not be read: nothing is concluded.
	if steps := Follow([]mcp.Server{s}, nil, map[string]bool{}, secretsOf); len(steps) != 0 {
		t.Errorf("unreadable file: %+v", steps)
	}

	// Removed from the settings (or the file deleted, which reads as empty).
	if steps := Follow([]mcp.Server{s}, nil, all, secretsOf); len(steps) != 1 || steps[0].Kind != StepGone || steps[0].ID != "files" {
		t.Errorf("removed: %+v", steps)
	}

	// Already gone and still gone: nothing new.
	gone := s
	gone.From = &mcp.Origin{}
	*gone.From = *s.From
	gone.From.GoneSince = "2026-11-14T08:00:00Z"

	if steps := Follow([]mcp.Server{gone}, nil, all, secretsOf); len(steps) != 0 {
		t.Errorf("still gone: %+v", steps)
	}

	// Back unchanged.
	if steps := Follow([]mcp.Server{gone}, first, all, secretsOf); len(steps) != 1 || steps[0].Kind != StepBack {
		t.Errorf("back: %+v", steps)
	}

	// A new key only.
	newKey := read(`{"mcpServers": {"files": {"command": "npx", "args": ["fs@1"], "env": {"X_TOKEN": "k2"}}}}`)
	if steps := Follow([]mcp.Server{s}, newKey, all, secretsOf); len(steps) != 1 || steps[0].Kind != StepSecrets || steps[0].Secrets["X_TOKEN"] != "k2" {
		t.Errorf("new key: %+v", steps)
	}

	// Changed: the new definition, keeping the id and the origin.
	changed := read(`{"mcpServers": {"files": {"command": "npx", "args": ["fs@2"], "env": {"X_TOKEN": "k1"}}}}`)
	steps := Follow([]mcp.Server{s}, changed, all, secretsOf)

	if len(steps) != 1 || steps[0].Kind != StepChanged || steps[0].Next.ID != "files" || steps[0].Next.Command[1] != "fs@2" ||
		steps[0].Next.From == nil || steps[0].Next.From.Print != changed[0].Print {
		t.Errorf("changed: %+v", steps)
	}

	// Changed into something that cannot run here: gone, not replaced.
	shell := read(`{"mcpServers": {"files": {"command": "bash", "args": ["-c", "npx fs"]}}}`)
	if steps := Follow([]mcp.Server{s}, shell, all, secretsOf); len(steps) != 1 || steps[0].Kind != StepGone {
		t.Errorf("changed into a shell: %+v", steps)
	}

	// One added by hand is never followed.
	own := mcp.Server{ID: "own", Transport: "stdio", Command: []string{"x"}}
	if steps := Follow([]mcp.Server{own}, nil, all, secretsOf); len(steps) != 0 {
		t.Errorf("own: %+v", steps)
	}
}

func TestIdsAreFreeAndValid(t *testing.T) {
	taken := map[string]bool{"filesystem": true, "filesystem2": true}
	is := func(id string) bool { return taken[id] }

	if got := FreeID("filesystem", is); got != "filesystem3" {
		t.Errorf("got %s", got)
	}

	long := "a-really-long-server-name-that-goes-on"
	first := FreeID(long, func(string) bool { return false })
	taken[first] = true

	second := FreeID(long, is)
	if second == first || len(second) > 21 {
		t.Errorf("%s then %s", first, second)
	}

	for _, id := range []string{first, second, FreeID("42", is), FreeID("@scope/my-mcp-server", is)} {
		if err := (mcp.Server{ID: id, Transport: "stdio", Command: []string{"x"}}).Check(); err != nil {
			t.Errorf("%q: %v", id, err)
		}
	}
}

func TestStripLeavesPlainJSONAlone(t *testing.T) {
	plain := `{"a": "x // y", "b": [1, 2], "c": "q\"/*"}`
	if got := string(Strip([]byte(plain))); got != plain {
		t.Errorf("got %s", got)
	}

	if got := string(Strip([]byte(`{"a": [1, 2, /* c */ ], }`))); !json.Valid([]byte(got)) {
		t.Errorf("trailing commas with a comment: %s", got)
	}
}
