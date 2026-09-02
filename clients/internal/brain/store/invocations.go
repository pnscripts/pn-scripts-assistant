package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Invocation statuses.
const (
	InvocationPending  = "pending"
	InvocationApproved = "approved"
	InvocationDenied   = "denied"
	InvocationDone     = "done"
	InvocationFailed   = "failed"
)

// Invocation is one tool call that needed a person's decision.
type Invocation struct {
	ID        int64     `json:"id"`
	Tool      string    `json:"tool"`
	Arguments string    `json:"arguments"`
	Summary   string    `json:"summary"`
	Risk      string    `json:"risk"`
	Status    string    `json:"status"`
	Result    string    `json:"result,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RecordInvocation stores a proposed action awaiting approval.
//
// The arguments are stored verbatim alongside the summary a person was shown.
// Keeping both is what makes an approval auditable: without the arguments the
// action cannot be replayed, and without the summary there is no record of what
// the person actually agreed to.
func (d *DB) RecordInvocation(conversationID int64, tool, arguments, summary, risk string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(
		`INSERT INTO tool_invocations (tool, arguments, summary, risk, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tool, arguments, summary, risk, InvocationPending, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("recording tool call: %w", err)
	}

	return res.LastInsertId()
}

// PendingInvocations lists what is waiting for a decision.
func (d *DB) PendingInvocations() ([]Invocation, error) {
	rows, err := d.sql().Query(`
		SELECT id, tool, arguments, COALESCE(summary,''), risk, status, created_at
		FROM tool_invocations WHERE status = ? ORDER BY id`, InvocationPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Invocation{}

	for rows.Next() {
		var i Invocation
		var created string

		if err := rows.Scan(&i.ID, &i.Tool, &i.Arguments, &i.Summary, &i.Risk, &i.Status, &created); err != nil {
			return nil, err
		}

		i.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, i)
	}

	return out, rows.Err()
}

// Invocation loads one by id.
func (d *DB) Invocation(id int64) (*Invocation, error) {
	var i Invocation
	var created string
	var result sql.NullString

	err := d.sql().QueryRow(`
		SELECT id, tool, arguments, COALESCE(summary,''), risk, status, result, created_at
		FROM tool_invocations WHERE id = ?`, id).
		Scan(&i.ID, &i.Tool, &i.Arguments, &i.Summary, &i.Risk, &i.Status, &result, &created)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	i.Result = result.String
	i.CreatedAt, _ = time.Parse(time.RFC3339, created)

	return &i, nil
}

// DecideInvocation records what a person chose, and only from pending.
//
// The WHERE clause on status is the guard: an action already decided cannot be
// decided again, so a replayed request cannot turn a denial into an approval.
// It returns whether the transition actually happened.
func (d *DB) DecideInvocation(id int64, status string) (bool, error) {
	res, err := d.sql().Exec(
		`UPDATE tool_invocations SET status = ?, decided_at = ?, updated_at = ?
		 WHERE id = ? AND status = ?`,
		status, time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339), id, InvocationPending,
	)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// CompleteInvocation records the outcome of an action that was carried out.
func (d *DB) CompleteInvocation(id int64, status, result string) error {
	_, err := d.sql().Exec(
		`UPDATE tool_invocations SET status = ?, result = ?, updated_at = ? WHERE id = ?`,
		status, result, time.Now().UTC().Format(time.RFC3339), id,
	)

	return err
}
