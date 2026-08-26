package store

import (
	"math"
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

func TestImportRefusesNonEmptyDatabase(t *testing.T) {
	db := open(t)
	db.AddFact("a", "already here", []float32{1, 0, 0})

	if _, err := db.ImportPostgresExport(t.TempDir()); err == nil {
		t.Fatal("expected import into a populated database to be refused")
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
