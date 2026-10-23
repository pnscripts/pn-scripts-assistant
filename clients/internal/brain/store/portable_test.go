package store

import (
	"path/filepath"
	"testing"
)

func TestPathsInsideTheBrainBecomeRelative(t *testing.T) {
	root := t.TempDir()

	db, err := Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inside := filepath.Join(root, "knowledge", "guides", "a.md")
	outside := "/elsewhere/notes.md"

	db.AddLesson(0, "inside", "proposed", "high", "document:"+inside)
	db.AddLesson(0, "read", "proposed", "high", "reading:"+inside)
	db.AddLesson(0, "outside", "proposed", "high", "document:"+outside)

	n, err := db.PortableSources(root)
	if err != nil || n != 2 {
		t.Fatalf("changed %d rows (%v), want 2", n, err)
	}

	got := map[string]bool{}

	rows, _ := db.sql().Query(`SELECT source FROM lessons`)
	for rows.Next() {
		var s string
		rows.Scan(&s)
		got[s] = true
	}
	rows.Close()

	for _, want := range []string{"document:@brain/knowledge/guides/a.md", "reading:@brain/knowledge/guides/a.md", "document:" + outside} {
		if !got[want] {
			t.Errorf("no %q in %v", want, got)
		}
	}

	if again, _ := db.PortableSources(root); again != 0 {
		t.Errorf("the second run changed %d rows", again)
	}
}
