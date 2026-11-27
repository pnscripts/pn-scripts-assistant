package store

import (
	"fmt"
	"strings"
	"time"
)

// TidyDelete is what a TidyLessonsOnce decision returns to remove a lesson.
const TidyDelete = "delete"

/*
 * TidyLessonsOnce walks the lessons waiting for a person whose source starts
 * with prefix and does to each what decide says: TidyDelete removes it, a
 * status moves it there, "" leaves it. It runs once per brain under the name
 * chore, and reports what it did by outcome; nothing, when the chore was
 * already done.
 *
 * For an older version's mistakes, which are in the data and so outlive the
 * fix in the code: 0.13 filed the same readings of the knowledge folder again
 * every ninety seconds until the review queue passed what reading allows.
 *
 * One transaction: a power cut part way leaves the lessons as they were and
 * the chore not done, and the next start does it again from the beginning.
 * The decision is made in Go because the words are sealed, each copy with a
 * nonce of its own, so two copies of one sentence differ on disk.
 */
func (d *DB) TidyLessonsOnce(chore, prefix string, decide func(Lesson) string) (map[string]int, error) {
	tx, err := d.sql().Begin()
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	var done int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM chores WHERE name = ?`, chore).Scan(&done); err != nil {
		return nil, fmt.Errorf("asking whether %s was done: %w", chore, err)
	}

	if done > 0 {
		return map[string]int{}, nil
	}

	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix) + "%"

	rows, err := tx.Query(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE status = 'proposed' AND source LIKE ? ESCAPE '\' ORDER BY id`, like)
	if err != nil {
		return nil, err
	}

	var lessons []Lesson

	for rows.Next() {
		var l Lesson
		var created string

		if err := rows.Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status,
			&l.Confidence, &l.Source, &created); err != nil {
			rows.Close()

			return nil, err
		}

		l.CreatedAt, _ = time.Parse(time.RFC3339, created)
		lessons = append(lessons, l)
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	out := map[string]int{}

	for _, l := range lessons {
		// LIKE is case-insensitive for ASCII; the prefix is not.
		if !strings.HasPrefix(l.Source, prefix) {
			continue
		}

		switch outcome := decide(l); outcome {
		case "":
			continue
		case TidyDelete:
			if _, err := tx.Exec(`DELETE FROM lessons WHERE id = ?`, l.ID); err != nil {
				return nil, fmt.Errorf("removing lesson %d: %w", l.ID, err)
			}

			out[outcome]++
		default:
			if _, err := tx.Exec(`UPDATE lessons SET status = ?, updated_at = ? WHERE id = ?`,
				outcome, now, l.ID); err != nil {
				return nil, fmt.Errorf("moving lesson %d: %w", l.ID, err)
			}

			out[outcome]++
		}
	}

	if _, err := tx.Exec(`INSERT INTO chores (name, done_at) VALUES (?, ?)`, chore, now); err != nil {
		return nil, fmt.Errorf("recording %s: %w", chore, err)
	}

	return out, tx.Commit()
}

/*
 * ClearDeedsAskedOnce empties what was asked on every line of the record
 * kept under tool whose asked words begin with prefix, once per brain under
 * the name chore, and says how many lines it cleared; none, when the chore
 * was already done. Everything else on those lines (the summary, the
 * outcome, the time) stays as it was.
 *
 * For 1.3.0, which wrote the command behind an action into the record that
 * programs on this computer and paired devices read (ASSISTANT-ACTION-
 * COMMAND-RECORD-1127). One transaction, decided in Go, as TidyLessonsOnce:
 * the words are sealed, so SQL cannot tell which lines begin with prefix.
 */
func (d *DB) ClearDeedsAskedOnce(chore, tool, prefix string) (int, error) {
	tx, err := d.sql().Begin()
	if err != nil {
		return 0, err
	}

	defer tx.Rollback()

	var done int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM chores WHERE name = ?`, chore).Scan(&done); err != nil {
		return 0, fmt.Errorf("asking whether %s was done: %w", chore, err)
	}

	if done > 0 {
		return 0, nil
	}

	rows, err := tx.Query(`SELECT id, COALESCE(asked,'') FROM deeds WHERE tool = ? ORDER BY id`, tool)
	if err != nil {
		return 0, err
	}

	var clear []int64

	for rows.Next() {
		var id int64
		var asked string

		if err := rows.Scan(&id, &asked); err != nil {
			rows.Close()

			return 0, err
		}

		if strings.HasPrefix(asked, prefix) {
			clear = append(clear, id)
		}
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range clear {
		if _, err := tx.Exec(`UPDATE deeds SET asked = ? WHERE id = ?`, d.seal(""), id); err != nil {
			return 0, fmt.Errorf("clearing line %d of the record: %w", id, err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := tx.Exec(`INSERT INTO chores (name, done_at) VALUES (?, ?)`, chore, now); err != nil {
		return 0, fmt.Errorf("recording %s: %w", chore, err)
	}

	return len(clear), tx.Commit()
}
