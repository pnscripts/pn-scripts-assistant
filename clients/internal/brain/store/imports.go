package store

import (
	"strings"
	"time"
)

// Where an import run has got to.
const (
	ImportRunning = "running"
	ImportDone    = "done"
	ImportStopped = "stopped"
	ImportFailed  = "failed"
)

// ImportRun is one import, as far as it got. See migration 14.
type ImportRun struct {
	ID       int64     `json:"id"`
	Source   string    `json:"source"`
	From     string    `json:"from"`
	State    string    `json:"state"`
	Stage    string    `json:"stage"`
	Done     int       `json:"done"`
	Of       int       `json:"of"`
	So       Imported  `json:"so_far"`
	Notes    []string  `json:"notes,omitempty"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started_at"`
	Updated  time.Time `json:"updated_at"`
	Finished time.Time `json:"finished_at,omitempty"`
}

// StartImport records that an import has begun.
func (d *DB) StartImport(source, from string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(`INSERT INTO imports (source, from_path, state, started_at, updated_at)
		VALUES (?,?,?,?,?)`, source, from, ImportRunning, now, now)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

/*
 * ImportProgress writes how far an import has got.
 *
 * Totals, not increments: the importer keeps its own running sum and says
 * what it is, so a write that is lost costs one out-of-date number rather
 * than a count that is wrong forever.
 */
func (d *DB) ImportProgress(id int64, stage string, done, of int, so Imported, notes []string) error {
	_, err := d.sql().Exec(`
		UPDATE imports SET stage = ?, done = ?, of = ?, jobs = ?, capabilities = ?, links = ?,
		       filled = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		stage, done, of, so.Jobs, so.Capabilities, so.Links, so.Filled,
		strings.Join(notes, "\n"), time.Now().UTC().Format(time.RFC3339), id)

	return err
}

// FinishImport closes a run, saying why when it did not finish.
func (d *DB) FinishImport(id int64, state, why string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.sql().Exec(`UPDATE imports SET state = ?, error = ?, updated_at = ?, finished_at = ?
		WHERE id = ?`, state, why, now, now, id)

	return err
}

// Imports is the most recent runs, newest first.
func (d *DB) Imports(limit int) ([]ImportRun, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := d.sql().Query(`
		SELECT id, source, from_path, state, stage, done, of, jobs, capabilities, links,
		       filled, notes, error, started_at, updated_at, COALESCE(finished_at,'')
		FROM imports ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []ImportRun{}

	for rows.Next() {
		var (
			r                         ImportRun
			notes, started, at, ended string
		)

		if err := rows.Scan(&r.ID, &r.Source, &r.From, &r.State, &r.Stage, &r.Done, &r.Of,
			&r.So.Jobs, &r.So.Capabilities, &r.So.Links, &r.So.Filled, &notes, &r.Error,
			&started, &at, &ended); err != nil {
			return nil, err
		}

		r.Notes = splitList(notes)
		r.Started, r.Updated, r.Finished = atTime(started), atTime(at), atTime(ended)

		out = append(out, r)
	}

	return out, rows.Err()
}

/*
 * InterruptRunningImports marks runs the last program left behind as stopped.
 *
 * Called at start, when nothing can be importing: a row still saying running
 * then is a run that was cut off, and saying so is what tells somebody to
 * press the button again rather than wait for it.
 */
func (d *DB) InterruptRunningImports() error {
	_, err := d.sql().Exec(`UPDATE imports SET state = ?, error = ?, finished_at = ?
		WHERE state = ?`, ImportStopped, "the program was closed while it ran",
		time.Now().UTC().Format(time.RFC3339), ImportRunning)

	return err
}

/*
 * Labels is every job and capability by name and alias, and nothing else.
 *
 * What an importer matches incoming names against, so a job the catalogue
 * already has under another source's spelling is enriched rather than
 * duplicated. Names only, because three thousand full rows with their links
 * would be read for the sake of their titles.
 */
func (d *DB) Labels() (jobs, capabilities []Occupation, err error) {
	read := func(query string) ([]Occupation, error) {
		rows, err := d.sql().Query(query)
		if err != nil {
			return nil, err
		}

		defer rows.Close()

		out := []Occupation{}

		for rows.Next() {
			var (
				o                Occupation
				aliases, sources string
			)

			if err := rows.Scan(&o.ID, &o.Title, &aliases, &sources); err != nil {
				return nil, err
			}

			o.Aliases = splitList(aliases)
			o.Sources = splitList(sources)
			out = append(out, o)
		}

		return out, rows.Err()
	}

	if jobs, err = read(`SELECT id, title, aliases, COALESCE(sources,'') FROM occupations`); err != nil {
		return nil, nil, err
	}

	capabilities, err = read(`SELECT id, name, aliases, '' FROM capabilities`)

	return jobs, capabilities, err
}
