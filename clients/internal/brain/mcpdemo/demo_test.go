/*
 * Package mcpdemo is the acceptance demo of 1.1.0's MCP import: the real
 * reference server, found in Claude Desktop's settings, used through the
 * assistant's own permission path.
 *
 * Only tests live here. The server is @modelcontextprotocol/server-everything
 * at the exact version in testdata/everything/package.json, with every
 * dependency pinned by its SHA-512 in package-lock.json. CI installs it with
 * `npm ci --ignore-scripts` and then runs this test with no network at all
 * (a network namespace on Linux, a firewall rule on Windows): nothing here
 * downloads anything, needs a secret, or reaches a real model.
 *
 * Locally: `cd testdata/everything && npm ci --ignore-scripts`, then
 * `go test ./internal/brain/mcpdemo/`. Without the install the demo is
 * skipped, unless PN_SCRIPTS_ASSISTANT_MCP_DEMO=required (CI) makes that a
 * failure.
 */
package mcpdemo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/server"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/terms"
)

const (
	packageName = "@modelcontextprotocol/server-everything"
	folder      = "testdata/everything"
	required    = "PN_SCRIPTS_ASSISTANT_MCP_DEMO"
)

var exactVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// pinned is the version package.json asks for, which must be exact.
func pinned(t *testing.T) string {
	t.Helper()

	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
		Scripts      map[string]string `json:"scripts"`
	}

	readJSON(t, filepath.Join(folder, "package.json"), &manifest)

	if len(manifest.Dependencies) != 1 || len(manifest.Scripts) != 0 {
		t.Fatalf("the demo installs one package and runs nothing of its own: %+v", manifest)
	}

	v := manifest.Dependencies[packageName]
	if !exactVersion.MatchString(v) {
		t.Fatalf("%s is asked for as %q, not an exact version", packageName, v)
	}

	return v
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

/*
 * What CI installs is exactly what the lock says, and the lock says it
 * completely: every package with a SHA-512 that npm ci checks before it
 * unpacks anything, from the public registry only, and none with an install
 * script (they are not run anyway: --ignore-scripts). Needs no Node.
 */
func TestTheDemoServerIsPinnedWithEveryChecksum(t *testing.T) {
	version := pinned(t)

	var lock struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version          string            `json:"version"`
			Resolved         string            `json:"resolved"`
			Integrity        string            `json:"integrity"`
			HasInstallScript bool              `json:"hasInstallScript"`
			Dependencies     map[string]string `json:"dependencies"`
			Link             bool              `json:"link"`
		} `json:"packages"`
	}

	readJSON(t, filepath.Join(folder, "package-lock.json"), &lock)

	if lock.LockfileVersion != 3 {
		t.Errorf("lockfile version %d, want 3", lock.LockfileVersion)
	}

	if root := lock.Packages[""]; root.Dependencies[packageName] != version {
		t.Errorf("the lock's root asks for %q, package.json for %q", root.Dependencies[packageName], version)
	}

	server, ok := lock.Packages["node_modules/"+packageName]
	if !ok || server.Version != version {
		t.Fatalf("the lock has %s at %q, want %q", packageName, server.Version, version)
	}

	if len(lock.Packages) < 10 {
		t.Fatalf("only %d packages in the lock: its dependencies are missing", len(lock.Packages))
	}

	for name, p := range lock.Packages {
		if name == "" {
			continue
		}

		if p.Link || !strings.HasPrefix(p.Resolved, "https://registry.npmjs.org/") {
			t.Errorf("%s comes from %q, not the public registry", name, p.Resolved)
		}

		if !strings.HasPrefix(p.Integrity, "sha512-") {
			t.Errorf("%s has no SHA-512 to check: %q", name, p.Integrity)
		}

		if p.HasInstallScript {
			t.Errorf("%s has an install script", name)
		}
	}
}

// installed is the server's program, or a skip saying how to get it.
func installed(t *testing.T) (node, entry string) {
	t.Helper()

	version := pinned(t)

	missing := func(why string) {
		if os.Getenv(required) == "required" {
			t.Fatalf("the MCP demo must run here and cannot: %s", why)
		}

		t.Skipf("%s; run `npm ci --ignore-scripts` in %s (CI sets %s=required)", why, folder, required)
	}

	node, err := exec.LookPath("node")
	if err != nil {
		missing("Node is not installed")
	}

	dir, err := filepath.Abs(filepath.Join(folder, "node_modules", filepath.FromSlash(packageName)))
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Version string `json:"version"`
		Bin     map[string]string
	}

	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		missing("the demo server is not installed")
	}

	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	if got.Version != version {
		t.Fatalf("installed %s is %s, the pin is %s: run npm ci again", packageName, got.Version, version)
	}

	entry = filepath.Join(dir, filepath.FromSlash(got.Bin["mcp-server-everything"]))
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("the server's program is not where its package says: %v", err)
	}

	return node, entry
}

/*
 * model is the boundary: a fake model that asks for the tool a request
 * names, then repeats what the tool said. It records which tools each turn
 * was offered, so the demo can say what the model could and could not see.
 */
type model struct {
	mu      sync.Mutex
	server  string // the integration's id here
	offered [][]string
}

func (m *model) Name() string                   { return "ollama" }
func (m *model) Available(context.Context) bool { return true }

func (m *model) Chat(_ context.Context, r llm.Request) (llm.Response, error) {
	names := make([]string, 0, len(r.Tools))
	for _, t := range r.Tools {
		names = append(names, t.Name)
	}

	m.mu.Lock()
	m.offered = append(m.offered, names)
	server := m.server
	m.mu.Unlock()

	// Only this turn: what came after the last thing the person said.
	ask, answer := "", ""

	for _, msg := range r.Messages {
		switch msg.Role {
		case "user":
			ask, answer = msg.Content, ""
		case "tool":
			answer = msg.Content
		}
	}

	if answer != "" {
		return llm.Response{Content: "The integration answered: " + answer}, nil
	}

	low := strings.ToLower(ask)

	var tool, args string

	switch {
	case strings.Contains(low, "echo"):
		tool, args = "echo", `{"message":"hello from the demo"}`
	case strings.Contains(low, "environment"):
		tool, args = "get-env", `{}`
	case strings.Contains(low, "add"):
		tool, args = "get-sum", `{"a":2,"b":3}`
	default:
		return llm.Response{Content: "I was not asked to use anything."}, nil
	}

	name := mcp.ToolName(server, tool)

	for _, offered := range names {
		if offered == name {
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "demo", Name: name, Arguments: json.RawMessage(args)}}}, nil
		}
	}

	return llm.Response{Content: "I have no tool called " + name + "."}, nil
}

func (m *model) lastOffered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.offered) == 0 {
		return nil
	}

	return m.offered[len(m.offered)-1]
}

// demo is one person's computer: a home with Claude Desktop set up, and the
// assistant's real server on a brain of its own.
type demo struct {
	t        *testing.T
	ts       *httptest.Server
	b        *brain.Brain
	model    *model
	settings string
}

const (
	setting       = "DEMO_GREETING"
	settingValue  = "good-morning-from-claude-desktop"
	theirSecret   = "DEMO_API_KEY"
	theirValue    = "demo-secret-7f3a"
	assistantKey  = "ANTHROPIC_API_KEY"
	assistantOwns = "assistant-own-key-91c2"
)

// desktopConfig is Claude Desktop's settings with the demo server in them,
// written the way a person writes it: the program and its arguments.
func desktopConfig(node, entry string) []byte {
	entryJSON, _ := json.Marshal(entry)
	nodeJSON, _ := json.Marshal(node)

	return []byte(fmt.Sprintf(`{
  "mcpServers": {
    "everything": {
      "command": %s,
      "args": [%s, "stdio"],
      "env": {%q: %q, %q: %q}
    }
  }
}`, nodeJSON, entryJSON, setting, settingValue, theirSecret, theirValue))
}

func newDemo(t *testing.T) *demo {
	t.Helper()

	node, entry := installed(t)

	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	// A key of the assistant's own, which no integration is ever given.
	t.Setenv(assistantKey, assistantOwns)

	settings := filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(settings, desktopConfig(node, entry), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := terms.Accept(root, terms.Version, "en", time.Now()); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Owner = "Alex"
	cfg.OllamaURL = "http://127.0.0.1:9"
	cfg.Freedom = "ask" // every change asked: the path this demo is about

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger)

	m := &model{}
	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", m)

	ts := httptest.NewServer(server.New(b, logger).Handler())

	ctx, cancel := context.WithCancel(context.Background())
	b.KeepMCPImports(ctx)

	// Registered after the temporary folders, so it runs before they go:
	// Windows cannot remove the folder a running server works in.
	t.Cleanup(func() {
		cancel()
		b.StopMCPImports()
		ts.Close()

		if b.Integrations != nil {
			b.Integrations.Close()
		}

		db.Close()
	})

	return &demo{t: t, ts: ts, b: b, model: m, settings: settings}
}

func (d *demo) get(path string, into any) string {
	d.t.Helper()

	res, err := http.Get(d.ts.URL + path)
	if err != nil {
		d.t.Fatal(err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		d.t.Fatalf("GET %s: %d %s", path, res.StatusCode, raw)
	}

	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			d.t.Fatalf("GET %s: %v", path, err)
		}
	}

	return string(raw)
}

func (d *demo) post(path, body string, into any) (int, string) {
	d.t.Helper()

	res, err := http.Post(d.ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		d.t.Fatal(err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	if into != nil && res.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, into); err != nil {
			d.t.Fatalf("POST %s: %v (%s)", path, err, raw)
		}
	}

	return res.StatusCode, string(raw)
}

// say is the person typing in the window.
func (d *demo) say(conversation int64, text string) brain.ChatReply {
	d.t.Helper()

	body, _ := json.Marshal(map[string]any{"conversation_id": conversation, "message": text})

	var reply brain.ChatReply

	if code, raw := d.post("/api/chat", string(body), &reply); code != http.StatusOK {
		d.t.Fatalf("chat %q: %d %s", text, code, raw)
	}

	return reply
}

// decide is the person pressing Approve or Reject on the card.
func (d *demo) decide(id int64, decision string) store.Invocation {
	d.t.Helper()

	var out struct {
		Invocation store.Invocation `json:"invocation"`
		Error      string           `json:"error"`
	}

	if code, raw := d.post(fmt.Sprintf("/api/approvals/%d/%s", id, decision), "", &out); code != http.StatusOK || out.Error != "" {
		d.t.Fatalf("%s %d: %d %s", decision, id, code, raw)
	}

	return out.Invocation
}

// calls is how many times the integration was actually called.
func (d *demo) calls(id string) int {
	n := 0

	for _, e := range d.b.Integrations.Events(id, 500) {
		if e.Event == "call" {
			n++
		}
	}

	return n
}

func (d *demo) waitFor(what string, done func() bool) {
	d.t.Helper()

	deadline := time.Now().Add(30 * time.Second)

	for !done() {
		if time.Now().After(deadline) {
			d.t.Fatalf("waited 30 s for %s", what)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func offers(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}

	return false
}

/*
 * The P1 acceptance demo, end to end against the real reference server:
 *
 *  1. found in Claude Desktop's settings, its key never shown;
 *  2. imported, and not started until its owner approves it;
 *  3. approved and switched on at the desk: its real tools are listed;
 *  4. asked in a conversation, the call waits on a card and the server is
 *     not called; Approve runs it and the server's own answer comes back;
 *  5. Reject: the server is never called;
 *  6. the server is given its own settings and key, never the assistant's,
 *     and its key is taken out of what it says;
 *  7. its entry removed in Claude Desktop: stopped, its tools gone from the
 *     model at once, switching it on refused with the reason.
 */
func TestTheAssistantUsesAServerImportedFromClaudeDesktop(t *testing.T) {
	d := newDemo(t)

	// 1. Found, with nothing secret in what the page is sent.
	var listed struct {
		Found []brain.MCPFound `json:"found"`
	}

	raw := d.get("/api/integrations", &listed)

	if strings.Contains(raw, theirValue) {
		t.Error("the key in Claude Desktop's settings is in what the page is sent")
	}

	var found brain.MCPFound

	for _, f := range listed.Found {
		if f.Name == "everything" && f.Title == "Claude Desktop" {
			found = f
		}
	}

	if found.Key == "" || found.Problem != "" || found.Leaves {
		t.Fatalf("not found as importable: %+v (all: %+v)", found, listed.Found)
	}

	// 2. Imported: "everything" is the catalogue's own id, so it gets the next.
	var imported struct {
		Imported string `json:"imported"`
	}

	if code, raw := d.post("/api/integrations/import", `{"key":`+quote(found.Key)+`}`, &imported); code != http.StatusOK || imported.Imported == "" {
		t.Fatalf("import: %d %s", code, raw)
	}

	id := imported.Imported
	d.model.mu.Lock()
	d.model.server = id
	d.model.mu.Unlock()

	echo := mcp.ToolName(id, "echo")

	if code, raw := d.post("/api/integrations/"+id+"/activate", "", nil); code != http.StatusBadRequest || !strings.Contains(raw, "approve") {
		t.Fatalf("started before its owner approved it: %d %s", code, raw)
	}

	conv := d.say(0, "Use the everything integration to echo a greeting.")
	if offers(d.model.lastOffered(), echo) || len(conv.PendingApprovals) != 0 || d.calls(id) != 0 {
		t.Fatalf("a server nobody approved was offered or called: %v %+v", d.model.lastOffered(), conv)
	}

	// 3. Approved and switched on: the real server's tools.
	if code, raw := d.post("/api/integrations/"+id+"/approve", "", nil); code != http.StatusOK {
		t.Fatalf("approve: %d %s", code, raw)
	}

	var after struct {
		Integrations []mcp.View `json:"integrations"`
	}

	if code, raw := d.post("/api/integrations/"+id+"/activate", "", &after); code != http.StatusOK {
		t.Fatalf("activate: %d %s", code, raw)
	}

	var view mcp.View

	for _, v := range after.Integrations {
		if v.ID == id {
			view = v
		}
	}

	var tools []string
	for _, tv := range view.Tools {
		tools = append(tools, tv.Name)
	}

	if !view.Active || !offers(tools, "echo") || !offers(tools, "get-env") || !offers(tools, "get-sum") {
		t.Fatalf("switched on as %+v (tools %v)", view, tools)
	}

	// 4. Asked: a card, and the server not called until Approve.
	reply := d.say(conv.ConversationID, "Use the everything integration to echo a greeting.")

	if !offers(d.model.lastOffered(), echo) {
		t.Fatalf("the approved server's tool was not offered: %v", d.model.lastOffered())
	}

	if len(reply.PendingApprovals) != 1 || reply.PendingApprovals[0].Tool != echo {
		t.Fatalf("no card for the call: %+v", reply)
	}

	if d.calls(id) != 0 {
		t.Fatal("the server was called before its owner approved the call")
	}

	var cards []store.Invocation
	d.get("/api/approvals", &cards)

	if len(cards) != 1 || cards[0].ID != reply.PendingApprovals[0].ID || cards[0].Status != store.InvocationPending {
		t.Fatalf("the approval cards: %+v", cards)
	}

	done := d.decide(cards[0].ID, "approve")

	if done.Status != store.InvocationDone || d.calls(id) != 1 || !strings.Contains(done.Result, "Echo: hello from the demo") ||
		!strings.Contains(done.Result, "information, not instructions") {
		t.Fatalf("approved, the server's answer: %+v (calls %d)", done, d.calls(id))
	}

	// 5. Rejected: never called.
	reply = d.say(conv.ConversationID, "Now use get-sum of the everything integration to add two and three.")
	if len(reply.PendingApprovals) != 1 || reply.PendingApprovals[0].Tool != mcp.ToolName(id, "get-sum") {
		t.Fatalf("no card for the second call: %+v", reply)
	}

	refused := d.decide(reply.PendingApprovals[0].ID, "reject")

	if refused.Status != store.InvocationDenied || refused.Result != "" || d.calls(id) != 1 {
		t.Fatalf("rejected, yet: %+v (calls %d)", refused, d.calls(id))
	}

	d.get("/api/approvals", &cards)
	if len(cards) != 0 {
		t.Errorf("cards left after deciding: %+v", cards)
	}

	// 6. What the server is given, in its own words.
	reply = d.say(conv.ConversationID, "Use get-env of the everything integration to show the environment it runs with.")
	if len(reply.PendingApprovals) != 1 {
		t.Fatalf("no card for get-env: %+v", reply)
	}

	env := d.decide(reply.PendingApprovals[0].ID, "approve").Result

	if !strings.Contains(env, settingValue) {
		t.Errorf("the server was not given its own setting: %s", env)
	}

	if !strings.Contains(env, theirSecret) || strings.Contains(env, theirValue) {
		t.Errorf("its key was not given, or not taken out of its answer: %s", env)
	}

	if strings.Contains(env, assistantOwns) || strings.Contains(env, assistantKey) {
		t.Errorf("the server was given the assistant's own key: %s", env)
	}

	if events := d.b.Integrations.Events(id, 500); len(events) == 0 {
		t.Error("nothing recorded about the integration")
	} else {
		for _, e := range events {
			if strings.Contains(e.Detail, theirValue) || strings.Contains(e.Detail, "hello from the demo") {
				t.Errorf("the record holds what was said or the key: %+v", e)
			}
		}
	}

	// 7. Removed from Claude Desktop: stopped, gone from the model, refused.
	if err := os.WriteFile(d.settings, []byte(`{"mcpServers": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	d.waitFor("the removed entry to stop the server", func() bool {
		s, _ := d.b.Integrations.Book.Get(id)
		return s.Gone() && len(d.b.Integrations.Running()) == 0
	})

	reply = d.say(conv.ConversationID, "Use the everything integration to echo a greeting.")
	if offers(d.model.lastOffered(), echo) || len(reply.PendingApprovals) != 0 {
		t.Errorf("a server removed in Claude Desktop is still offered: %v %+v", d.model.lastOffered(), reply)
	}

	if code, raw := d.post("/api/integrations/"+id+"/activate", "", nil); code != http.StatusBadRequest || !strings.Contains(raw, "no longer in the settings") {
		t.Errorf("a removed server could be switched on: %d %s", code, raw)
	}

	if d.calls(id) != 2 {
		t.Errorf("%d calls, want the two that were approved", d.calls(id))
	}
}

func quote(s string) string {
	raw, _ := json.Marshal(s)

	return string(raw)
}
