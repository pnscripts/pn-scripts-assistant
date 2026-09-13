package setup

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is the first thing a new user sees, on a machine where nothing works
// yet, and it is where they decide whether to trust the software at all. It
// lives in its own package rather than beside the window code specifically so
// it can be exercised without a graphical toolkit installed.

func startServer(t *testing.T) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	if err := os.WriteFile(envPath, []byte("APP_NAME=\"PN Scripts Assistant\"\nANTHROPIC_API_KEY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server, err := New(envPath)
	if err != nil {
		t.Fatal(err)
	}

	go server.Serve(func() {})

	// The listener is already bound by New, so the port is live immediately;
	// this only gives the goroutine a moment to attach the handlers.
	time.Sleep(50 * time.Millisecond)

	return server, envPath
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	return out
}

func postJSON(t *testing.T, url, body string) map[string]any {
	t.Helper()

	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	return out
}

func TestServesASetupPage(t *testing.T) {
	server, _ := startServer(t)

	resp, err := http.Get(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup page returned %d", resp.StatusCode)
	}
}

func TestReportsWhatTheMachineNeeds(t *testing.T) {
	server, _ := startServer(t)
	state := getJSON(t, server.URL()+"/state")

	for _, key := range []string{"requirements", "hardware", "recommended_model", "blocking"} {
		if _, ok := state[key]; !ok {
			t.Errorf("state is missing %q, which the page renders", key)
		}
	}

	hardware, _ := state["hardware"].(map[string]any)
	if hardware["ram_gb"] == nil || hardware["cores"] == nil {
		t.Error("hardware must be reported; the model recommendation depends on it")
	}
}

// The starter is the point of the page before anything is installed.
func TestAnswersQuestionsBeforeAnythingIsInstalled(t *testing.T) {
	server, _ := startServer(t)

	reply := postJSON(t, server.URL()+"/ask", `{"question":"is my data private?"}`)

	if reply["matched"] != true {
		t.Fatal("a question it has an answer for went unmatched")
	}

	if answer, _ := reply["answer"].(string); !strings.Contains(answer, "nothing leaves your computer") {
		t.Errorf("wrong answer returned: %.70s", answer)
	}
}

func TestSuggestionsAreOffered(t *testing.T) {
	server, _ := startServer(t)

	out := getJSON(t, server.URL()+"/suggestions")
	if list, _ := out["suggestions"].([]any); len(list) == 0 {
		t.Error("no suggestions offered; an empty box is one nobody types into")
	}
}

func TestRejectsSomethingThatIsNotAKey(t *testing.T) {
	server, _ := startServer(t)

	out := postJSON(t, server.URL()+"/api-key", `{"key":"hunter2"}`)

	if out["ok"] == true {
		t.Error("accepted a value that is obviously not an API key")
	}
}

// The key must reach the file the app actually reads, and nowhere else.
func TestStoresAKeyInTheEnvFileWithRestrictivePermissions(t *testing.T) {
	server, envPath := startServer(t)

	key := "sk-ant-" + strings.Repeat("a", 20)

	if out := postJSON(t, server.URL()+"/api-key", `{"key":"`+key+`"}`); out["ok"] != true {
		t.Fatalf("a well-formed key was rejected: %v", out["error"])
	}

	contents, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(contents), "ANTHROPIC_API_KEY="+key) {
		t.Error("the key did not reach .env")
	}

	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the file now holds a credential; permissions are %o, want 600", perm)
	}
}

// The setup log is rendered in the window, so a key must never reach it.
func TestTheKeyIsNeverEchoedIntoTheVisibleLog(t *testing.T) {
	server, _ := startServer(t)

	key := "sk-ant-" + strings.Repeat("b", 20)
	postJSON(t, server.URL()+"/api-key", `{"key":"`+key+`"}`)

	state := getJSON(t, server.URL()+"/state")

	if log, _ := state["log"].(string); strings.Contains(log, key) {
		t.Error("the API key appeared in the log shown on screen")
	}

	if state["has_api_key"] != true {
		t.Error("the page should know a key is present without revealing it")
	}
}

/*
 * The brain is offered a home on each drive, in the place that suits it.
 *
 * Judged on the folder it would use rather than on the mount point, which is
 * what the first attempt got wrong: an ordinary user cannot write to /, so
 * testing the mount dropped "this computer" from the list entirely and left a
 * single option — and a single option is not a choice. The brain does not live
 * at /; it lives in a home directory on that filesystem.
 */
func TestEachDriveIsOfferedTheRightFolder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	// The system disk: inside the home directory, where a user can write and
	// where somebody would expect to find their own things.
	onRoot := brainFolderOn("/")

	if !strings.HasPrefix(onRoot, home) {
		t.Errorf("the system disk offers %q, which is not inside the home directory",
			onRoot)
	}

	/*
	 * Any other drive: a named folder at the top, which is what somebody
	 * plugging that drive into another machine would go looking for.
	 */
	onOther := brainFolderOn("/media/petar/backup")

	if onOther != filepath.Join("/media/petar/backup", DataFolder) {
		t.Errorf("an external drive offers %q, want the named folder at its top",
			onOther)
	}

	// And a home directory that is itself on external media belongs to that
	// drive rather than to the system disk.
	if onSameFilesystem("/media/petar/disk/home/petar", "/") {
		t.Error("a home directory on external media was claimed by the system disk")
	}

	if !onSameFilesystem("/media/petar/disk/things", "/media/petar/disk") {
		t.Error("a path under a mount point was not recognised as being on it")
	}
}

/*
 * Somewhere that cannot be written to is not offered.
 *
 * Tried rather than inspected: a drive can be mounted, report every permission
 * correctly and still refuse — a read-only mount, a full disk, a filesystem
 * that will not take the ownership this needs. Finding that out now costs a
 * file; finding it out after setup costs somebody their first impression.
 */
func TestAFolderThatCannotBeWrittenIsNotOffered(t *testing.T) {
	if canCreate("/proc/pn-scripts-assistant-should-never-work") {
		t.Error("a folder that cannot exist was reported as usable")
	}

	usable := filepath.Join(t.TempDir(), "somewhere", "new")

	if !canCreate(usable) {
		t.Errorf("%s is writable but was not offered", usable)
	}

	// And the probe leaves nothing behind.
	if _, err := os.Stat(usable); err == nil {
		t.Error("checking a folder created it and left it there")
	}
}
