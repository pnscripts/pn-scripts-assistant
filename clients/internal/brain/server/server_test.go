package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
)

func newServer(t *testing.T) (*httptest.Server, *store.DB, *brain.Brain) {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Owner = "Petar"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger)

	ts := httptest.NewServer(New(b, logger).Handler())

	t.Cleanup(func() {
		ts.Close()
		db.Close()
	})

	return ts, db, b
}

func TestListenRefusesNonLoopback(t *testing.T) {
	// A brain bound to a public interface works perfectly for its owner while
	// serving everything it knows to the network around it.
	for _, addr := range []string{"0.0.0.0:8790", "192.168.1.10:8790", ":8790"} {
		ln, err := Listen(addr)

		if err == nil {
			ln.Close()
			t.Errorf("Listen(%q) was allowed", addr)
		}
	}
}

func TestListenAllowsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		ln, err := Listen(addr)
		if err != nil {
			t.Errorf("Listen(%q) was refused: %v", addr, err)

			continue
		}

		ln.Close()
	}
}

// The full gate, over HTTP: a recorded action does nothing until approved, then
// does exactly what its summary said.
func TestApprovalGateOverHTTP(t *testing.T) {
	ts, db, _ := newServer(t)

	target := filepath.Join(t.TempDir(), "approved.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "written after approval"})

	id, err := db.RecordInvocation(0, "write_file", string(args), "Create "+target, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	// It must appear as waiting, or the owner can never act on it.
	var pending []store.Invocation
	getJSON(t, ts.URL+"/api/approvals", &pending)

	if len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("pending is %+v", pending)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file existed before approval")
	}

	// Approve.
	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if e, ok := result["error"]; ok {
		t.Fatalf("approval reported an error: %v", e)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("approved action did not run: %v", err)
	}

	if string(content) != "written after approval" {
		t.Errorf("file holds %q", content)
	}

	// It must leave the queue, or it could be run twice.
	getJSON(t, ts.URL+"/api/approvals", &pending)

	if len(pending) != 0 {
		t.Errorf("approved action is still pending: %+v", pending)
	}
}

func TestDenialDoesNotRunTheAction(t *testing.T) {
	ts, db, _ := newServer(t)

	target := filepath.Join(t.TempDir(), "denied.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "should never exist"})

	id, _ := db.RecordInvocation(0, "write_file", string(args), "Create "+target, "mutating")

	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/deny", &result)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a denied action wrote its file")
	}

	// And it cannot be revived by asking again.
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a denied action was re-approved and ran")
	}
}

// Arguments imported from the old system are not valid JSON. Approving one must
// fail cleanly rather than panic or write something unintended.
func TestCorruptStoredArgumentsFailSafely(t *testing.T) {
	ts, db, _ := newServer(t)

	id, _ := db.RecordInvocation(0, "write_file",
		"map[content:The word is banana path:output.txt]",
		"Create output.txt", "mutating")

	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if _, ok := result["error"]; !ok {
		t.Fatal("corrupt arguments were accepted without error")
	}

	if _, err := os.Stat("output.txt"); err == nil {
		os.Remove("output.txt")
		t.Fatal("a file was written from unreadable arguments")
	}
}

func TestStatusCarriesEverythingTheInterfaceReads(t *testing.T) {
	ts, _, _ := newServer(t)

	var s map[string]any
	getJSON(t, ts.URL+"/api/status", &s)

	for _, key := range []string{"name", "provider", "model", "privacy", "storage", "capabilities", "memory"} {
		if _, ok := s[key]; !ok {
			t.Errorf("status is missing %q", key)
		}
	}

	memory, _ := s["memory"].(map[string]any)

	for _, key := range []string{"facts", "pending_lessons", "conversations"} {
		if _, ok := memory[key]; !ok {
			t.Errorf("status.memory is missing %q", key)
		}
	}

	privacy, _ := s["privacy"].(map[string]any)
	if privacy["mode"] != "private" {
		t.Errorf("default privacy is %v, want private", privacy["mode"])
	}
}

func TestInterfaceIsServedFromTheBinary(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, path := range []string{"/", "/css/console.css", "/js/console.js", "/js/brainmap.js"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s returned %d", path, resp.StatusCode)
		}

		if len(body) == 0 {
			t.Errorf("%s was empty", path)
		}
	}

	// The configured name must reach the page, since that is the only reason
	// it is a template rather than a static file.
	resp, _ := http.Get(ts.URL + "/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "PN Brain") {
		t.Error("the brain's name did not reach the page")
	}
}

// A tool the model wants but privacy or configuration forbids must not appear
// as usable. Here: with no key, Anthropic is not registered at all.
func TestUnconfiguredProviderIsNotOffered(t *testing.T) {
	ts, _, _ := newServer(t)

	var s struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}

	getJSON(t, ts.URL+"/api/status", &s)

	for _, p := range s.Providers {
		if p.Name == "anthropic" {
			t.Error("anthropic was offered without an API key")
		}
	}
}

func TestChatRejectsEmptyMessage(t *testing.T) {
	ts, _, _ := newServer(t)

	resp, err := http.Post(ts.URL+"/api/chat", "application/json",
		strings.NewReader(`{"message":"   "}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty message returned %d", resp.StatusCode)
	}
}

// The agent registry must expose exactly the tools that were built, so a tool
// cannot be silently dropped or added.
func TestRegisteredTools(t *testing.T) {
	_, _, b := newServer(t)

	var names []string
	for _, tool := range b.Agent.Registry.All() {
		names = append(names, tool.Name())
	}

	want := []string{"list_directory", "read_file", "run_command", "write_file"}

	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("tools are %v, want %v", names, want)
	}

	// And the dangerous ones must still be gated.
	for _, n := range []string{"write_file", "run_command"} {
		tool, _ := b.Agent.Registry.Get(n)

		if tool.Risk() != tools.Mutating {
			t.Errorf("%s is not gated", n)
		}
	}
}

func getJSON(t *testing.T, url string, into any) {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}

func postJSON(t *testing.T, url string, into any) {
	t.Helper()

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// A tool the model can see is a tool it will try. In private mode the web tool
// is not registered at all, rather than registered and refused — an assistant
// that keeps proposing something it may never do is worse than one that simply
// cannot.
func TestWebToolExistsOnlyWhenPrivacyAllowsIt(t *testing.T) {
	cases := map[string]bool{"private": false, "research": true, "open": true}

	for privacy, wantWeb := range cases {
		t.Run(privacy, func(t *testing.T) {
			root := t.TempDir()

			db, err := store.Open(filepath.Join(root, "brain.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			cfg := config.Default()
			cfg.Privacy = privacy

			b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"),
				slog.New(slog.NewTextHandler(io.Discard, nil)))

			_, hasWeb := b.Agent.Registry.Get("fetch_url")

			if hasWeb != wantWeb {
				t.Errorf("privacy %q: fetch_url present = %v, want %v", privacy, hasWeb, wantWeb)
			}
		})
	}
}
