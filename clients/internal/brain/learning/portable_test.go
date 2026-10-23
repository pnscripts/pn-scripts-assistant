package learning

import (
	"path/filepath"
	"testing"
)

// The same file inside a brain is the same source wherever the brain is.
func TestAPathInsideTheBrainIsTheSameWhereverTheBrainIs(t *testing.T) {
	defer SetBrainRoot("")

	first, second := filepath.Join(t.TempDir(), "BRAIN"), filepath.Join(t.TempDir(), "stick", "BRAIN")

	SetBrainRoot(first)
	before := Document{Path: filepath.Join(first, "knowledge", "a.md")}.Source()

	SetBrainRoot(second)
	after := Document{Path: filepath.Join(second, "knowledge", "a.md")}.Source()

	if before != after || before != "document:@brain/knowledge/a.md" {
		t.Fatalf("%q before the move, %q after", before, after)
	}

	if got := Resolve("@brain/knowledge/a.md"); got != filepath.Join(second, "knowledge", "a.md") {
		t.Fatalf("resolved to %q", got)
	}

	if got := Portable("/elsewhere/a.md"); got != "/elsewhere/a.md" {
		t.Fatalf("a path outside the brain changed: %q", got)
	}

	if path, ok := WantsReading(Observation{Source: ReadingSource(filepath.Join(second, "knowledge", "a.md"))}); !ok || path != filepath.Join(second, "knowledge", "a.md") {
		t.Fatalf("a reading placeholder resolved to %q", path)
	}
}
