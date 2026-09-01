package store

import "time"

// RecentFact is a fact as the knowledge panel shows it: no vector, because a
// 768-number array per row would dwarf the text it belongs to.
type RecentFact struct {
	ID        int64     `json:"id"`
	Category  string    `json:"category"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// RecentFacts returns the newest facts, newest first.
func (d *DB) RecentFacts(limit int) ([]RecentFact, error) {
	rows, err := d.sql.Query(`
		SELECT id, COALESCE(category,'unknown'), content, created_at
		FROM knowledge_facts ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RecentFact{}

	for rows.Next() {
		var f RecentFact
		var created string

		if err := rows.Scan(&f.ID, &f.Category, &f.Content, &created); err != nil {
			return nil, err
		}

		f.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, f)
	}

	return out, rows.Err()
}

// CountLessonsByStatus reports how many lessons sit at each stage of the
// quarantine pipeline.
func (d *DB) CountLessonsByStatus() (map[string]int, error) {
	rows, err := d.sql.Query(`SELECT status, COUNT(*) FROM lessons GROUP BY status`)
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

// CountPendingLessons is what the interface shows as "waiting": anything that
// has not yet been accepted or rejected.
func (d *DB) CountPendingLessons() (int, error) {
	var n int

	/*
	 * What is waiting for a person, which is not the same as unfinished.
	 *
	 * Counting everything that is neither promoted nor rejected swept in
	 * "validated" — a lesson checked against reality and waiting for the
	 * machine to promote it, which needs nobody. Four of those left stranded
	 * by an interrupted scan made the interface say "4 awaiting review" beside
	 * a panel correctly reading "Nothing needs a decision", because the count
	 * and the list were answering different questions under one name.
	 *
	 * Proposed is the one that means a person: nothing here can verify it.
	 */
	err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM lessons WHERE status = 'proposed'`,
	).Scan(&n)

	return n, err
}

// CountConversations reports how many threads exist.
func (d *DB) CountConversations() (int, error) {
	var n int

	err := d.sql.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&n)

	return n, err
}
