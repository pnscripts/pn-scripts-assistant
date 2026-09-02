package copies

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-brain/internal/brain/store"
)

// brain makes a working root: a marker, settings, and a database with
// something in it, which is what every test here copies from.
func brain(t *testing.T) (string, *store.DB) {
	t.Helper()

	root := t.TempDir()

	os.WriteFile(filepath.Join(root, ".brain-root.json"),
		[]byte(`{"id":"the-one","schema":1,"created":"2026-09-01T00:00:00Z"}`), 0o644)
	os.WriteFile(filepath.Join(root, "brain.conf"), []byte("BRAIN_NAME=Ariel\n"), 0o600)

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	for _, what := range []string{"one", "two", "three"} {
		if _, err := db.AddFact("test", what, nil); err != nil {
			t.Fatal(err)
		}
	}

	return root, db
}

/*
 * A copy holds everything needed to be a brain again.
 *
 * The database on its own is not enough. Somebody restoring from a backup and
 * being shown the setup wizard, because the name and the owner were left
 * behind, has not been given back what they lost.
 */
func TestACopyHoldsTheMemoryAndTheSettings(t *testing.T) {
	root, db := brain(t)
	dir := filepath.Join(t.TempDir(), "backup")

	made, err := Write(context.Background(), db, root, dir)
	if err != nil {
		t.Fatal(err)
	}

	if made.Facts != 3 {
		t.Errorf("the copy records %d facts; it was made from 3", made.Facts)
	}

	copied, err := store.Open(filepath.Join(dir, "brain.sqlite"))
	if err != nil {
		t.Fatalf("the copy will not open: %v", err)
	}
	defer copied.Close()

	if n, _ := copied.CountFacts(); n != 3 {
		t.Errorf("the copy holds %d facts, not 3", n)
	}

	settings, err := os.ReadFile(filepath.Join(dir, "brain.conf"))
	if err != nil || !strings.Contains(string(settings), "Ariel") {
		t.Errorf("the copy did not take the settings with it: %v", err)
	}
}

/*
 * And nothing goes looking for the brain and finds a copy.
 *
 * The whole design rests on this. A copy carrying .brain-root.json would be
 * started as the brain by any machine that had the backup drive attached and
 * its own drive missing — and once a day of conversation has gone into it,
 * there are two brains and no honest way back to one.
 */
func TestACopyIsNotMistakenForTheBrain(t *testing.T) {
	root, db := brain(t)
	dir := filepath.Join(t.TempDir(), "backup")

	if _, err := Write(context.Background(), db, root, dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".brain-root.json")); err == nil {
		t.Fatal("a copy carries the marker that makes a folder the brain itself")
	}

	if _, err := os.Stat(filepath.Join(dir, Marker)); err != nil {
		t.Errorf("a copy is not marked as one: %v", err)
	}

	// And Use is what turns one into a brain, on purpose.
	if err := Use(dir); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".brain-root.json"))
	if err != nil {
		t.Fatalf("after choosing it, it is still not a brain: %v", err)
	}

	var marker struct {
		ID string `json:"id"`
	}

	json.Unmarshal(raw, &marker)

	// The same brain, not a new one: its identity is what says the memories in
	// it belong to the brain somebody has been talking to for a year.
	if marker.ID != "the-one" {
		t.Errorf("the promoted copy calls itself %q rather than the brain it came from", marker.ID)
	}

	if _, err := os.Stat(filepath.Join(dir, Marker)); err == nil {
		t.Error("it is still marked as a copy as well, so the next thing to read it has to guess")
	}
}

// Somewhere that cannot be a copy is refused with a reason, rather than
// producing something that looks like a backup and is not one.
func TestNowhereThatWouldMakeAUselessCopy(t *testing.T) {
	root, db := brain(t)

	other := t.TempDir()
	os.WriteFile(filepath.Join(other, ".brain-root.json"), []byte(`{"id":"someone-elses"}`), 0o644)

	for _, c := range []struct {
		what string
		dir  string
	}{
		{"the brain's own folder", root},
		{"inside the brain's own folder", filepath.Join(root, "backup")},
		{"a folder holding the brain", filepath.Dir(root)},
		{"on top of another brain", other},
		{"a relative path", "backup"},
	} {
		if err := Keep(root, c.dir); err == nil {
			t.Errorf("keeping a copy in %s was allowed", c.what)
		}
	}

	// And the refusal is not advisory: writing directly is refused too.
	if _, err := Write(context.Background(), db, root, other); err == nil {
		t.Error("a copy was written over another brain")
	}

	if n, _ := os.ReadFile(filepath.Join(other, ".brain-root.json")); !strings.Contains(string(n), "someone-elses") {
		t.Error("the other brain's marker was overwritten")
	}
}

/*
 * A drive that is not plugged in is the ordinary case.
 *
 * It must stay on the list — the moment somebody wants to know where their
 * backups are is the moment one of them is missing — and it must not stop the
 * copies that can be made.
 */
func TestAnUnreachableCopyIsListedRatherThanForgotten(t *testing.T) {
	root, db := brain(t)

	here := filepath.Join(t.TempDir(), "attached")
	away := filepath.Join(t.TempDir(), "unplugged", "PN-BRAIN-COPY")

	if err := Keep(root, here); err != nil {
		t.Fatal(err)
	}

	if err := Keep(root, away); err != nil {
		t.Fatal(err)
	}

	made := WriteAll(context.Background(), db, root)

	if len(made) != 2 {
		t.Fatalf("expected both places to be reported, got %d", len(made))
	}

	byPath := map[string]Copy{}
	for _, c := range made {
		byPath[c.Path] = c
	}

	if got := byPath[here]; got.Facts != 3 || got.Trouble != "" {
		t.Errorf("the attached copy was not written: %+v", got)
	}

	if got := byPath[away]; got.Reachable || got.Trouble == "" {
		t.Errorf("the missing drive was not reported as missing: %+v", got)
	}

	// Still listed, with the reason, rather than dropped.
	status, err := Status(root)
	if err != nil || len(status) != 2 {
		t.Fatalf("the list forgot a copy: %v %+v", err, status)
	}
}

/*
 * A copy that fails halfway leaves the last good one alone.
 *
 * This is the only failure that would actually cost somebody their backup:
 * overwriting a complete copy from yesterday with half of one from today,
 * because a drive was pulled out during the write.
 */
func TestAFailedCopyLeavesTheLastGoodOneInPlace(t *testing.T) {
	root, db := brain(t)
	dir := filepath.Join(t.TempDir(), "backup")

	first, err := Write(context.Background(), db, root, dir)
	if err != nil {
		t.Fatal(err)
	}

	// The drive goes away mid-write: the snapshot cannot be taken.
	stopped, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Write(stopped, db, root, dir); err == nil {
		t.Fatal("a copy that could not be taken reported success")
	}

	copied, err := store.Open(filepath.Join(dir, "brain.sqlite"))
	if err != nil {
		t.Fatalf("the copy from before is unusable: %v", err)
	}
	defer copied.Close()

	if n, _ := copied.CountFacts(); n != first.Facts {
		t.Errorf("the good copy holds %d facts now; it held %d", n, first.Facts)
	}

	if _, err := os.Stat(filepath.Join(dir, "brain.sqlite.copying")); err == nil {
		t.Error("half a database was left behind next to the copy")
	}
}

// Removing a place from the list leaves the copy on the disk. Somebody who
// stopped copying to a drive has said nothing about destroying what is on it.
func TestStoppingDoesNotDeleteTheCopy(t *testing.T) {
	root, db := brain(t)
	dir := filepath.Join(t.TempDir(), "backup")

	Keep(root, dir)

	if _, err := Write(context.Background(), db, root, dir); err != nil {
		t.Fatal(err)
	}

	if err := Stop(root, dir); err != nil {
		t.Fatal(err)
	}

	if places, _ := Places(root); len(places) != 0 {
		t.Errorf("still on the list: %v", places)
	}

	if _, err := os.Stat(filepath.Join(dir, "brain.sqlite")); err != nil {
		t.Errorf("the copy itself was deleted: %v", err)
	}
}
