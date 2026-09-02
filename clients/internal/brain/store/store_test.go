package store

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *DB {
	t.Helper()

	db, err := Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	return db
}

func TestVectorRoundTrip(t *testing.T) {
	in := []float32{0.4101461, 0.11022698, -2.8475182, 0, 1e-9}

	out, err := DecodeVector(EncodeVector(in))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if len(out) != len(in) {
		t.Fatalf("got %d values, want %d", len(out), len(in))
	}

	for i := range in {
		if out[i] != in[i] {
			t.Errorf("value %d: got %v, want %v", i, out[i], in[i])
		}
	}
}

func TestDecodeRejectsRaggedBlob(t *testing.T) {
	if _, err := DecodeVector([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected an error for a blob that is not a multiple of 4 bytes")
	}
}

func TestCosineKnownValues(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 2, 3}, []float32{1, 2, 3}, 1},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"scale invariant", []float32{1, 2, 3}, []float32{10, 20, 30}, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Cosine(c.a, c.b)

			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// A vector from a different embedding model is not comparable to one from this
// model. Scoring such a pair would produce a number that looks like a
// relationship but means nothing, so the answer is zero.
func TestCosineRefusesMismatchedDimensions(t *testing.T) {
	if got := Cosine([]float32{1, 2, 3}, []float32{1, 2}); got != 0 {
		t.Errorf("got %v, want 0", got)
	}

	if got := Cosine(nil, nil); got != 0 {
		t.Errorf("empty vectors: got %v, want 0", got)
	}
}

func TestNormaliseProducesUnitLength(t *testing.T) {
	got := Normalise([]float32{3, 4})

	var sum float64
	for _, f := range got {
		sum += float64(f) * float64(f)
	}

	if math.Abs(math.Sqrt(sum)-1) > 1e-6 {
		t.Errorf("length %v, want 1", math.Sqrt(sum))
	}

	// A zero vector has no direction; normalising must not divide by zero.
	if z := Normalise([]float32{0, 0}); z[0] != 0 || z[1] != 0 {
		t.Errorf("zero vector became %v", z)
	}
}

func TestSearchRanksBySimilarity(t *testing.T) {
	db := open(t)

	// Three facts pointing in clearly different directions.
	near, _ := db.AddFact("project", "almost the query", []float32{1, 0.1, 0})
	exact, _ := db.AddFact("project", "the query itself", []float32{1, 0, 0})
	far, _ := db.AddFact("project", "unrelated", []float32{0, 0, 1})

	got, err := db.Search([]float32{1, 0, 0}, 10, -1)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}

	if got[0].ID != exact {
		t.Errorf("best match is %d, want %d", got[0].ID, exact)
	}

	if got[1].ID != near {
		t.Errorf("second match is %d, want %d", got[1].ID, near)
	}

	if got[2].ID != far {
		t.Errorf("worst match is %d, want %d", got[2].ID, far)
	}
}

func TestSearchHonoursThresholdAndLimit(t *testing.T) {
	db := open(t)

	db.AddFact("a", "one", []float32{1, 0, 0})
	db.AddFact("a", "two", []float32{0.9, 0.1, 0})
	db.AddFact("a", "three", []float32{0, 0, 1})

	got, err := db.Search([]float32{1, 0, 0}, 10, 0.5)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("threshold 0.5 returned %d results, want 2", len(got))
	}

	if got, _ = db.Search([]float32{1, 0, 0}, 1, -1); len(got) != 1 {
		t.Fatalf("limit 1 returned %d results", len(got))
	}
}

// Facts embedded by a different model must not pollute results.
func TestSearchSkipsOtherDimensions(t *testing.T) {
	db := open(t)

	keep, _ := db.AddFact("a", "same model", []float32{1, 0, 0})
	db.AddFact("a", "different model", []float32{1, 0, 0, 0})

	got, err := db.Search([]float32{1, 0, 0}, 10, -1)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}

	if len(got) != 1 || got[0].ID != keep {
		t.Fatalf("got %d results (%v), want only fact %d", len(got), got, keep)
	}
}

func TestBuildMapLinksEachPairOnce(t *testing.T) {
	db := open(t)

	// Two nearly identical facts and one unrelated: exactly one link.
	db.AddFact("project", "alpha", []float32{1, 0, 0})
	db.AddFact("project", "alpha again", []float32{0.99, 0.01, 0})
	db.AddFact("document", "elsewhere", []float32{0, 0, 1})

	m, err := db.BuildMap()
	if err != nil {
		t.Fatalf("building map: %v", err)
	}

	if len(m.Nodes) != 3 {
		t.Errorf("got %d nodes, want 3", len(m.Nodes))
	}

	if len(m.Links) != 1 {
		t.Fatalf("got %d links, want 1 (a pair must not be linked from both ends)", len(m.Links))
	}

	if m.Links[0].Source >= m.Links[0].Target {
		t.Errorf("link is not stored low-to-high: %+v", m.Links[0])
	}

	if m.Total != 3 {
		t.Errorf("total is %d, want 3", m.Total)
	}

	if m.Categories["project"] != 2 || m.Categories["document"] != 1 {
		t.Errorf("categories are %v", m.Categories)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	first.AddFact("a", "survives", []float32{1, 0, 0})
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopening must not re-run migrations: %v", err)
	}
	defer second.Close()

	n, err := second.CountFacts()
	if err != nil {
		t.Fatalf("counting: %v", err)
	}

	if n != 1 {
		t.Errorf("got %d facts after reopen, want 1", n)
	}
}

/*
 * The drive left, and came back as a different file.
 *
 * An open SQLite connection holds the file it opened, not the path it opened
 * it from. Unplug the drive and every write still reports success — they go to
 * an object that no longer has a name and will never be read again. Plug it
 * back in and the file at that path is a different object from the one being
 * written to, so the brain ends up holding two databases and disagreeing with
 * itself about what it knows.
 *
 * Nothing in the interface can show this. The counts keep going up, the
 * conversation saves, and it is all landing nowhere.
 */
func TestReopenFollowsTheFileRatherThanTheOneItOpened(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brain.sqlite")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.AddFact("test", "learned before the drive left", nil); err != nil {
		t.Fatal(err)
	}

	// The drive goes, and comes back carrying a file that is not the one this
	// connection is holding — which is what a remount is.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}

	returned, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, what := range []string{"one", "two"} {
		if _, err := returned.AddFact("test", what, nil); err != nil {
			t.Fatal(err)
		}
	}

	returned.Close()

	// The state this exists to fix: still reporting the vanished file.
	if n, _ := db.CountFacts(); n != 1 {
		t.Fatalf("expected the old connection to still see its own file, got %d facts", n)
	}

	if err := db.Reopen(); err != nil {
		t.Fatal(err)
	}

	n, err := db.CountFacts()
	if err != nil {
		t.Fatal(err)
	}

	if n != 2 {
		t.Errorf("after reopening it sees %d facts; the file on disk holds 2", n)
	}

	// And it is usable, not just readable: the pool was swapped, not detached.
	if _, err := db.AddFact("test", "learned after the drive came back", nil); err != nil {
		t.Errorf("writing after reopening: %v", err)
	}
}

/*
 * What has already been read, found by the path it was read from.
 *
 * A source is written "project:/path/to/it", not "/path/to/it", so asking for
 * everything beginning with a folder matched nothing — and a drive that had
 * just been read reported every file on it as new, at three seconds each. The
 * whole purpose of this query is not doing that work a second time.
 */
func TestKnownSourcesFindsWhatWasReadFromAFolder(t *testing.T) {
	db := open(t)

	const work = "/media/someone/work"

	for _, source := range []string{
		"project:" + work + "/alpha",
		"document:" + work + "/notes/plan.md",
		"project:/somewhere/else/beta",
		"", // a lesson from a conversation, which came from no file at all
	} {
		if _, err := db.AddLesson(0, "something", "validated", "high", source); err != nil {
			t.Fatal(err)
		}
	}

	seen, err := db.KnownSources(work)
	if err != nil {
		t.Fatal(err)
	}

	if len(seen) != 2 {
		t.Fatalf("found %d things read from %s, want 2: %v", len(seen), work, seen)
	}

	if !seen["project:"+work+"/alpha"] || !seen["document:"+work+"/notes/plan.md"] {
		t.Errorf("the wrong things came back: %v", seen)
	}

	if seen["project:/somewhere/else/beta"] {
		t.Error("a file from another folder was counted as read from this one")
	}
}
