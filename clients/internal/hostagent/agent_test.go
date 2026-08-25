package hostagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This daemon can read any file on the machine and run commands on it. Its
// safety properties are therefore the ones most worth pinning: they are what
// stands between "the assistant can act" and "anything on this machine can act
// as the assistant".

const token = "test-token-value"

func request(t *testing.T, path, body, bearer string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	rec := httptest.NewRecorder()
	New(token).Handler().ServeHTTP(rec, req)

	var decoded map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)

	return rec, decoded
}

// Loopback is not an authorisation boundary. Any process on this machine can
// reach 127.0.0.1, including a web page running someone else's JavaScript.
func TestRejectsRequestsWithoutTheToken(t *testing.T) {
	for _, bearer := range []string{"", "wrong-token", "test-token-valu"} {
		rec, _ := request(t, "/read", `{"path":"/etc/hostname"}`, bearer)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q was accepted (status %d); anything on this machine could then read any file",
				bearer, rec.Code)
		}
	}
}

func TestReadsAFileWithTheToken(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")

	if err := os.WriteFile(file, []byte("hello from the host"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, body := request(t, "/read", `{"path":"`+file+`"}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("read failed: %d %v", rec.Code, body["error"])
	}

	if body["content"] != "hello from the host" {
		t.Errorf("wrong content: %v", body["content"])
	}
}

// The brain guards this too. Repeated here because a daemon with shell access
// should not depend on every future caller being careful, or being the brain.
func TestRefusesCredentialsEvenWithAValidToken(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{".env", "id_rsa", "server.pem", "credentials"} {
		path := filepath.Join(dir, name)

		if err := os.WriteFile(path, []byte("SECRET=value"), 0o600); err != nil {
			t.Fatal(err)
		}

		rec, _ := request(t, "/read", `{"path":"`+path+`"}`, token)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s was readable (status %d)", name, rec.Code)
		}
	}
}

func TestRefusesCredentialDirectories(t *testing.T) {
	rec, _ := request(t, "/read", `{"path":"/home/someone/.ssh/known_hosts"}`, token)

	if rec.Code != http.StatusForbidden {
		t.Errorf("a path inside .ssh was not refused (status %d)", rec.Code)
	}
}

func TestListsADirectory(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), nil, 0o644)
	_ = os.Mkdir(filepath.Join(dir, "a"), 0o755)

	rec, body := request(t, "/list", `{"path":"`+dir+`"}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("list failed: %d", rec.Code)
	}

	entries, _ := body["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", entries)
	}

	// Directories are marked, so the model can tell them apart without a
	// second call.
	if entries[0] != "a/" {
		t.Errorf("directories should be marked with a slash; got %v", entries[0])
	}
}

func TestRunsACommandAndReportsItsExitCode(t *testing.T) {
	rec, body := request(t, "/exec", `{"argv":["echo","from the host"]}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("exec failed: %d", rec.Code)
	}

	if !strings.Contains(body["output"].(string), "from the host") {
		t.Errorf("unexpected output: %v", body["output"])
	}

	if body["exit_code"].(float64) != 0 {
		t.Errorf("expected exit 0, got %v", body["exit_code"])
	}
}

// A failing command is information, not an error. The model needs to read what
// went wrong to correct itself.
func TestAFailingCommandReportsRatherThanErroring(t *testing.T) {
	rec, body := request(t, "/exec", `{"argv":["false"]}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("a non-zero exit should still be a successful report; got %d", rec.Code)
	}

	if body["exit_code"].(float64) == 0 {
		t.Error("expected a non-zero exit code to be reported")
	}
}

// argv, never a shell string: there is no shell to inject into, so a semicolon
// is an argument rather than a second command.
func TestShellMetacharactersAreNotInterpreted(t *testing.T) {
	rec, body := request(t, "/exec", `{"argv":["echo","hello; touch /tmp/pn-brain-should-not-exist"]}`, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("exec failed: %d", rec.Code)
	}

	if _, err := os.Stat("/tmp/pn-brain-should-not-exist"); err == nil {
		os.Remove("/tmp/pn-brain-should-not-exist")
		t.Fatal("the shell interpreted a semicolon; commands must not go through a shell")
	}

	if !strings.Contains(body["output"].(string), "touch") {
		t.Error("the metacharacters should have been passed through as literal text")
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	New(token).Handler().ServeHTTP(rec, req)

	// Health carries nothing sensitive and is what the brain uses to notice the
	// agent is running at all.
	if rec.Code != http.StatusOK {
		t.Errorf("health should be reachable without a token; got %d", rec.Code)
	}
}
