package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Reminder is something the brain has to say at a particular time.
type Reminder struct {
	ID        int64      `json:"id"`
	What      string     `json:"what"`
	At        time.Time  `json:"at"`
	SaidAt    *time.Time `json:"said_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// AddReminder records something to say later.
func (d *DB) AddReminder(what string, at time.Time) (Reminder, error) {
	now := time.Now().UTC()

	res, err := d.sql().Exec(
		`INSERT INTO reminders (what, at, created_at) VALUES (?, ?, ?)`,
		what, at.UTC().Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return Reminder{}, fmt.Errorf("saving the reminder: %w", err)
	}

	id, _ := res.LastInsertId()

	return Reminder{ID: id, What: what, At: at, CreatedAt: now}, nil
}

// Reminders lists what is still to come, soonest first.
func (d *DB) Reminders(includeSaid bool, limit int) ([]Reminder, error) {
	query := `SELECT id, what, at, said_at, created_at FROM reminders`
	if !includeSaid {
		query += ` WHERE said_at IS NULL`
	}

	query += ` ORDER BY at LIMIT ?`

	rows, err := d.sql().Query(query, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanReminders(rows)
}

/*
 * DueReminders returns what should be said by now and has not been.
 *
 * The read and the marking are separate on purpose: something is only marked as
 * said once it has actually been said, so a brain that is stopped between the
 * two still owes the reminder rather than having silently swallowed it.
 */
func (d *DB) DueReminders(now time.Time) ([]Reminder, error) {
	rows, err := d.sql().Query(
		`SELECT id, what, at, said_at, created_at FROM reminders
		 WHERE said_at IS NULL AND at <= ? ORDER BY at`,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanReminders(rows)
}

// MarkReminderSaid records that somebody has been told.
func (d *DB) MarkReminderSaid(id int64, when time.Time) error {
	_, err := d.sql().Exec(`UPDATE reminders SET said_at = ? WHERE id = ?`,
		when.UTC().Format(time.RFC3339), id)

	return err
}

// ForgetReminder removes one.
func (d *DB) ForgetReminder(id int64) error {
	res, err := d.sql().Exec(`DELETE FROM reminders WHERE id = ?`, id)
	if err != nil {
		return err
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("there is no reminder %d", id)
	}

	return nil
}

func scanReminders(rows *sql.Rows) ([]Reminder, error) {
	var out []Reminder

	for rows.Next() {
		var (
			r          Reminder
			at, create string
			said       sql.NullString
		)

		if err := rows.Scan(&r.ID, &r.What, &at, &said, &create); err != nil {
			return nil, err
		}

		r.At, _ = time.Parse(time.RFC3339, at)
		r.CreatedAt, _ = time.Parse(time.RFC3339, create)

		if said.Valid {
			if t, err := time.Parse(time.RFC3339, said.String); err == nil {
				r.SaidAt = &t
			}
		}

		out = append(out, r)
	}

	return out, rows.Err()
}
