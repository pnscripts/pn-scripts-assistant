package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The whole round trip: write over a file through the tool, then put it back.
 *
 * Checked through the tools rather than through the store underneath, because
 * the part that was missing was not the copying — it was that nothing called
 * it. A store of previous versions that no writer hands anything to is an
 * empty folder with a good excuse.
 */
func TestWritingThenPuttingItBack(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(t.TempDir(), "notes.md")

	if err := os.WriteFile(work, []byte("what was there"), 0o644); err != nil {
		t.Fatal(err)
	}

	was := Root
	Root = root

	t.Cleanup(func() { Root = was })

	written, err := WriteFile{}.Execute(context.Background(),
		json.RawMessage(`{"path":`+quoted(work)+`,"content":"what it wrote instead"}`))
	if err != nil {
		t.Fatalf("writing: %v (%s)", err, written)
	}

	now, _ := os.ReadFile(work)

	if string(now) != "what it wrote instead" {
		t.Fatalf("the write did not happen: %q", now)
	}

	said, err := (PutBack{Root: root}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	back, _ := os.ReadFile(work)

	if string(back) != "what was there" {
		t.Errorf("after putting it back the file reads %q", back)
	}

	if !strings.Contains(said, "notes.md") {
		t.Errorf("it does not say what it put back: %s", said)
	}
}

// Undoing writes over what is there now, so it waits for a decision — a
// misheard "put it back" restoring an hour-old version over an hour of work
// would be a worse mistake than the one being undone.
func TestPuttingBackWaitsForAPerson(t *testing.T) {
	if (PutBack{}).Risk() != Mutating {
		t.Error("undoing a change runs without being approved")
	}

	if said := (PutBack{}).Summarize(json.RawMessage(`{"list":true}`)); !strings.Contains(said, "List") {
		t.Errorf("listing reads as %q", said)
	}
}

// Nothing to undo is an answer, not an error.
func TestNothingToPutBack(t *testing.T) {
	said, err := (PutBack{Root: t.TempDir()}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(said, "nothing to put back") {
		t.Errorf("it said %q", said)
	}
}

func quoted(s string) string {
	raw, _ := json.Marshal(s)

	return string(raw)
}
