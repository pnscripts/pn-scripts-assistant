package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
)

// Only a short word for a kind of tool is ever sent to a registry.
func TestOnlyAShortWordGoesToARegistry(t *testing.T) {
	for _, ok := range []string{"sqlite", "PDF tools", "git", "time zones"} {
		if _, err := SearchWord(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}

	for _, bad := range []string{"", "make me a space strategy game about my uncle", "/home/alex/notes",
		"база данни", "alex@example.com", "a", "sqlite; rm -rf"} {
		if w, err := SearchWord(bad); err == nil {
			t.Errorf("%q would be sent as %q", bad, w)
		}
	}
}

const registryPage = `{"servers":[
 {"server":{"name":"io.github.stranger/sqlite-mcp","description":"SQLite","version":"1.0.0",
   "packages":[{"registryType":"npm","identifier":"@stranger/sqlite-mcp","version":"1.0.0","transport":{"type":"stdio"}}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"status":"active"}}},
 {"server":{"name":"com.example/sqlite","description":"SQLite by a trusted publisher","version":"2.1.0",
   "repository":{"url":"https://github.com/example/sqlite"},
   "packages":[{"registryType":"pypi","identifier":"example-sqlite","version":"2.1.0","transport":{"type":"stdio"}}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"status":"active"}}},
 {"server":{"name":"com.example/cloud-sqlite","description":"hosted","version":"1.0.0",
   "remotes":[{"type":"streamable-http","url":"https://sqlite.example.com/mcp"}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"status":"active"}}},
 {"server":{"name":"com.example/sqlite-keyed","description":"needs a key","version":"1.0.0",
   "packages":[{"registryType":"npm","identifier":"@example/keyed","version":"1.0.0","transport":{"type":"stdio"},
     "environmentVariables":[{"name":"API_KEY","isSecret":true,"isRequired":true}]}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"status":"active"}}},
 {"server":{"name":"com.example/sqlite-old","description":"old","version":"0.9.0",
   "packages":[{"registryType":"npm","identifier":"@example/old","version":"latest","transport":{"type":"stdio"}}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"status":"deprecated"}}}
]}`

// The official registry is read, and only what passes every check is used:
// a stranger's server, a server elsewhere, one needing a secret and an
// unpinned deprecated one are refused, each with its reason.
func TestRegistryServersAreReviewedBeforeAnythingIsDownloaded(t *testing.T) {
	var asked string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.String()
		w.Write([]byte(registryPage))
	}))
	defer srv.Close()

	reg := OfficialRegistry{URL: srv.URL, Trusted: []string{"com.example"}}

	found, err := reg.Find(context.Background(), "sqlite")
	if err != nil || len(found) != 5 {
		t.Fatalf("%d found, %v", len(found), err)
	}

	if asked != "/v0.1/servers?limit=30&search=sqlite&version=latest" {
		t.Errorf("the registry was asked %q", asked)
	}

	want := map[string]string{
		"sqlite":      "not on the trusted list",
		"sqlite2":     "",
		"cloudsqlite": "leave this machine",
		"sqlitekeyed": "API_KEY (a secret)",
		"sqliteold":   "not pinned",
	}

	for i, c := range found {
		id := c.ID
		if i == 1 {
			id = "sqlite2"
		}

		r := Inspect(c, true)
		reason := want[id]

		switch {
		case reason == "" && !r.OK:
			t.Errorf("%s refused: %s", c.Name(), r.Summary())
		case reason != "" && (r.OK || !strings.Contains(r.Summary(), reason)):
			t.Errorf("%s: %s, want a refusal saying %q", c.Name(), r.Summary(), reason)
		}
	}

	good := found[1]
	if strings.Join(good.Command, " ") != "uvx example-sqlite==2.1.0" || good.Recipe != "uv" || !good.Network {
		t.Errorf("the trusted one would run as %+v", good.Server)
	}

	// A registry program may reach out itself: not while privacy keeps
	// everything here.
	if r := Inspect(good, false); r.OK || !strings.Contains(r.Summary(), "privacy") {
		t.Errorf("offline: %s", r.Summary())
	}
}

func (c Candidate) Name() string { return c.Source }

type fakeSource struct{ list []Candidate }

func (fakeSource) Name() string { return "a fake registry" }

func (f fakeSource) Find(context.Context, string) ([]Candidate, error) { return f.list, nil }

/*
 * Acquired for one specialist, with the least it needs: the stranger's is
 * passed over, what it runs on is installed first, only the specialist named
 * may use it, the tool that says it destroys is not offered, and goal mode
 * hears of it so it can be recorded and later removed.
 */
func TestAnIntegrationIsAcquiredWithTheLeastItNeeds(t *testing.T) {
	g, _ := gateway(t)

	s, err := fakeCommand("modern")
	if err != nil {
		t.Fatal(err)
	}

	s.ID, s.Title, s.Secrets, s.Recipe, s.Version, s.Source = "notes", "Notes", nil, "node", "1.2.3", "fake registry, notes"

	stranger := Candidate{Server: s, From: "a fake registry", Publisher: "io.github.stranger", Status: "active",
		Package: "npm", Pinned: true}
	stranger.ID = "strangernotes"
	trusted := Candidate{Server: s, From: "a fake registry", Publisher: "com.example", Status: "active",
		Package: "npm", Pinned: true, trusted: true}

	installed := false
	g.Ready = func(string) bool { return installed }

	var provided, recorded []string

	provide := func(recipe, who string) error {
		provided = append(provided, recipe+" for "+who)
		installed = true

		return nil
	}

	g.Acquired = func(s Server, who string) { recorded = append(recorded, s.ID+" for "+who) }

	got, err := g.Acquire(context.Background(), []Source{fakeSource{[]Candidate{stranger, trusted}}},
		"notes", "game_developer", true, provide)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := g.Book.Get("strangernotes"); ok {
		t.Error("the stranger's server was added")
	}

	if strings.Join(provided, ",") != "node for game_developer" || strings.Join(recorded, ",") != "notes for game_developer" {
		t.Errorf("provided %v, recorded %v", provided, recorded)
	}

	added, _ := g.Book.Get("notes")
	if !added.Approved || strings.Join(added.Agents, ",") != "game_developer" || len(added.Secrets) != 0 {
		t.Errorf("granted as %+v", added)
	}

	if strings.Join(got.Hidden, ",") != "wipe-all" || has(g, ToolName("notes", "wipe-all")) {
		t.Errorf("the destroying tool is offered: hidden %v", got.Hidden)
	}

	tool, _ := g.Registry.Get(ToolName("notes", "echo"))
	echo, ok := tool.(*Wrapped)
	if !ok || echo.GrantedTo("assistant") || !echo.GrantedTo("game_developer") {
		t.Error("echo is not granted to the specialist alone")
	}

	var used []string
	g.Used = func(id string) { used = append(used, id) }

	if _, err := echo.Execute(context.Background(), []byte(`{"text":"hi"}`)); err != nil || strings.Join(used, ",") != "notes" {
		t.Errorf("a call was not counted as use: %v %v", used, err)
	}

	// And removed again when it is no longer used, with nothing left over.
	if err := g.Discard("notes"); err != nil {
		t.Fatal(err)
	}

	if _, ok := g.Book.Get("notes"); ok || has(g, ToolName("notes", "echo")) {
		t.Error("the integration is still here after being removed")
	}
}

// When nothing passes, nothing is added and the reasons say why.
func TestNothingIsAddedWhenNothingPasses(t *testing.T) {
	g, _ := gateway(t)

	c := Candidate{Server: Server{ID: "stranger", Title: "Stranger", Transport: "stdio", Command: []string{"npx", "-y", "x@1.0.0"}},
		From: "a fake registry", Publisher: "io.github.stranger", Status: "active", Package: "npm", Pinned: true}

	_, err := g.Acquire(context.Background(), []Source{fakeSource{[]Candidate{c}}}, "notes", "writer", true, nil)
	if !errors.Is(err, ErrNoneFit) || !strings.Contains(err.Error(), "not on the trusted list") {
		t.Errorf("err = %v", err)
	}

	if len(g.Book.Servers()) != 0 {
		t.Error("something was added")
	}
}

func has(g *Gateway, name string) bool {
	_, ok := g.Registry.Get(name)

	return ok
}

// Outside goal mode adding one is asked; in goal mode it is not one of the
// four questions, so it goes ahead on its own.
func TestAcquiringIsAskedOnlyOutsideGoalMode(t *testing.T) {
	args := json.RawMessage(`{"kind":"sqlite","for":"game_developer"}`)

	if tools.ConsentFor(AcquireIntegration{}, args) == "" {
		t.Error("outside goal mode, adding an integration was not asked")
	}

	if why := tools.GoalModeAsks(AcquireIntegration{}, args, nil); why != "" {
		t.Errorf("goal mode asks: %s", why)
	}
}
