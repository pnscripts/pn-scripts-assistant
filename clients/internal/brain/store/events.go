package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

/*
 * A diary, kept on this machine.
 *
 * Separate from reminders on purpose. A reminder speaks at a moment and is
 * then done with; an event occupies time, has an end, and is the thing that
 * answers "am I free on Thursday". Putting both in one table would mean every
 * query about one had to explain itself to the other.
 */
type Event struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`

	Starts time.Time `json:"starts_at"`
	Ends   time.Time `json:"ends_at"`

	// AllDay is a day somebody has taken rather than an hour they have booked.
	// Stored as its own flag rather than inferred from midnight-to-midnight,
	// because an event that genuinely runs from midnight is not the same thing.
	AllDay bool `json:"all_day"`

	Place string `json:"place,omitempty"`
	Notes string `json:"notes,omitempty"`

	// UID is the iCalendar identity, so importing the same file twice updates
	// what is there rather than doubling it.
	UID string `json:"uid,omitempty"`

	// CameFrom is where it was imported from, or empty when somebody put it in
	// here. Shown, so an event that appeared on its own can be told from one
	// that was typed.
	CameFrom string `json:"came_from,omitempty"`
}

// Length is how long it runs for.
func (e Event) Length() time.Duration { return e.Ends.Sub(e.Starts) }

const eventColumns = `id, COALESCE(uid,''), title, starts_at, ends_at, all_day,
	place, notes, COALESCE(came_from,'')`

func scanEvent(row interface{ Scan(...any) error }) (Event, error) {
	var (
		e            Event
		starts, ends string
	)

	err := row.Scan(&e.ID, &e.UID, &e.Title, &starts, &ends, &e.AllDay,
		&e.Place, &e.Notes, &e.CameFrom)
	if err != nil {
		return e, err
	}

	e.Starts, e.Ends = atTime(starts), atTime(ends)

	return e, nil
}

/*
 * PutInTheDiary records an event, or updates one with the same identity.
 *
 * The upsert is what makes importing a calendar file safe to do twice. Without
 * it, the second import of the same file is a diary with everything in it
 * twice, and there is no way to tell which copy to delete.
 */
func (d *DB) PutInTheDiary(e Event) (int64, error) {
	if strings.TrimSpace(e.Title) == "" {
		return 0, fmt.Errorf("an event with no name is not an event")
	}

	if e.Ends.Before(e.Starts) {
		return 0, fmt.Errorf("that ends before it starts")
	}

	if e.Ends.IsZero() {
		// An hour, which is what somebody means when they say a time and not
		// a length. A zero-length event shows as a line with no height.
		e.Ends = e.Starts.Add(time.Hour)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if e.UID != "" {
		var existing int64

		err := d.sql().QueryRow(`SELECT id FROM events WHERE uid = ?`, e.UID).Scan(&existing)

		if err == nil {
			e.ID = existing

			return existing, d.ChangeTheDiary(e)
		}

		if err != sql.ErrNoRows {
			return 0, err
		}
	}

	res, err := d.sql().Exec(`
		INSERT INTO events (uid, title, starts_at, ends_at, all_day, place, notes,
			came_from, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		e.UID, strings.TrimSpace(e.Title),
		e.Starts.UTC().Format(time.RFC3339), e.Ends.UTC().Format(time.RFC3339),
		e.AllDay, e.Place, e.Notes, e.CameFrom, now, now)
	if err != nil {
		return 0, fmt.Errorf("writing it in the diary: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if e.UID != "" {
		return id, nil
	}

	/*
	 * An identity of its own, kept, for everything that goes in without one.
	 *
	 * Generated at export time and not stored, this was the shape of a real
	 * bug: exporting the diary and reading the file straight back in produced
	 * a second copy of every event somebody had typed. The two that arrived in
	 * a file matched and updated; the ones this program made had a different
	 * identity each time they were written out, so they matched nothing.
	 */
	if _, err := d.sql().Exec(`UPDATE events SET uid = ? WHERE id = ?`,
		fmt.Sprintf("%d@pn-scripts-assistant", id), id); err != nil {
		return id, err
	}

	return id, nil
}

// ChangeTheDiary moves or rewrites an event that is already there.
func (d *DB) ChangeTheDiary(e Event) error {
	if e.Ends.Before(e.Starts) {
		return fmt.Errorf("that ends before it starts")
	}

	_, err := d.sql().Exec(`
		UPDATE events
		SET title = ?, starts_at = ?, ends_at = ?, all_day = ?, place = ?,
		    notes = ?, updated_at = ?
		WHERE id = ?`,
		strings.TrimSpace(e.Title),
		e.Starts.UTC().Format(time.RFC3339), e.Ends.UTC().Format(time.RFC3339),
		e.AllDay, e.Place, e.Notes, time.Now().UTC().Format(time.RFC3339), e.ID)

	return err
}

// Event reads one.
func (d *DB) Event(id int64) (*Event, error) {
	e, err := scanEvent(d.sql().QueryRow(
		`SELECT `+eventColumns+` FROM events WHERE id = ?`, id))

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &e, nil
}

/*
 * WhatIsOn is everything overlapping a window, soonest first.
 *
 * Overlapping rather than starting inside it, which is the difference between
 * "what is on today" and "what starts today". Something that began yesterday
 * and runs until Friday is very much on today, and a diary that hid it would
 * be answering a question nobody asked.
 */
func (d *DB) WhatIsOn(from, to time.Time) ([]Event, error) {
	rows, err := d.sql().Query(`
		SELECT `+eventColumns+` FROM events
		WHERE ends_at >= ? AND starts_at <= ?
		ORDER BY starts_at, id`,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Event{}

	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, e)
	}

	return out, rows.Err()
}

// CancelIt removes an event.
func (d *DB) CancelIt(id int64) error {
	_, err := d.sql().Exec(`DELETE FROM events WHERE id = ?`, id)

	return err
}

// CountEvents is how many are in the diary, for the interface.
func (d *DB) CountEvents() (int, error) {
	var n int

	err := d.sql().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)

	return n, err
}
