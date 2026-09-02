package store

import (
	"fmt"
	"time"
)

/*
 * Questions left with nothing after them.
 *
 * A reply is written when a turn finishes. That is right until a turn does not
 * finish: learning a folder takes over an hour on this machine, and anything
 * that ends the process meanwhile — a restart, a crash, the machine going to
 * sleep — leaves the question stored and no answer beside it. Nothing running
 * can fix that, because nothing is running.
 *
 * So it is repaired on the way back up. Five conversations were found holding
 * five questions and one answer between them, which reads as an assistant that
 * was asked things and ignored them. That is untrue, and it is the most
 * damaging thing a record of a conversation can say about the thing keeping it.
 */

// UnfinishedNote is what is written where an answer never arrived.
const UnfinishedNote = "That turn was cut short — PN Brain stopped before it could answer. " +
	"Nothing was lost except the answer; ask again."

/*
 * FinishAbandonedTurns writes a note into every conversation whose last word
 * was a question.
 *
 * Only the last message, and only when it is the owner's. A question with
 * anything after it was answered, and a conversation that ends with the
 * assistant is simply one nobody has replied to yet.
 */
func (d *DB) FinishAbandonedTurns() (int, error) {
	rows, err := d.sql().Query(`
		SELECT c.id
		FROM conversations c
		JOIN messages m ON m.id = (
			SELECT id FROM messages
			WHERE conversation_id = c.id
			ORDER BY id DESC LIMIT 1
		)
		WHERE m.role = 'user'`)
	if err != nil {
		return 0, fmt.Errorf("looking for unfinished turns: %w", err)
	}

	var dangling []int64

	for rows.Next() {
		var id int64

		if err := rows.Scan(&id); err != nil {
			rows.Close()

			return 0, err
		}

		dangling = append(dangling, id)
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range dangling {
		if _, err := d.AddMessage(id, "assistant", "", "", UnfinishedNote); err != nil {
			return 0, fmt.Errorf("closing conversation %d: %w", id, err)
		}

		// The thread has changed, so it sorts by when it was actually touched.
		if _, err := d.sql().Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return 0, err
		}
	}

	return len(dangling), nil
}
