package store

import (
	"fmt"
	"strings"
	"time"
)

// PathRewrite is one substitution to apply across stored text.
type PathRewrite struct {
	From string
	To   string
}

// Rewritten is a fact whose text changed and therefore needs re-embedding.
type Rewritten struct {
	ID      int64
	Old     string
	New     string
	Table   string
	Changed bool
}

// RewritePaths replaces stale paths in facts and lessons.
//
// This exists because the brain learned everything it knows while running
// inside a container, where the owner's disk appeared at /mnt/scan. Outside the
// container those paths do not exist, so every memory pointed somewhere real
// that could no longer be opened — the knowledge was correct and unusable at
// the same time.
//
// The caller must re-embed whatever this returns. Changing the text without
// recomputing the vector leaves the two describing different things, and the
// mismatch is invisible: recall keeps working, just slightly wrong, forever.
// This deliberately does not embed anything itself, so that requirement is
// visible in the signature rather than buried.
func (d *DB) RewritePaths(rules []PathRewrite) ([]Rewritten, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	var out []Rewritten

	for _, table := range []string{"knowledge_facts", "lessons"} {
		rows, err := d.sql.Query(`SELECT id, content FROM ` + table)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}

		var pending []Rewritten

		for rows.Next() {
			var r Rewritten
			r.Table = table

			if err := rows.Scan(&r.ID, &r.Old); err != nil {
				rows.Close()

				return nil, err
			}

			r.New = r.Old

			// Longest prefix first, so /mnt/scan/dev-projects is not partly
			// consumed by a rule for /mnt/scan.
			for _, rule := range sortedByLengthDesc(rules) {
				r.New = strings.ReplaceAll(r.New, rule.From, rule.To)
			}

			if r.New != r.Old {
				r.Changed = true
				pending = append(pending, r)
			}
		}

		rows.Close()

		if err := rows.Err(); err != nil {
			return nil, err
		}

		now := time.Now().UTC().Format(time.RFC3339)

		for _, r := range pending {
			_, err := d.sql.Exec(
				`UPDATE `+r.Table+` SET content = ?, updated_at = ? WHERE id = ?`,
				r.New, now, r.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("updating %s %d: %w", r.Table, r.ID, err)
			}
		}

		out = append(out, pending...)
	}

	return out, nil
}

// SetEmbedding replaces a row's vector after its text changed.
func (d *DB) SetEmbedding(table string, id int64, v []float32) error {
	if table != "knowledge_facts" && table != "lessons" {
		return fmt.Errorf("refusing to write an embedding into %q", table)
	}

	_, err := d.sql.Exec(
		`UPDATE `+table+` SET embedding = ?, dimensions = ? WHERE id = ?`,
		EncodeVector(v), len(v), id,
	)

	return err
}

func sortedByLengthDesc(rules []PathRewrite) []PathRewrite {
	out := append([]PathRewrite(nil), rules...)

	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j].From) > len(out[j-1].From); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}
