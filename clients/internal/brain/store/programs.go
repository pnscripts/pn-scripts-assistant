package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The program inventory (package programs decides; this keeps).
 *
 * The source's id is sealed with the rest, so a program is found by
 * reading a source's rows and comparing in Go: a few thousand packages,
 * read when a source changed, which is seldom.
 */

// Programs is every remembered program of one kind, or of every kind for "",
// present and removed, in the order they were first written.
func (d *DB) Programs(kind programs.Kind) ([]programs.Record, error) {
	q := `SELECT id, kind, ident, name, version, path, exec, present, first_seen, changed_at,
	             removed_at, previous_version, initial, last_change FROM programs`

	var args []any

	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, string(kind))
	}

	rows, err := d.sql().Query(q+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading the programs: %w", err)
	}

	defer rows.Close()

	out := []programs.Record{}

	for rows.Next() {
		var (
			r                programs.Record
			kind             string
			present, initial int
			first, changed   string
			removed          sql.NullString
			last             string
		)

		if err := rows.Scan(&r.RowID, &kind, &r.ID, &r.Name, &r.Version, &r.Path, &r.Exec, &present,
			&first, &changed, &removed, &r.PreviousVersion, &initial, &last); err != nil {
			return nil, err
		}

		r.Kind, r.Present, r.Initial = programs.Kind(kind), present == 1, initial == 1
		r.LastChange = programs.ChangeKind(last)
		r.FirstSeen, _ = time.Parse(time.RFC3339Nano, first)
		r.ChangedAt, _ = time.Parse(time.RFC3339Nano, changed)

		if removed.Valid {
			at, err := time.Parse(time.RFC3339Nano, removed.String)
			if err == nil {
				r.RemovedAt = &at
			}
		}

		out = append(out, r)
	}

	return out, rows.Err()
}

// ProgramSourceLooked is whether a source has had its first look.
func (d *DB) ProgramSourceLooked(kind programs.Kind) (bool, error) {
	var n int

	err := d.sql().QueryRow(`SELECT COUNT(*) FROM program_sources WHERE kind = ?`, string(kind)).Scan(&n)

	return n > 0, err
}

// ProgramFirstLooks is when each source was first read, for the sources
// that have been.
func (d *DB) ProgramFirstLooks() (map[programs.Kind]time.Time, error) {
	rows, err := d.sql().Query(`SELECT kind, first_look FROM program_sources`)
	if err != nil {
		return nil, fmt.Errorf("reading the first looks: %w", err)
	}

	defer rows.Close()

	out := map[programs.Kind]time.Time{}

	for rows.Next() {
		var kind, at string

		if err := rows.Scan(&kind, &at); err != nil {
			return nil, err
		}

		if when, err := time.Parse(time.RFC3339Nano, at); err == nil {
			out[programs.Kind(kind)] = when
		}
	}

	return out, rows.Err()
}

// SavePrograms writes what programs.Reconcile returned, all or nothing:
// new records (RowID 0) are added, the others updated.
func (d *DB) SavePrograms(rs []programs.Record) error {
	return d.saveLook("", rs, time.Time{})
}

// SaveLook is SavePrograms for one look at a source, which it also marks as
// looked at (the first time), in the same transaction: a look that fails
// part way leaves the source as never looked at.
func (d *DB) SaveLook(kind programs.Kind, rs []programs.Record, at time.Time) error {
	if kind == "" {
		return errors.New("a look is at a source")
	}

	return d.saveLook(kind, rs, at)
}

func (d *DB) saveLook(kind programs.Kind, rs []programs.Record, at time.Time) error {
	if len(rs) == 0 && kind == "" {
		return nil
	}

	tx, err := d.sql().Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, r := range rs {
		if r.ID == "" || r.Kind == "" {
			return errors.New("a program needs a kind and an id")
		}

		var removed any
		if r.RemovedAt != nil {
			removed = r.RemovedAt.UTC().Format(time.RFC3339Nano)
		}

		present, initial := 0, 0
		if r.Present {
			present = 1
		}

		if r.Initial {
			initial = 1
		}

		first := r.FirstSeen.UTC().Format(time.RFC3339Nano)
		changed := r.ChangedAt.UTC().Format(time.RFC3339Nano)

		if r.RowID == 0 {
			_, err = tx.Exec(`INSERT INTO programs (kind, ident, name, version, path, exec, present,
				first_seen, changed_at, removed_at, previous_version, initial, last_change) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				string(r.Kind), d.seal(r.ID), d.seal(r.Name), d.seal(r.Version), d.seal(r.Path), d.seal(r.Exec),
				present, first, changed, removed, d.seal(r.PreviousVersion), initial, string(r.LastChange))
			if err != nil {
				return fmt.Errorf("writing down a program: %w", err)
			}

			continue
		}

		res, err := tx.Exec(`UPDATE programs SET kind = ?, ident = ?, name = ?, version = ?, path = ?, exec = ?,
			present = ?, first_seen = ?, changed_at = ?, removed_at = ?, previous_version = ?, initial = ?,
			last_change = ? WHERE id = ?`,
			string(r.Kind), d.seal(r.ID), d.seal(r.Name), d.seal(r.Version), d.seal(r.Path), d.seal(r.Exec),
			present, first, changed, removed, d.seal(r.PreviousVersion), initial, string(r.LastChange), r.RowID)
		if err != nil {
			return fmt.Errorf("changing a program: %w", err)
		}

		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("there is no program %d to change", r.RowID)
		}
	}

	if kind != "" {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO program_sources (kind, first_look) VALUES (?, ?)`,
			string(kind), at.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("writing down the look: %w", err)
		}
	}

	return tx.Commit()
}
