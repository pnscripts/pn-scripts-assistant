package store

import (
	"fmt"
	"sort"
	"time"
)

// Fact is one thing the brain durably knows.
type Fact struct {
	ID         int64
	Category   string
	Content    string
	Embedding  []float32
	Dimensions int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Scored pairs a fact with how well it matched a query.
type Scored struct {
	Fact
	Score float64
}

// AddFact stores a fact and returns its id.
func (d *DB) AddFact(category, content string, embedding []float32) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	var blob any
	var dims any

	if len(embedding) > 0 {
		blob = EncodeVector(embedding)
		dims = len(embedding)
	}

	res, err := d.sql.Exec(
		`INSERT INTO knowledge_facts (category, content, embedding, dimensions, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		category, content, blob, dims, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("storing fact: %w", err)
	}

	return res.LastInsertId()
}

// CountFacts reports how many facts exist, embedded or not.
func (d *DB) CountFacts() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM knowledge_facts`).Scan(&n)

	return n, err
}

// FactsByCategory reports the spread of what is known, for the interface.
func (d *DB) FactsByCategory() (map[string]int, error) {
	rows, err := d.sql.Query(`
		SELECT COALESCE(category, 'unknown'), COUNT(*)
		FROM knowledge_facts GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}

	for rows.Next() {
		var k string
		var n int

		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}

		out[k] = n
	}

	return out, rows.Err()
}

// loadEmbedded reads every fact that carries a usable vector.
//
// Rows whose dimensions differ from want are skipped rather than scored,
// because a vector from a different embedding model is not comparable to one
// from this model — the numbers would combine into a similarity that means
// nothing. Skipping is visible in the count; scoring would be silent nonsense.
func (d *DB) loadEmbedded(want int) ([]Fact, error) {
	rows, err := d.sql.Query(`
		SELECT id, COALESCE(category, 'unknown'), content, embedding, COALESCE(dimensions, 0)
		FROM knowledge_facts
		WHERE embedding IS NOT NULL
		ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Fact

	for rows.Next() {
		var f Fact
		var blob []byte

		if err := rows.Scan(&f.ID, &f.Category, &f.Content, &blob, &f.Dimensions); err != nil {
			return nil, err
		}

		if want > 0 && f.Dimensions != want {
			continue
		}

		v, err := DecodeVector(blob)
		if err != nil {
			continue
		}

		f.Embedding = v
		out = append(out, f)
	}

	return out, rows.Err()
}

// Search returns the facts most similar to the query vector, best first.
//
// This is a full scan and a sort. At the size a personal brain reaches — tens of
// thousands of facts at the very most — that is a few milliseconds and it is
// exact, where an approximate index would trade correctness for a speed nobody
// here would notice. If this ever holds hundreds of thousands of facts the scan
// is the thing to replace, and the signature will not have to change.
func (d *DB) Search(query []float32, limit int, minScore float64) ([]Scored, error) {
	if len(query) == 0 {
		return nil, nil
	}

	facts, err := d.loadEmbedded(len(query))
	if err != nil {
		return nil, err
	}

	scored := make([]Scored, 0, len(facts))

	for _, f := range facts {
		s := Cosine(query, f.Embedding)

		if s < minScore {
			continue
		}

		scored = append(scored, Scored{Fact: f, Score: s})
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	return scored, nil
}

// MapNode and MapLink describe the picture the interface draws.
type MapNode struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
	Label    string `json:"label"`
}

type MapLink struct {
	Source   int64   `json:"source"`
	Target   int64   `json:"target"`
	Strength float64 `json:"strength"`
}

type MemoryMap struct {
	Nodes      []MapNode      `json:"nodes"`
	Links      []MapLink      `json:"links"`
	Total      int            `json:"total"`
	Categories map[string]int `json:"categories"`
}

// BuildMap turns the stored vectors into nodes and the links between them.
//
// The constants match what the Postgres version used, so the picture does not
// change shape as a result of the port: three neighbours per node, nothing
// weaker than 0.55 (below which everything is faintly like everything), and a
// ceiling on nodes because this is redrawn every frame on a canvas.
func (d *DB) BuildMap() (MemoryMap, error) {
	const (
		linksPerNode  = 3
		minSimilarity = 0.55
		maxNodes      = 220
	)

	out := MemoryMap{Nodes: []MapNode{}, Links: []MapLink{}}

	total, err := d.CountFacts()
	if err != nil {
		return out, err
	}
	out.Total = total

	if out.Categories, err = d.FactsByCategory(); err != nil {
		return out, err
	}

	facts, err := d.loadEmbedded(0)
	if err != nil {
		return out, err
	}

	if len(facts) > maxNodes {
		facts = facts[:maxNodes]
	}

	for _, f := range facts {
		out.Nodes = append(out.Nodes, MapNode{
			ID:       f.ID,
			Category: f.Category,
			Label:    truncate(f.Content, 60),
		})
	}

	// Each node keeps its strongest few neighbours. A link found from both ends
	// is one edge, so the pair is recorded once — drawing it twice doubles the
	// work and darkens the line for no reason.
	seen := map[[2]int64]bool{}

	for i := range facts {
		type cand struct {
			id    int64
			score float64
		}

		var best []cand

		for j := range facts {
			if i == j || facts[i].Dimensions != facts[j].Dimensions {
				continue
			}

			s := Cosine(facts[i].Embedding, facts[j].Embedding)

			if s < minSimilarity {
				continue
			}

			best = append(best, cand{facts[j].ID, s})
		}

		sort.Slice(best, func(a, b int) bool { return best[a].score > best[b].score })

		if len(best) > linksPerNode {
			best = best[:linksPerNode]
		}

		for _, c := range best {
			key := [2]int64{min64(facts[i].ID, c.id), max64(facts[i].ID, c.id)}

			if seen[key] {
				continue
			}

			seen[key] = true
			out.Links = append(out.Links, MapLink{
				Source:   key[0],
				Target:   key[1],
				Strength: round3(c.score),
			})
		}
	}

	return out, nil
}

func truncate(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

func round3(f float64) float64 {
	return float64(int64(f*1000+0.5)) / 1000
}

// AllFacts returns every fact's id and text, for re-embedding.
//
// Without the vectors: the caller is about to replace them, and loading a few
// hundred blobs it is going to discard is work for nothing.
func (d *DB) AllFacts() ([]Fact, error) {
	rows, err := d.sql.Query(`
		SELECT id, COALESCE(category, 'unknown'), content
		FROM knowledge_facts
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Fact

	for rows.Next() {
		var f Fact

		if err := rows.Scan(&f.ID, &f.Category, &f.Content); err != nil {
			return nil, err
		}

		out = append(out, f)
	}

	return out, rows.Err()
}

// ReplaceEmbedding writes a new vector for a fact.
//
// Used when the embedding model changes. Every vector has to be replaced or
// none of them should be: a search compares the question's vector against all
// of them at once, and a table holding two models' output would rank by which
// model produced each row rather than by meaning.
func (d *DB) ReplaceEmbedding(id int64, embedding []float32) error {
	_, err := d.sql.Exec(
		`UPDATE knowledge_facts SET embedding = ?, dimensions = ?, updated_at = ? WHERE id = ?`,
		EncodeVector(embedding), len(embedding), time.Now().UTC().Format(time.RFC3339), id)

	return err
}
