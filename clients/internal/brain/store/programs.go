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
	             removed_at, previous_version FROM programs`

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
			r              programs.Record
			kind           string
			present        int
			first, changed string
			removed        sql.NullString
		)

		if err := rows.Scan(&r.RowID, &kind, &r.ID, &r.Name, &r.Version, &r.Path, &r.Exec, &present,
			&first, &changed, &removed, &r.PreviousVersion); err != nil {
			return nil, err
		}

		r.Kind, r.Present = programs.Kind(kind), present == 1
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

// SavePrograms writes what programs.Reconcile returned, all or nothing:
// new records (RowID 0) are added, the others updated.
func (d *DB) SavePrograms(rs []programs.Record) error {
	if len(rs) == 0 {
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

		present := 0
		if r.Present {
			present = 1
		}

		first := r.FirstSeen.UTC().Format(time.RFC3339Nano)
		changed := r.ChangedAt.UTC().Format(time.RFC3339Nano)

		if r.RowID == 0 {
			_, err = tx.Exec(`INSERT INTO programs (kind, ident, name, version, path, exec, present,
				first_seen, changed_at, removed_at, previous_version) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				string(r.Kind), d.seal(r.ID), d.seal(r.Name), d.seal(r.Version), d.seal(r.Path), d.seal(r.Exec),
				present, first, changed, removed, d.seal(r.PreviousVersion))
			if err != nil {
				return fmt.Errorf("writing down a program: %w", err)
			}

			continue
		}

		res, err := tx.Exec(`UPDATE programs SET kind = ?, ident = ?, name = ?, version = ?, path = ?, exec = ?,
			present = ?, first_seen = ?, changed_at = ?, removed_at = ?, previous_version = ? WHERE id = ?`,
			string(r.Kind), d.seal(r.ID), d.seal(r.Name), d.seal(r.Version), d.seal(r.Path), d.seal(r.Exec),
			present, first, changed, removed, d.seal(r.PreviousVersion), r.RowID)
		if err != nil {
			return fmt.Errorf("changing a program: %w", err)
		}

		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("there is no program %d to change", r.RowID)
		}
	}

	return tx.Commit()
}
