package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

/*
 * A task is work that outlives the sentence that asked for it.
 *
 * Everything here is a column rather than a field in memory for one reason:
 * the program gets closed. A task that has been running for ten minutes, has
 * approved two actions and is parked waiting on a third has to be able to say
 * all of that again after a restart — including how much of its budget it has
 * already spent, which is the part that cannot be reconstructed and the part
 * that matters most if it is wrong.
 */

// Where a task has got to.
const (
	TaskPlanning = "planning" // being broken into steps
	TaskWorking  = "working"  // a step is running
	TaskWaiting  = "waiting"  // parked on somebody's decision
	TaskBlocked  = "blocked"  // stuck, and said why
	TaskDone     = "done"
	TaskStopped  = "stopped"
)

// Where one step has got to.
const (
	StepWaiting  = "waiting"
	StepRunning  = "running"
	StepNeedsYou = "needs_you"
	StepDone     = "done"
	StepFailed   = "failed"
	StepSkipped  = "skipped"
)

/*
 * What a check concluded, and how much it is worth.
 *
 * Verified and Claimed are kept apart deliberately. "I have written the file"
 * is not evidence that a file exists, and a record that cannot tell the two
 * apart is a record that flatters — which is the one thing a report on
 * unattended work must not do.
 */
const (
	// Verified: the outcome was found in something a tool actually returned.
	Verified = "verified"

	// Claimed: the only support is the assistant's own account of itself.
	Claimed = "claimed"

	Unmet = "unmet"
)

// What shape of work a step is. A closed set, because it chooses the model and
// narrows the tools, and because it is what a named agent will be routed by.
const (
	StepLook  = "look"
	StepDo    = "do"
	StepWrite = "write"
	StepCheck = "check"
)

type Task struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Goal     string `json:"goal"`
	DoneWhen string `json:"done_when"`
	State    string `json:"state"`

	ConversationID     int64  `json:"conversation_id"`
	WorkConversationID int64  `json:"work_conversation_id"`
	Provider           string `json:"provider"`
	JobID              int64  `json:"job_id,omitempty"`

	StepsLeft   int `json:"steps_left"`
	CallsLeft   int `json:"calls_left"`
	ReplansLeft int `json:"replans_left"`

	Deadline time.Time `json:"deadline,omitempty"`

	// Risk is the most serious of its steps. See the risk package.
	Risk string `json:"risk,omitempty"`

	Report    string    `json:"report,omitempty"`
	Because   string    `json:"blocked_because,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Finished  time.Time `json:"finished_at,omitempty"`

	// Steps is filled by TaskWithSteps, not by the listings.
	Steps []TaskStep `json:"steps,omitempty"`
}

type TaskStep struct {
	ID       int64 `json:"id"`
	TaskID   int64 `json:"task_id"`
	Position int   `json:"position"`

	Instruction string `json:"instruction"`
	DoneWhen    string `json:"done_when"`

	// Kind is what shape of work this is; see the constants above.
	Kind string `json:"kind"`

	// Changes is what the plan said this step would do. It is not the gate —
	// permits is — but a step that said it would change nothing and then
	// reached for a tool that does is worth stopping for.
	Changes bool `json:"changes"`

	/*
	 * Assignee is which agent this belongs to, and is empty for now.
	 *
	 * There is one agent today. Writing the column now rather than later is
	 * most of what makes several of them an addition instead of a migration, a
	 * planner change and an interface change arriving together.
	 */
	Assignee string `json:"assignee,omitempty"`

	// Provider and Model record what actually ran, not what was planned.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`

	State    string `json:"state"`
	Attempts int    `json:"attempts"`

	// Risk is how serious this step is, worked out when it was planned and
	// raised — never lowered — when it is known who is doing it.
	Risk string `json:"risk,omitempty"`

	// Acted is what ran at high or critical without being asked, one line
	// each. See migration 10.
	Acted string `json:"acted,omitempty"`

	Answer    string `json:"answer,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
	CheckedBy string `json:"checked_by,omitempty"`
	Verdict   string `json:"verdict,omitempty"`
	Why       string `json:"why,omitempty"`

	Started time.Time `json:"started_at,omitempty"`
	Ended   time.Time `json:"ended_at,omitempty"`
}

const taskColumns = `id, name, goal, done_when, state,
	COALESCE(conversation_id,0), COALESCE(work_conversation_id,0), provider,
	COALESCE(job_id,0), steps_left, calls_left, replans_left,
	COALESCE(deadline,''), COALESCE(report,''), COALESCE(blocked_because,''),
	created_at, updated_at, COALESCE(finished_at,''), COALESCE(risk,'')`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var (
		t                                 Task
		deadline, created, updated, ended string
		report, because                   string
	)

	err := row.Scan(&t.ID, &t.Name, &t.Goal, &t.DoneWhen, &t.State,
		&t.ConversationID, &t.WorkConversationID, &t.Provider,
		&t.JobID, &t.StepsLeft, &t.CallsLeft, &t.ReplansLeft,
		&deadline, &report, &because, &created, &updated, &ended, &t.Risk)
	if err != nil {
		return t, err
	}

	t.Report, t.Because = report, because
	t.Deadline = atTime(deadline)
	t.CreatedAt = atTime(created)
	t.UpdatedAt = atTime(updated)
	t.Finished = atTime(ended)

	return t, nil
}

func atTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	at, _ := time.Parse(time.RFC3339, s)

	return at
}

func asText(t time.Time) any {
	if t.IsZero() {
		return nil
	}

	return t.UTC().Format(time.RFC3339)
}

/*
 * NewTaskThread creates the conversation a task works in.
 *
 * Marked, so the four queries that list conversations leave it out. A task's
 * steps write their tool output into a real thread — that is how the agent
 * loop records anything — and without the mark, opening the program would
 * reopen the assistant's working notes instead of what somebody was saying,
 * and ten steps of tool output would be read back into the prompt of their
 * next question.
 */
func (d *DB) NewTaskThread(title string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(
		`INSERT INTO conversations (title, kind, created_at, updated_at) VALUES (?, 'task', ?, ?)`,
		title, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("starting the thread a task works in: %w", err)
	}

	return res.LastInsertId()
}

// NewTask records a task before any of it runs, so that what is about to
// happen can be read — and stopped — before it does.
func (d *DB) NewTask(t Task) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(`
		INSERT INTO tasks (name, goal, done_when, state, conversation_id,
			work_conversation_id, provider, steps_left, calls_left, replans_left,
			deadline, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.Name, t.Goal, t.DoneWhen, orElse(t.State, TaskPlanning),
		nullable(t.ConversationID), nullable(t.WorkConversationID), t.Provider,
		t.StepsLeft, t.CallsLeft, t.ReplansLeft, asText(t.Deadline), now, now)
	if err != nil {
		return 0, fmt.Errorf("recording the task: %w", err)
	}

	return res.LastInsertId()
}

func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func nullable(id int64) any {
	if id == 0 {
		return nil
	}

	return id
}

// Task reads one task without its steps.
func (d *DB) Task(id int64) (*Task, error) {
	t, err := scanTask(d.sql().QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &t, nil
}

// TaskWithSteps reads one task and everything planned for it.
func (d *DB) TaskWithSteps(id int64) (*Task, error) {
	t, err := d.Task(id)
	if err != nil || t == nil {
		return t, err
	}

	t.Steps, err = d.Steps(id)

	return t, err
}

// Tasks lists tasks in the given states, newest first. No states means all.
func (d *DB) Tasks(states []string, limit int) ([]Task, error) {
	query := `SELECT ` + taskColumns + ` FROM tasks`
	args := []any{}

	if len(states) > 0 {
		query += ` WHERE state IN (` + strings.TrimSuffix(strings.Repeat("?,", len(states)), ",") + `)`

		for _, s := range states {
			args = append(args, s)
		}
	}

	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.sql().Query(query, args...)
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

// LiveTasks is everything still going on, which is what the interface polls.
func (d *DB) LiveTasks() ([]Task, error) {
	return d.Tasks([]string{TaskPlanning, TaskWorking, TaskWaiting}, 20)
}

// SetTaskState moves a task, recording why when it stopped for a reason.
func (d *DB) SetTaskState(id int64, state, because string) error {
	_, err := d.sql().Exec(
		`UPDATE tasks SET state = ?, blocked_because = ?, updated_at = ? WHERE id = ?`,
		state, because, time.Now().UTC().Format(time.RFC3339), id)

	return err
}

/*
 * SpendOnTask takes model calls out of the budget and says what is left.
 *
 * One statement rather than a read, a subtraction and a write. A budget that
 * can be raced is not a budget, and the whole purpose of this number is to be
 * the thing that cannot be talked round.
 */
func (d *DB) SpendOnTask(id int64, calls int) (int, error) {
	var left int

	err := d.sql().QueryRow(`
		UPDATE tasks SET calls_left = MAX(calls_left - ?, 0), updated_at = ?
		WHERE id = ? RETURNING calls_left`,
		calls, time.Now().UTC().Format(time.RFC3339), id).Scan(&left)

	return left, err
}

// SpendAStep takes one step out of the plan's allowance.
func (d *DB) SpendAStep(id int64) (int, error) {
	var left int

	err := d.sql().QueryRow(`
		UPDATE tasks SET steps_left = MAX(steps_left - 1, 0), updated_at = ?
		WHERE id = ? RETURNING steps_left`,
		time.Now().UTC().Format(time.RFC3339), id).Scan(&left)

	return left, err
}

// SpendReplan uses up the one chance a task has to think again.
func (d *DB) SpendReplan(id int64) (int, error) {
	var left int

	err := d.sql().QueryRow(`
		UPDATE tasks SET replans_left = MAX(replans_left - 1, 0), updated_at = ?
		WHERE id = ? RETURNING replans_left`,
		time.Now().UTC().Format(time.RFC3339), id).Scan(&left)

	return left, err
}

/*
 * FinishTask closes a task and stores the account it wrote of itself.
 *
 * State, report and reason in one statement rather than two. Split across two,
 * anything polling could see the state go terminal and read the reason before
 * it had been written — so a task that stopped because it ran out of time
 * reported that it had stopped for no reason at all.
 */
func (d *DB) FinishTask(id int64, state, report, because string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.sql().Exec(`
		UPDATE tasks
		SET state = ?, report = ?, blocked_because = ?, updated_at = ?, finished_at = ?
		WHERE id = ?`,
		state, report, because, now, now, id)

	return err
}

/*
 * RaiseRisk makes a task and one of its steps at least this serious.
 *
 * Raise only, in SQL rather than by the caller comparing first: the order of
 * the four words is not alphabetical, and a comparison written in two places
 * is a comparison that will one day disagree with itself. A step can learn
 * it is more serious once it is known who is doing it; nothing learns that it
 * is less.
 */
func (d *DB) RaiseRisk(taskID, stepID int64, level string) error {
	const rank = `CASE %s WHEN 'critical' THEN 3 WHEN 'high' THEN 2 WHEN 'medium' THEN 1 ELSE 0 END`

	now := time.Now().UTC().Format(time.RFC3339)

	if stepID != 0 {
		if _, err := d.sql().Exec(fmt.Sprintf(`
			UPDATE task_steps SET risk = ?, updated_at = ?
			WHERE id = ? AND `+rank+` < `+rank, "risk", "?"),
			level, now, stepID, level); err != nil {
			return err
		}
	}

	_, err := d.sql().Exec(fmt.Sprintf(`
		UPDATE tasks SET risk = ?, updated_at = ?
		WHERE id = ? AND `+rank+` < `+rank, "COALESCE(risk,'')", "?"),
		level, now, taskID, level)

	return err
}

func nullText(s string) any {
	if s == "" {
		return nil
	}

	return s
}

// SetTaskJob records the background handle, so Stop in the interface reaches
// the goroutine actually doing the work.
func (d *DB) SetTaskJob(id, jobID int64) error {
	_, err := d.sql().Exec(`UPDATE tasks SET job_id = ? WHERE id = ?`, jobID, id)

	return err
}

// AddSteps writes a plan. Positions are assigned here so a caller cannot
// produce two steps that both claim to be next.
func (d *DB) AddSteps(taskID int64, steps []TaskStep) error {
	if len(steps) == 0 {
		return nil
	}

	tx, err := d.sql().Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	var highest int

	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), 0) FROM task_steps WHERE task_id = ?`, taskID,
	).Scan(&highest); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for i, s := range steps {
		_, err := tx.Exec(`
			INSERT INTO task_steps (task_id, position, instruction, done_when, kind,
				changes, assignee, risk, state, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			taskID, highest+i+1, s.Instruction, s.DoneWhen, orElse(s.Kind, StepDo),
			s.Changes, s.Assignee, orElse(s.Risk, "low"), StepWaiting, now, now)
		if err != nil {
			return fmt.Errorf("writing step %d: %w", i+1, err)
		}
	}

	return tx.Commit()
}

const stepColumns = `id, task_id, position, instruction, done_when, kind, changes,
	assignee, provider, model, state, attempts,
	COALESCE(answer,''), COALESCE(evidence,''), COALESCE(checked_by,''),
	COALESCE(verdict,''), COALESCE(why,''),
	COALESCE(started_at,''), COALESCE(ended_at,''), COALESCE(risk,''), COALESCE(acted,'')`

func scanStep(row interface{ Scan(...any) error }) (TaskStep, error) {
	var (
		s              TaskStep
		started, ended string
	)

	err := row.Scan(&s.ID, &s.TaskID, &s.Position, &s.Instruction, &s.DoneWhen,
		&s.Kind, &s.Changes, &s.Assignee, &s.Provider, &s.Model, &s.State,
		&s.Attempts, &s.Answer, &s.Evidence, &s.CheckedBy, &s.Verdict, &s.Why,
		&started, &ended, &s.Risk, &s.Acted)
	if err != nil {
		return s, err
	}

	s.Started, s.Ended = atTime(started), atTime(ended)

	return s, nil
}

// Steps lists a task's plan in the order it will be worked through.
func (d *DB) Steps(taskID int64) ([]TaskStep, error) {
	rows, err := d.sql().Query(
		`SELECT `+stepColumns+` FROM task_steps WHERE task_id = ? ORDER BY position`, taskID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out := []TaskStep{}

	for rows.Next() {
		s, err := scanStep(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, s)
	}

	return out, rows.Err()
}

// NextStep is the lowest-numbered step still to be done. Nil means the plan is
// finished, which is the only way a task ends well.
func (d *DB) NextStep(taskID int64) (*TaskStep, error) {
	s, err := scanStep(d.sql().QueryRow(
		`SELECT `+stepColumns+` FROM task_steps
		 WHERE task_id = ? AND state NOT IN (?,?,?)
		 ORDER BY position LIMIT 1`,
		taskID, StepDone, StepSkipped, StepFailed))

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &s, nil
}

/*
 * StartStep marks a step running and records who is about to do it.
 *
 * Assignee, provider and model are written with what actually ran, not with
 * what was planned. A record of intentions is no use at all when somebody is
 * looking at a step that went wrong and asking which of five agents, and which
 * of three models, produced it.
 */
func (d *DB) StartStep(id int64, assignee, provider, model string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.sql().Exec(`
		UPDATE task_steps
		SET state = ?, assignee = ?, provider = ?, model = ?, attempts = attempts + 1,
		    started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = ?`,
		StepRunning, assignee, provider, model, now, now, id)

	return err
}

// FinishStep writes down what happened, what was checked, and what the check
// was actually worth.
/*
 * SetAsideUnstartedSteps marks the steps nobody has begun as skipped.
 *
 * For a replan, which is a change of mind about the rest of the job. Only the
 * ones still waiting: a step that is running, parked on a decision or already
 * finished is not the caller's to withdraw, and one that failed is already
 * accounted for.
 *
 * The reason is written onto each of them, so the view can say why a step that
 * was going to happen did not — which is the difference between a plan that
 * changed and a plan that quietly lost a step.
 */
func (d *DB) SetAsideUnstartedSteps(taskID int64, because string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(`
		UPDATE task_steps
		SET state = ?, verdict = ?, why = ?, ended_at = ?, updated_at = ?
		WHERE task_id = ? AND state = ?`,
		StepSkipped, Unmet, because, now, now, taskID, StepWaiting)
	if err != nil {
		return 0, fmt.Errorf("setting aside the rest of the plan: %w", err)
	}

	put, err := res.RowsAffected()

	return int(put), err
}

func (d *DB) FinishStep(s TaskStep) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var ended any
	if s.State == StepDone || s.State == StepFailed || s.State == StepSkipped {
		ended = now
	}

	_, err := d.sql().Exec(`
		UPDATE task_steps
		SET state = ?, answer = ?, evidence = ?, checked_by = ?, verdict = ?,
		    why = ?, acted = ?, ended_at = ?, updated_at = ?
		WHERE id = ?`,
		s.State, s.Answer, s.Evidence, s.CheckedBy, s.Verdict, s.Why, nullText(s.Acted),
		ended, now, s.ID)

	return err
}

// SetStepState moves a step without touching what was recorded about it.
func (d *DB) SetStepState(id int64, state string) error {
	_, err := d.sql().Exec(
		`UPDATE task_steps SET state = ?, updated_at = ? WHERE id = ?`,
		state, time.Now().UTC().Format(time.RFC3339), id)

	return err
}

// LinkInvocationToStep records which step is parked on which decision, so an
// approval given twenty minutes later can find its way back to the work.
func (d *DB) LinkInvocationToStep(invocationID, taskID, stepID int64) error {
	_, err := d.sql().Exec(
		`UPDATE tool_invocations SET task_id = ?, step_id = ? WHERE id = ?`,
		taskID, stepID, invocationID)

	return err
}

// StepWaitingOn finds the step an approval belongs to. Nil means the decision
// was an ordinary one made in a conversation, with no task behind it.
func (d *DB) StepWaitingOn(invocationID int64) (*TaskStep, error) {
	var stepID int64

	err := d.sql().QueryRow(
		`SELECT COALESCE(step_id,0) FROM tool_invocations WHERE id = ?`, invocationID).Scan(&stepID)

	if err == sql.ErrNoRows || stepID == 0 {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	s, err := scanStep(d.sql().QueryRow(`SELECT `+stepColumns+` FROM task_steps WHERE id = ?`, stepID))

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &s, nil
}

// TaskPendingCount is how many decisions a task is still waiting on. A step
// that asked for two things must not carry on when only one is answered.
func (d *DB) TaskPendingCount(taskID int64) (int, error) {
	var n int

	err := d.sql().QueryRow(
		`SELECT COUNT(*) FROM tool_invocations WHERE task_id = ? AND status = ?`,
		taskID, InvocationPending).Scan(&n)

	return n, err
}

// InterruptWorkingTasks is called at startup. A task that was running when the
// program closed is moved to waiting rather than resumed: something that was
// halfway through changing files should not carry on the moment somebody opens
// their assistant, without them having said so.
func (d *DB) InterruptWorkingTasks(because string) (int, error) {
	res, err := d.sql().Exec(`
		UPDATE tasks SET state = ?, blocked_because = ?, updated_at = ?
		WHERE state IN (?,?)`,
		TaskWaiting, because, time.Now().UTC().Format(time.RFC3339),
		TaskWorking, TaskPlanning)
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()

	return int(n), err
}
