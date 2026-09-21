package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"pn-scripts-assistant/internal/protocol"
)

/*
 * What happened, in the order it happened.
 *
 * Kept because a client is not always there. A phone asleep on a train, a
 * window that was closed, an execution node that lost its connection halfway
 * through a build — each comes back and has to be told what it missed, and
 * "here is the state now" is not the same answer: it cannot say that the
 * check failed twice before it passed, and that is usually the part worth
 * knowing.
 *
 * So every event is written down with a number, and catching up is asking for
 * everything after a number. That is also what makes the record an audit: the
 * same rows answer "what is happening" and "what happened", and there is no
 * second, quieter log that could disagree with the first.
 */

/*
 * Happening is one recorded event, as it came off the wire.
 *
 * Not store.Activity, which is the sentence the interface shows in its
 * activity panel — a summary of messages and lessons, built for reading. This
 * is the machine-readable record a client catches up from, and the two must
 * not be the same type: one is allowed to change shape whenever the panel
 * does, and the other is a protocol.
 */
type Happening struct {
	Seq  int64           `json:"seq"`
	ID   string          `json:"id"`
	Type protocol.Kind   `json:"type"`
	Task int64           `json:"task,omitempty"`
	Step int64           `json:"step,omitempty"`
	Item string          `json:"device,omitempty"`
	Said string          `json:"said,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
	At   time.Time       `json:"at"`
}

// Envelope is the event again, in the shape it is sent in.
func (a Happening) Envelope() protocol.Envelope {
	return protocol.Envelope{
		V: protocol.Version, Seq: a.Seq, ID: a.ID, At: a.At, Type: a.Type,
		Task: a.Task, Step: a.Step, Device: a.Item, Said: a.Said, Data: a.Data,
	}
}

/*
 * RecordActivity writes one event down and gives it its number.
 *
 * The number comes from the database rather than from the caller, so that two
 * things happening at once cannot claim the same place in the order — which
 * is the one property everything reading this depends on.
 */
func (d *DB) RecordHappening(e protocol.Envelope) (int64, error) {
	if e.ID == "" {
		e.ID = protocol.NewID("evt")
	}

	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}

	res, err := d.sql().Exec(`
		INSERT INTO activity (id, type, task_id, step_id, device, said, data, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, string(e.Type), nullable(e.Task), nullable(e.Step), e.Device, e.Said,
		string(e.Data), e.At.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("recording what happened: %w", err)
	}

	return res.LastInsertId()
}

// Happenings is everything after seq, oldest first, at most most of them. A seq
// of zero is the beginning.
func (d *DB) Happenings(after int64, most int) ([]Happening, error) {
	if most <= 0 {
		most = 200
	}

	rows, err := d.sql().Query(`
		SELECT seq, id, type, COALESCE(task_id,0), COALESCE(step_id,0), device, said, data, created_at
		FROM activity WHERE seq > ? ORDER BY seq LIMIT ?`, after, most)
	if err != nil {
		return nil, fmt.Errorf("reading what happened: %w", err)
	}

	defer rows.Close()

	return scanHappenings(rows)
}

// HappeningsFor is everything recorded about one task, oldest first.
func (d *DB) HappeningsFor(taskID int64) ([]Happening, error) {
	rows, err := d.sql().Query(`
		SELECT seq, id, type, COALESCE(task_id,0), COALESCE(step_id,0), device, said, data, created_at
		FROM activity WHERE task_id = ? ORDER BY seq`, taskID)
	if err != nil {
		return nil, fmt.Errorf("reading what happened to task %d: %w", taskID, err)
	}

	defer rows.Close()

	return scanHappenings(rows)
}

// LastHappening is the number of the last event, which is where a client
// with no history of its own starts from.
func (d *DB) LastHappening() (int64, error) {
	var seq sql.NullInt64

	if err := d.sql().QueryRow(`SELECT MAX(seq) FROM activity`).Scan(&seq); err != nil {
		return 0, err
	}

	return seq.Int64, nil
}

/*
 * KeepHappenings is how much of the record is worth keeping.
 *
 * Enough that a client away for a week comes back to a complete answer on a
 * machine used every day, and small enough that the file stays a file. At
 * about 300 bytes an event this is some fifteen megabytes.
 */
const KeepHappenings = 50_000

/*
 * TrimHappenings keeps the newest rows and drops the rest.
 *
 * A log nobody prunes is a database that grows for as long as the program is
 * used, on a machine whose disk is also its owner's. Kept by count rather
 * than by age because what matters is being able to answer a client that has
 * been away, and a quiet fortnight should not throw away less than a busy
 * afternoon.
 */
func (d *DB) TrimHappenings(keep int64) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}

	res, err := d.sql().Exec(`
		DELETE FROM activity WHERE seq <= (SELECT MAX(seq) - ? FROM activity)`, keep)
	if err != nil {
		return 0, fmt.Errorf("trimming what happened: %w", err)
	}

	return res.RowsAffected()
}

func scanHappenings(rows *sql.Rows) ([]Happening, error) {
	out := []Happening{}

	for rows.Next() {
		var (
			a    Happening
			kind string
			data string
			at   string
		)

		if err := rows.Scan(&a.Seq, &a.ID, &kind, &a.Task, &a.Step, &a.Item, &a.Said, &data, &at); err != nil {
			return nil, err
		}

		a.Type = protocol.Kind(kind)
		a.At = atTime(at)

		if data != "" {
			a.Data = json.RawMessage(data)
		}

		out = append(out, a)
	}

	return out, rows.Err()
}
