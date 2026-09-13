package store

import (
	"fmt"
	"strings"
	"time"
)

/*
 * Work handed from one agent to another.
 *
 * Everything here is a row for the same reason every task is: the program
 * gets closed. A step handed to a specialist twenty minutes ago has to still
 * be handed on after a restart, with the specialist's task still knowing whose
 * step it is and how much of whose budget it is spending.
 */

// names reads a list written as commas, dropping the blanks.
func names(written string) []string {
	out := []string{}

	for _, one := range strings.Split(written, ",") {
		if one = strings.TrimSpace(one); one != "" {
			out = append(out, one)
		}
	}

	return out
}

// withinText writes a tool limit so that no limit and no tools stay two
// different answers: NULL for the first, an empty string for the second.
func withinText(within *[]string) any {
	if within == nil {
		return nil
	}

	return strings.Join(*within, ",")
}

/*
 * HandOn marks a step as being done by a child task.
 *
 * Only a step that is running can be handed on, checked in the statement: a
 * step that finished, or was already handed to somebody, is not the caller's
 * to give away again, and two handings-on of one step would be two
 * specialists doing the same work on one budget.
 */
func (d *DB) HandOn(stepID, childID int64, why string) error {
	res, err := d.sql().Exec(`
		UPDATE task_steps SET state = ?, handed_to = ?, why = ?, updated_at = ?
		WHERE id = ? AND state IN (?, ?)`,
		StepHandedOn, childID, why, time.Now().UTC().Format(time.RFC3339),
		stepID, StepRunning, StepWaiting)
	if err != nil {
		return fmt.Errorf("handing the step on: %w", err)
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("that step is not one that can be handed on")
	}

	return nil
}

// Children is every task handed on from this one's steps, oldest first.
func (d *DB) Children(taskID int64) ([]Task, error) {
	rows, err := d.sql().Query(
		`SELECT `+taskColumns+` FROM tasks WHERE parent_task_id = ? ORDER BY id`, taskID)
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

/*
 * ShareBudget takes a child's allowance out of its parent's, and says whether
 * there was enough.
 *
 * In one statement, and only when the parent can afford it — a delegation
 * that could spend budget its parent did not have would be a way of minting
 * more by handing work around.
 */
func (d *DB) ShareBudget(parentID int64, calls, steps int) (bool, error) {
	res, err := d.sql().Exec(`
		UPDATE tasks SET calls_left = calls_left - ?, steps_left = steps_left - ?, updated_at = ?
		WHERE id = ? AND calls_left > ? AND steps_left > ?`,
		calls, steps, time.Now().UTC().Format(time.RFC3339), parentID, calls, steps)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n == 1, err
}

// Refund gives back what a child did not spend, so handing work on costs the
// parent what the work cost and not what it was allowed.
func (d *DB) Refund(parentID int64, calls, steps int) error {
	if calls <= 0 && steps <= 0 {
		return nil
	}

	_, err := d.sql().Exec(`
		UPDATE tasks SET calls_left = calls_left + MAX(?, 0), steps_left = steps_left + MAX(?, 0),
		       updated_at = ?
		WHERE id = ?`,
		calls, steps, time.Now().UTC().Format(time.RFC3339), parentID)

	return err
}

/*
 * InsertStepAfter puts a step straight after another, rather than at the end.
 *
 * For a manager's answer to a step that failed, and for a review of one that
 * finished: both are about that step and must happen before the plan moves on
 * past it. Appending would put them behind everything still waiting, which is
 * the right step at the wrong time.
 */
func (d *DB) InsertStepAfter(taskID int64, position int, s TaskStep) (int64, error) {
	tx, err := d.sql().Begin()
	if err != nil {
		return 0, err
	}

	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	/*
	 * Moved out of the way in two passes, because position is unique within a
	 * task and SQLite checks that row by row: adding one to every later step
	 * in a single statement collides with the next step before that one has
	 * moved. Negative first, then back.
	 */
	if _, err := tx.Exec(`UPDATE task_steps SET position = -(position + 1)
		WHERE task_id = ? AND position > ?`, taskID, position); err != nil {
		return 0, fmt.Errorf("making room for a step: %w", err)
	}

	if _, err := tx.Exec(`UPDATE task_steps SET position = -position
		WHERE task_id = ? AND position < 0`, taskID); err != nil {
		return 0, fmt.Errorf("making room for a step: %w", err)
	}

	res, err := tx.Exec(`
		INSERT INTO task_steps (task_id, position, instruction, done_when, kind,
			changes, assignee, risk, state, created_at, updated_at, escalated_from, review_of)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		taskID, position+1, s.Instruction, s.DoneWhen, orElse(s.Kind, StepDo),
		s.Changes, s.Assignee, orElse(s.Risk, "low"), StepWaiting, now, now,
		nullable(s.EscalatedFrom), nullable(s.ReviewOf))
	if err != nil {
		return 0, fmt.Errorf("writing the step: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, tx.Commit()
}

// Step reads one step.
func (d *DB) Step(id int64) (*TaskStep, error) {
	s, err := scanStep(d.sql().QueryRow(`SELECT `+stepColumns+` FROM task_steps WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}

	return &s, nil
}
