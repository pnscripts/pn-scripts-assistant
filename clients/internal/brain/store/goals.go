package store

import (
	"database/sql"
	"fmt"
	"time"
)

/*
 * A goal is what the work is for.
 *
 * A task finishes; a goal comes round again. "Go through my projects and tell
 * me what is broken" is a task. "Keep my projects building" is a goal, and the
 * useful question about it is never whether it is done — it is when it was
 * last looked at, what happened that time, and whether it is due again.
 */

const (
	GoalActive = "active"
	GoalPaused = "paused"
	GoalDone   = "done"
)

type Goal struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Why   string `json:"why"`
	State string `json:"state"`

	// EveryDays is how often it is worth coming back to. Zero means never on
	// its own: it is a standing intention somebody works on when they choose.
	EveryDays int `json:"every_days"`

	/*
	 * StartsItself is off by default, and kept separate from EveryDays on
	 * purpose.
	 *
	 * A goal that says it is due and a goal that begins work while nobody is
	 * at the machine are different things, and the second should never become
	 * the first by accident. Everything a task does still passes the gate
	 * either way — this decides only whether the work starts without being
	 * asked, not what it may do once it has.
	 */
	StartsItself bool `json:"starts_itself"`

	LastWorked time.Time `json:"last_worked,omitempty"`
	NextDue    time.Time `json:"next_due,omitempty"`
	CreatedAt  time.Time `json:"created_at"`

	// Tasks is filled by the listing: how much has been done towards this, and
	// how it went. A goal with no history is a note somebody wrote.
	Tasks []Task `json:"tasks,omitempty"`

	// Due is worked out when read rather than stored, so a goal does not
	// become due only when something happens to look at it.
	Due bool `json:"due"`
}

const goalColumns = `id, name, why, state, every_days, starts_itself,
	COALESCE(last_worked,''), COALESCE(next_due,''), created_at`

func scanGoal(row interface{ Scan(...any) error }, now time.Time) (Goal, error) {
	var (
		g                    Goal
		worked, due, created string
	)

	err := row.Scan(&g.ID, &g.Name, &g.Why, &g.State, &g.EveryDays,
		&g.StartsItself, &worked, &due, &created)
	if err != nil {
		return g, err
	}

	g.LastWorked = atTime(worked)
	g.NextDue = atTime(due)
	g.CreatedAt = atTime(created)
	g.Due = g.State == GoalActive && !g.NextDue.IsZero() && !now.Before(g.NextDue)

	return g, nil
}

// NewGoal records a standing intention.
func (d *DB) NewGoal(g Goal) (int64, error) {
	now := time.Now().UTC()

	// Due the moment it is made when it has a rhythm at all. Somebody who has
	// just said they want this looked at weekly means starting now, not in a
	// week.
	next := any(nil)

	if g.EveryDays > 0 {
		next = now.Format(time.RFC3339)
	}

	res, err := d.sql().Exec(`
		INSERT INTO goals (name, why, state, every_days, starts_itself, next_due,
			created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		g.Name, g.Why, orElse(g.State, GoalActive), g.EveryDays, g.StartsItself,
		next, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("recording the goal: %w", err)
	}

	return res.LastInsertId()
}

// Goal reads one, without its history.
func (d *DB) Goal(id int64) (*Goal, error) {
	g, err := scanGoal(d.sql().QueryRow(
		`SELECT `+goalColumns+` FROM goals WHERE id = ?`, id), time.Now())

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &g, nil
}

// Goals lists them, newest first, with what has been done towards each.
func (d *DB) Goals() ([]Goal, error) {
	rows, err := d.sql().Query(
		`SELECT ` + goalColumns + ` FROM goals ORDER BY state, id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	now := time.Now()
	out := []Goal{}

	for rows.Next() {
		g, err := scanGoal(rows, now)
		if err != nil {
			return nil, err
		}

		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		out[i].Tasks, _ = d.TasksForGoal(out[i].ID, 5)
	}

	return out, nil
}

// DueGoals is what is ready to be worked on, soonest first.
func (d *DB) DueGoals(now time.Time) ([]Goal, error) {
	rows, err := d.sql().Query(`
		SELECT `+goalColumns+` FROM goals
		WHERE state = ? AND next_due IS NOT NULL AND next_due <= ?
		ORDER BY next_due`,
		GoalActive, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Goal{}

	for rows.Next() {
		g, err := scanGoal(rows, now)
		if err != nil {
			return nil, err
		}

		out = append(out, g)
	}

	return out, rows.Err()
}

// TasksForGoal is what has been done towards one, most recent first.
func (d *DB) TasksForGoal(goalID int64, limit int) ([]Task, error) {
	rows, err := d.sql().Query(
		`SELECT `+taskColumns+` FROM tasks WHERE goal_id = ? ORDER BY id DESC LIMIT ?`,
		goalID, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []Task{}

	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, t)
	}

	return out, rows.Err()
}

// SetTaskGoal records which standing intention a piece of work belongs to.
func (d *DB) SetTaskGoal(taskID, goalID int64) error {
	_, err := d.sql().Exec(`UPDATE tasks SET goal_id = ? WHERE id = ?`, goalID, taskID)

	return err
}

// UpdateGoal changes the parts somebody can edit.
func (d *DB) UpdateGoal(g Goal) error {
	now := time.Now().UTC().Format(time.RFC3339)

	/*
	 * A rhythm added later starts now; one removed clears the date.
	 *
	 * Keeping a stale next_due on a goal with no rhythm would leave it
	 * permanently and invisibly due — the row would say one thing and the
	 * interface another.
	 */
	var next any

	if g.EveryDays > 0 {
		if g.NextDue.IsZero() {
			next = now
		} else {
			next = g.NextDue.UTC().Format(time.RFC3339)
		}
	}

	_, err := d.sql().Exec(`
		UPDATE goals
		SET name = ?, why = ?, state = ?, every_days = ?, starts_itself = ?,
		    next_due = ?, updated_at = ?
		WHERE id = ?`,
		g.Name, g.Why, orElse(g.State, GoalActive), g.EveryDays, g.StartsItself,
		next, now, g.ID)

	return err
}

/*
 * GoalWorkedOn records that something was done towards it and when it comes
 * round again.
 *
 * Counted from now rather than from when it was due, so a goal nobody looked
 * at for a month does not arrive with four overdue turns queued behind it.
 */
func (d *DB) GoalWorkedOn(id int64, now time.Time) error {
	g, err := d.Goal(id)
	if err != nil || g == nil {
		return err
	}

	var next any

	if g.EveryDays > 0 {
		next = now.UTC().AddDate(0, 0, g.EveryDays).Format(time.RFC3339)
	}

	_, err = d.sql().Exec(
		`UPDATE goals SET last_worked = ?, next_due = ?, updated_at = ? WHERE id = ?`,
		now.UTC().Format(time.RFC3339), next, now.UTC().Format(time.RFC3339), id)

	return err
}

// ForgetGoal removes one. The tasks done towards it stay: they happened, and a
// record that disappears when somebody tidies up is not a record.
func (d *DB) ForgetGoal(id int64) error {
	_, err := d.sql().Exec(`DELETE FROM goals WHERE id = ?`, id)

	return err
}
