package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-brain/internal/brain/protect"
)

/*
 * The route this closes: read a credentials file, then fetch a URL with the
 * contents attached. Both steps are Safe, so without this nobody sees it
 * happen.
 *
 * It is closed by asking rather than by refusing. Nothing here is out of
 * reach — every one of these files can be read, and the owner is the one who
 * says so, which is precisely the step the attack cannot survive.
 */
func TestSensitivePathsAreAskedAbout(t *testing.T) {
	ask := []string{
		"/home/petar/.env",
		"/home/petar/project/.env.local",
		"/home/petar/project/.env.production",
		"/home/petar/project/.env.anything-at-all",
		"/home/petar/.ssh/id_rsa",
		"/home/petar/.ssh/id_ed25519",
		"/home/petar/.gnupg/secring.gpg",
		"/home/petar/.aws/credentials",
		"/home/petar/.kube/config",
		"/home/petar/.docker/config.json",
		"/home/petar/.config/gcloud/token.json",
		"/home/petar/.password-store/x.gpg",
		"/etc/shadow",
		"/home/petar/certs/server.pem",
		"/home/petar/certs/private.key",
		"/home/petar/certs/bundle.p12",
		"/home/petar/.npmrc",
		"/home/petar/.netrc",
		"/home/petar/.git-credentials",
		"/home/petar/.pgpass",
		"/home/petar/.my.cnf",
		"/home/petar/composer/auth.json",
		"/home/petar/.mozilla/firefox/profile/key4.db",

		// Windows-style separators must not slip past the directory checks.
		`C:\Users\petar\.ssh\id_rsa`,
	}

	for _, p := range ask {
		if !IsSensitive(p) {
			t.Errorf("would have read a credentials file without asking: %s", p)
		}

		// And nothing reaches a tool without having been asked about, for the
		// case where a caller does not go through the agent at all.
		if err := GuardSensitive(p); err == nil {
			t.Errorf("a tool would have opened %s with no question anywhere", p)
		}
	}
}

// A guard that refuses ordinary files makes the brain useless. Both halves have
// to hold at once.
func TestOrdinaryFilesAreAllowed(t *testing.T) {
	allow := []string{
		"/home/petar/Projects/app/README.md",
		"/home/petar/Projects/app/composer.json",
		"/home/petar/Projects/app/src/main.go",
		"/home/petar/Documents/notes.txt",
		"/home/petar/Projects/app/config/database.php",
		"/home/petar/Projects/app/.gitignore",
		"/home/petar/Projects/app/environment.md",
		"/home/petar/Projects/keyboard-shortcuts.md",
		"/home/petar/Projects/app/public/style.css",
	}

	for _, p := range allow {
		if IsSensitive(p) {
			t.Errorf("guard blocked an ordinary file: %s", p)
		}
	}
}

/*
 * The question says enough to be answerable.
 *
 * Three things, because with fewer there is nothing to decide on: what is
 * being asked for, which file, and why that file is one it stops at. A prompt
 * reading only "allow access?" teaches people to say yes to everything.
 */
func TestTheQuestionSaysWhatItIsAsking(t *testing.T) {
	rule, ask := protect.Ask("/home/petar/.ssh/id_rsa")
	if !ask {
		t.Fatal("a private key did not raise a question")
	}

	said := protect.Explain("Read a file", "/home/petar/.ssh/id_rsa", rule)

	for _, want := range []string{"Read a file", "id_rsa", "ssh keys", "log into"} {
		if !strings.Contains(said, want) {
			t.Errorf("the question does not mention %q: %s", want, said)
		}
	}

	// And a tool reached without the question having been asked says so, in
	// terms that point at the way to do it properly.
	err := GuardSensitive("/home/petar/.ssh/id_rsa")
	if err == nil {
		t.Fatal("a tool opened a private key with no question anywhere")
	}

	if !strings.Contains(err.Error(), "id_rsa") {
		t.Errorf("the refusal does not say which file: %v", err)
	}
}

// Reading and listing observe; writing and running change things. The whole
// approval system rests on this classification being right.
func TestRiskClassification(t *testing.T) {
	cases := map[Tool]Risk{
		ReadFile{}:      Safe,
		ListDirectory{}: Safe,
		WriteFile{}:     Mutating,
		RunCommand{}:    Mutating,
	}

	for tool, want := range cases {
		if got := tool.Risk(); got != want {
			t.Errorf("%s is classed %q, want %q", tool.Name(), got, want)
		}
	}
}

func TestReadFileRefusesSensitiveAndDirectories(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, ".env")
	os.WriteFile(secret, []byte("API_KEY=hunter2"), 0o600)

	out, err := ReadFile{}.Execute(context.Background(),
		json.RawMessage(`{"path":`+quote(secret)+`}`))

	if err == nil {
		t.Fatalf("read a .env file and returned: %q", out)
	}

	if strings.Contains(out, "hunter2") {
		t.Fatal("the secret leaked into the output")
	}

	if _, err := (ReadFile{}).Execute(context.Background(),
		json.RawMessage(`{"path":`+quote(dir)+`}`)); err == nil {
		t.Error("reading a directory should point at list_directory")
	}
}

func TestReadFileReportsTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	os.WriteFile(path, make([]byte, MaxReadBytes+5000), 0o644)

	out, err := ReadFile{}.Execute(context.Background(),
		json.RawMessage(`{"path":`+quote(path)+`}`))
	if err != nil {
		t.Fatal(err)
	}

	// Silent truncation would have the model reason about a file it only
	// partly saw, with no way to know.
	if !strings.Contains(out, "truncated") {
		t.Error("a truncated read did not say so")
	}
}

// The person approving needs to know whether they are creating or destroying.
func TestWriteSummaryDistinguishesOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "there.txt")
	os.WriteFile(existing, []byte("original"), 0o644)

	fresh := filepath.Join(dir, "new.txt")

	if s := (WriteFile{}).Summarize(json.RawMessage(`{"path":` + quote(existing) + `,"content":"x"}`)); !strings.Contains(s, "OVERWRITE") {
		t.Errorf("overwriting an existing file was summarised as %q", s)
	}

	if s := (WriteFile{}).Summarize(json.RawMessage(`{"path":` + quote(fresh) + `,"content":"x"}`)); !strings.Contains(s, "Create") {
		t.Errorf("creating a new file was summarised as %q", s)
	}
}

func TestWriteFileRefusesSensitivePaths(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".env")

	if _, err := (WriteFile{}).Execute(context.Background(),
		json.RawMessage(`{"path":`+quote(target)+`,"content":"x"}`)); err == nil {
		t.Error("wrote to a credentials path")
	}
}

// There is no shell, so a semicolon is an argument rather than a second
// command. This is what makes the approval summary trustworthy.
func TestRunCommandHasNoShell(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")

	out, err := RunCommand{}.Execute(context.Background(), json.RawMessage(
		`{"argv":["echo","hello; touch `+marker+`"]}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the shell interpreted a semicolon and ran a second command")
	}

	if !strings.Contains(out, "hello; touch") {
		t.Errorf("the argument was not passed through literally: %q", out)
	}
}

// A failing command is usually the answer, not an error to raise.
func TestRunCommandReportsExitCode(t *testing.T) {
	out, err := RunCommand{}.Execute(context.Background(),
		json.RawMessage(`{"argv":["false"]}`))
	if err != nil {
		t.Fatalf("a non-zero exit should be reported, not raised: %v", err)
	}

	if !strings.Contains(out, "Exited with code 1") {
		t.Errorf("exit code not reported: %q", out)
	}
}

func TestRunCommandSummaryShowsExactCommand(t *testing.T) {
	s := RunCommand{}.Summarize(json.RawMessage(
		`{"argv":["git","commit","-m","a message"],"dir":"/tmp/x"}`))

	for _, want := range []string{"git", "commit", `"a message"`, "/tmp/x"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary %q is missing %q", s, want)
		}
	}
}

func TestRegistryIsStableAndComplete(t *testing.T) {
	r := NewRegistry(WriteFile{}, ReadFile{}, RunCommand{}, ListDirectory{})

	all := r.All()
	if len(all) != 4 {
		t.Fatalf("registry holds %d tools", len(all))
	}

	// Stable order, so the model sees the same list each time and its
	// behaviour does not drift with map iteration.
	want := []string{"list_directory", "read_file", "run_command", "write_file"}

	for i, n := range want {
		if all[i].Name() != n {
			t.Errorf("position %d is %q, want %q", i, all[i].Name(), n)
		}
	}

	if _, ok := r.Get("read_file"); !ok {
		t.Error("read_file not found by name")
	}

	if _, ok := r.Get("nonexistent"); ok {
		t.Error("registry invented a tool")
	}
}

func TestEveryToolDeclaresValidSchema(t *testing.T) {
	for _, tool := range NewRegistry(ReadFile{}, ListDirectory{}, WriteFile{}, RunCommand{}).All() {
		var schema map[string]any

		if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
			t.Errorf("%s has invalid JSON Schema: %v", tool.Name(), err)

			continue
		}

		if schema["type"] != "object" {
			t.Errorf("%s schema is not an object", tool.Name())
		}

		if tool.Description() == "" {
			t.Errorf("%s has no description", tool.Name())
		}
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}

// Approving "set lock.front to off" should not be how somebody discovers they
// unlocked their front door.
func TestUnlockingIsSpelledOutInTheApproval(t *testing.T) {
	var s SetDevice

	unlock := s.Summarize(json.RawMessage(`{"id":"lock.front","state":"on"}`))

	if !strings.Contains(unlock, "UNLOCK") || !strings.Contains(unlock, "physically unlocks") {
		t.Errorf("unlocking summarised as %q", unlock)
	}

	if lock := s.Summarize(json.RawMessage(`{"id":"lock.front","state":"off"}`)); !strings.Contains(lock, "LOCK") {
		t.Errorf("locking summarised as %q", lock)
	}

	// An ordinary device still names itself and the change.
	light := s.Summarize(json.RawMessage(`{"id":"light.kitchen","state":"on"}`))

	for _, want := range []string{"light.kitchen", "on"} {
		if !strings.Contains(light, want) {
			t.Errorf("summary %q is missing %q", light, want)
		}
	}
}

// Reading the state of a house changes nothing; changing it is physical and
// cannot always be undone.
func TestSmartHomeRiskClassification(t *testing.T) {
	if (ListDevices{}).Risk() != Safe {
		t.Error("listing devices should not need approval")
	}

	if (SetDevice{}).Risk() != Mutating {
		t.Error("changing a device must need approval")
	}
}
