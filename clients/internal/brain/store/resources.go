package store

import (
	"database/sql"
	"strings"
	"time"
)

// Results of giving a resource work.
const (
	RunOK       = "ok"
	RunFailed   = "failed"
	RunSwitched = "switched"
)

// ResourceRun is one time a resource was given work.
type ResourceRun struct {
	ID       int64     `json:"id"`
	Resource string    `json:"resource"`
	Kind     string    `json:"kind"`
	Work     string    `json:"work,omitempty"`
	Model    string    `json:"model,omitempty"`
	TaskID   int64     `json:"task_id,omitempty"`
	StepID   int64     `json:"step_id,omitempty"`
	Result   string    `json:"result"`
	Failure  string    `json:"failure,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	Millis   int64     `json:"millis"`
	Verified bool      `json:"verified,omitempty"`
	At       time.Time `json:"at"`
}

// RecordRun keeps one run.
func (d *DB) RecordRun(r ResourceRun) error {
	_, err := d.sql().Exec(`INSERT INTO resource_runs (resource, kind, work, model, task_id, step_id,
		result, failure, detail, millis, verified, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.Resource, r.Kind, r.Work, r.Model, nullable(r.TaskID), nullable(r.StepID), r.Result,
		r.Failure, strings.TrimSpace(r.Detail), r.Millis, r.Verified, time.Now().UTC().Format(time.RFC3339))

	return err
}

/*
 * VerifyWriting marks what was written for a task as verified, once this
 * program's own check has passed on it: every run of an agent or a model on
 * the task that finished and was not yet verified.
 */
func (d *DB) VerifyWriting(taskID int64) error {
	_, err := d.sql().Exec(`UPDATE resource_runs SET verified = 1
		WHERE task_id = ? AND kind IN ('agent','model') AND result = ? AND verified = 0`, taskID, RunOK)

	return err
}

// Runs is a resource's latest runs, newest first; "" is every resource's.
func (d *DB) Runs(resource string, most int) ([]ResourceRun, error) {
	query := `SELECT id, resource, kind, work, model, COALESCE(task_id,0), COALESCE(step_id,0), result,
		failure, detail, millis, verified, created_at FROM resource_runs`

	args := []any{}

	if resource != "" {
		query += ` WHERE resource = ?`
		args = append(args, resource)
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, most)

	rows, err := d.sql().Query(query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []ResourceRun

	for rows.Next() {
		var (
			r  ResourceRun
			at string
		)

		if err := rows.Scan(&r.ID, &r.Resource, &r.Kind, &r.Work, &r.Model, &r.TaskID, &r.StepID,
			&r.Result, &r.Failure, &r.Detail, &r.Millis, &r.Verified, &at); err != nil {
			return nil, err
		}

		r.At = atTime(at)
		out = append(out, r)
	}

	return out, rows.Err()
}

// Usage is the last reading of a subscription's capacity.
type Usage struct {
	Resource string `json:"resource"`
	State    string `json:"state"`

	// Remaining is the percentage left, nil when the service does not say.
	Remaining *float64  `json:"remaining,omitempty"`
	Window    string    `json:"window,omitempty"`
	ResetsAt  time.Time `json:"resets_at,omitempty"`
	Plan      string    `json:"plan,omitempty"`
	Source    string    `json:"source,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	At        time.Time `json:"observed_at"`
}

// NoteUsage keeps the latest reading of one resource's capacity.
func (d *DB) NoteUsage(u Usage) error {
	var remaining any
	if u.Remaining != nil {
		remaining = *u.Remaining
	}

	var resets any
	if !u.ResetsAt.IsZero() {
		resets = u.ResetsAt.UTC().Format(time.RFC3339)
	}

	_, err := d.sql().Exec(`INSERT INTO resource_usage (resource, state, remaining, window, resets_at,
		plan, source, detail, observed_at) VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(resource) DO UPDATE SET state=excluded.state, remaining=excluded.remaining,
		window=excluded.window, resets_at=excluded.resets_at, plan=excluded.plan,
		source=excluded.source, detail=excluded.detail, observed_at=excluded.observed_at`,
		u.Resource, u.State, remaining, u.Window, resets, u.Plan, u.Source, u.Detail,
		time.Now().UTC().Format(time.RFC3339))

	return err
}

// UsageOf is the latest reading for a resource, or nil when there is none.
func (d *DB) UsageOf(resource string) (*Usage, error) {
	var (
		u         Usage
		remaining sql.NullFloat64
		resets    sql.NullString
		at        string
	)

	err := d.sql().QueryRow(`SELECT resource, state, remaining, window, resets_at, plan, source, detail,
		observed_at FROM resource_usage WHERE resource = ?`, resource).Scan(&u.Resource, &u.State,
		&remaining, &u.Window, &resets, &u.Plan, &u.Source, &u.Detail, &at)
	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if remaining.Valid {
		v := remaining.Float64
		u.Remaining = &v
	}

	if resets.Valid {
		u.ResetsAt = atTime(resets.String)
	}

	u.At = atTime(at)

	return &u, nil
}
