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

	// The conversation is stored rather than discarded. It has been a
	// parameter since this was written and went nowhere, so an approval could
	// never be traced back to what was being discussed when it was proposed.
	res, err := d.sql().Exec(
		`INSERT INTO tool_invocations
		   (tool, arguments, summary, risk, status, conversation_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tool, arguments, summary, risk, InvocationPending, conversationID, now, now,
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

/*
 * Habit is what somebody has actually decided about one tool.
 *
 * Every approval and every refusal has been written down since the gate was
 * built, and nothing has ever read any of it back. That record is the only
 * honest answer to "what does this person want me to stop asking about" — and
 * without it the gate cannot get less annoying, so it gets clicked through
 * instead, at which point it looks like protection and is not.
 */
type Habit struct {
	Tool     string `json:"tool"`
	Approved int    `json:"approved"`
	Refused  int    `json:"refused"`

	// Last is the most recent decision, so a habit somebody has since changed
	// their mind about can be told from one they are still in.
	Last time.Time `json:"last"`
}

// Settled reports whether this is a habit rather than a coincidence: enough
// decisions, all of them the same way.
func (h Habit) Settled(enough int) bool {
	if h.Approved+h.Refused < enough {
		return false
	}

	return h.Approved == 0 || h.Refused == 0
}

// Habits is what has been decided about each tool, most-decided first.
func (d *DB) Habits() ([]Habit, error) {
	rows, err := d.sql().Query(`
		SELECT tool,
		       COUNT(*) FILTER (WHERE status IN (?, ?)) AS approved,
		       COUNT(*) FILTER (WHERE status = ?)       AS refused,
		       MAX(COALESCE(decided_at, updated_at))    AS last
		FROM tool_invocations
		WHERE status <> ?
		GROUP BY tool
		ORDER BY approved + refused DESC, tool`,
		InvocationApproved, InvocationDone, InvocationDenied, InvocationPending)
	if err != nil {
		return nil, fmt.Errorf("reading what has been decided: %w", err)
	}

	defer rows.Close()

	out := []Habit{}

	for rows.Next() {
		var (
			h    Habit
			last string
		)

		if err := rows.Scan(&h.Tool, &h.Approved, &h.Refused, &last); err != nil {
			return nil, err
		}

		h.Last = atTime(last)

		out = append(out, h)
	}

	return out, rows.Err()
}
