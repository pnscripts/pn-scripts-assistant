package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * An edit that matches more than once must not guess.
 *
 * Replacing the first of several matches is how an edit silently changes the
 * wrong line: the tool reports success, the file is wrong, and nobody finds out
 * until much later. Refusing and saying how many were found gives the model
 * something it can act on.
 */
func TestAnEditMustMatchExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")

	body := "func one() {\n\treturn nil\n}\n\nfunc two() {\n\treturn nil\n}\n"

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	call := func(old, new string) (string, error) {
		args, _ := json.Marshal(editArgs{Path: path, Old: old, New: new})

		return (EditFile{}).Execute(context.Background(), args)
	}

	if _, err := call("\treturn nil", "\treturn errors.New(\"no\")"); err == nil {
		t.Error("an edit matching twice was applied anyway")
	} else if !strings.Contains(err.Error(), "2 times") {
		t.Errorf("the error does not say how many matched: %v", err)
	}

	// Unchanged, because a refused edit must not half-apply.
	if after, _ := os.ReadFile(path); string(after) != body {
		t.Error("the file changed despite the edit being refused")
	}

	// With enough context it is unambiguous, and it applies.
	if _, err := call("func two() {\n\treturn nil", "func two() {\n\treturn errNo"); err != nil {
		t.Fatalf("an unambiguous edit was refused: %v", err)
	}

	after, _ := os.ReadFile(path)

	if !strings.Contains(string(after), "errNo") {
		t.Error("the edit did not apply")
	}

	if !strings.Contains(string(after), "func one() {\n\treturn nil") {
		t.Error("the edit changed the wrong function")
	}
}

// Text that is not there is a mistake worth reporting, not a no-op.
func TestAnEditThatMatchesNothingSaysSo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")

	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(editArgs{Path: path, Old: "goodbye", New: "hi"})

	if _, err := (EditFile{}).Execute(context.Background(), args); err == nil {
		t.Error("replacing text that is not in the file succeeded")
	}
}

// An edit keeps the file's permissions.
//
// Writing a script back as plain 0644 makes it stop being executable, and the
// failure appears somewhere else entirely, long afterwards.
func TestAnEditKeepsThePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.sh")

	if err := os.WriteFile(path, []byte("echo one\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(editArgs{Path: path, Old: "one", New: "two"})

	if _, err := (EditFile{}).Execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o755 {
		t.Errorf("the file is now %v; it was executable", info.Mode().Perm())
	}
}

// Searching reports where things are, and does not wander into the places that
// hold no answers and enormous numbers of files.
func TestSearchingFindsAndSkips(t *testing.T) {
	dir := t.TempDir()

	write := func(rel, body string) {
		at := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("a.go", "package main\n\nfunc wanted() {}\n")
	write("b.txt", "wanted here too\n")
	write("node_modules/c.go", "func wanted() {}\n")
	write(".git/d.go", "func wanted() {}\n")

	args, _ := json.Marshal(searchArgs{Text: "wanted", Dir: dir})

	out, err := (SearchFiles{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "a.go:3") {
		t.Errorf("did not find the match, or lost the line number: %s", out)
	}

	if !strings.Contains(out, "b.txt") {
		t.Errorf("searched only one kind of file: %s", out)
	}

	for _, skip := range []string{"node_modules", ".git"} {
		if strings.Contains(out, skip) {
			t.Errorf("searched %s, which holds no answers and endless files", skip)
		}
	}

	// And it can be narrowed.
	args, _ = json.Marshal(searchArgs{Text: "wanted", Dir: dir, Extension: ".go"})

	out, err = (SearchFiles{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out, "b.txt") {
		t.Errorf("the extension was ignored: %s", out)
	}
}

// Credentials stay out of it, including out of search results, which quote the
// line they matched.
func TestSearchingWillNotQuoteASecret(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("API_KEY=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(searchArgs{Text: "hunter2", Dir: dir})

	out, err := (SearchFiles{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// The search term is echoed back in "no match for ...", so look for the
	// file and the line itself rather than the word that was searched for.
	if strings.Contains(out, "API_KEY") || strings.Contains(out, ".env") {
		t.Errorf("a search quoted a credential back: %s", out)
	}
}

// A search that would fill the whole context stops and says so.
func TestSearchingStopsBeforeItFillsEverything(t *testing.T) {
	dir := t.TempDir()

	var lines strings.Builder

	for i := 0; i < MaxMatches*3; i++ {
		fmt.Fprintf(&lines, "wanted %d\n", i)
	}

	if err := os.WriteFile(filepath.Join(dir, "many.txt"),
		[]byte(lines.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(searchArgs{Text: "wanted", Dir: dir})

	out, err := (SearchFiles{}).Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "narrow the search") {
		t.Error("it returned everything it found without saying it had stopped")
	}

	if n := strings.Count(out, "many.txt"); n > MaxMatches {
		t.Errorf("returned %d matches, past the cap of %d", n, MaxMatches)
	}
}
