package store

import (
	"path/filepath"
	"testing"
	"time"
)

// A task's own thread is a full record of what it did, and belongs in the
// Tasks view. If it leaks into the conversation lists, opening the program
// reopens the assistant's working notes instead of what somebody was saying.
func TestATasksWorkspaceIsNotAConversation(t *testing.T) {
	db := open(t)

	mine, _ := db.NewConversation("what I asked")
	if _, err := db.AddMessage(mine, "user", "", "", "go through my projects"); err != nil {
		t.Fatal(err)
	}

	workspace, _ := db.NewConversation("Going through your projects")

	if _, err := db.sql().Exec(`UPDATE conversations SET kind = 'task' WHERE id = ?`, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(workspace, "user", "", "", "step one: list the folders"); err != nil {
		t.Fatal(err)
	}

	recent, err := db.RecentConversations(10)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range recent {
		if r.ID == workspace {
			t.Error("a task's workspace was listed as a conversation somebody had")
		}
	}

	latest, err := db.LatestConversation()
	if err != nil {
		t.Fatal(err)
	}

	if latest == nil || latest.ID != mine {
		t.Errorf("reopening would land in the wrong thread: %+v", latest)
	}

	n, err := db.CountConversations()
	if err != nil {
		t.Fatal(err)
	}

	if n != 1 {
		t.Errorf("counted %d conversations, want 1", n)
	}
}

// And a task killed halfway through must not have "that turn was cut short"
// written into its working notes.
func TestATasksWorkspaceIsNotSweptForAbandonedTurns(t *testing.T) {
	db := open(t)

	workspace, _ := db.NewConversation("working")

	if _, err := db.sql().Exec(`UPDATE conversations SET kind = 'task' WHERE id = ?`, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := db.AddMessage(workspace, "user", "", "", "step three: read the file"); err != nil {
		t.Fatal(err)
	}

	n, err := db.FinishAbandonedTurns()
	if err != nil {
		t.Fatal(err)
	}

	if n != 0 {
		t.Errorf("swept %d task threads", n)
	}
}

func aTask(t *testing.T, db *DB) int64 {
	t.Helper()

	id, err := db.NewTask(Task{
		Name: "Going through your projects", Goal: "find what is broken",
		StepsLeft: 12, CallsLeft: 40, ReplansLeft: 1,
		Deadline: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	return id
}

// A task can say where it had got to after the program was closed, which is
// the whole reason any of this is a table rather than a field.
func TestATaskSurvivesBeingReadBack(t *testing.T) {
	db := open(t)
	id := aTask(t, db)

	got, err := db.Task(id)
	if err != nil || got == nil {
		t.Fatalf("reading the task back: %v %v", got, err)
	}

	if got.Name != "Going through your projects" || got.State != TaskPlanning {
		t.Errorf("came back as %+v", got)
	}

	if got.CallsLeft != 40 || got.StepsLeft != 12 || got.ReplansLeft != 1 {
		t.Errorf("the budget did not survive: %+v", got)
	}

	if got.Deadline.IsZero() {
		t.Error("the deadline did not survive, so the task would run forever")
	}
}

/*
 * The budget only goes down, and never below nothing.
 *
 * A budget held in memory refills on restart. This one is a column and is
 * spent in one statement, so two things spending at once cannot both read
 * forty and both write thirty-nine.
 */
func TestTheBudgetOnlyGoesDown(t *testing.T) {
	db := open(t)
	id := aTask(t, db)

	left, err := db.SpendOnTask(id, 3)
	if err != nil {
		t.Fatal(err)
	}

	if left != 37 {
		t.Errorf("spending 3 of 40 left %d", left)
	}

	if left, _ = db.SpendOnTask(id, 100); left != 0 {
		t.Errorf("overspending left %d, want 0", left)
	}

	got, _ := db.Task(id)

	if got.CallsLeft != 0 {
		t.Errorf("the row says %d", got.CallsLeft)
	}
}

// The plan is worked through in order, and a step that is done, skipped or
// failed is not offered again.
func TestStepsAreWorkedThroughInOrder(t *testing.T) {
	db := open(t)
	id := aTask(t, db)

	err := db.AddSteps(id, []TaskStep{
		{Instruction: "list the folders", DoneWhen: "a list of folders", Kind: StepLook},
		{Instruction: "read each project file", Kind: StepLook},
		{Instruction: "write the summary", Kind: StepWrite, Changes: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(id)

	if len(steps) != 3 || steps[0].Position != 1 || steps[2].Position != 3 {
		t.Fatalf("the plan came back as %+v", steps)
	}

	if !steps[2].Changes || steps[2].Kind != StepWrite {
		t.Errorf("what the plan said about the last step was lost: %+v", steps[2])
	}

	next, _ := db.NextStep(id)

	if next == nil || next.Instruction != "list the folders" {
		t.Fatalf("next step is %+v", next)
	}

	next.State = StepDone
	next.Verdict = Verified
	next.Evidence = "Documents  Games  Projects"

	if err := db.FinishStep(*next); err != nil {
		t.Fatal(err)
	}

	after, _ := db.NextStep(id)

	if after == nil || after.Position != 2 {
		t.Fatalf("after finishing step one, next is %+v", after)
	}

	done, _ := db.Steps(id)

	if done[0].Verdict != Verified || done[0].Evidence == "" {
		t.Errorf("the evidence was not kept: %+v", done[0])
	}

	if done[0].Ended.IsZero() {
		t.Error("a finished step has no end time")
	}
}

/*
 * An approval given twenty minutes later finds its way back to the work.
 *
 * A conversation that hits the gate simply ends and its owner asks again. A
 * task has a quarter of an hour behind it and nobody to ask, so the link from
 * the decision to the step is the only route home.
 */
func TestAnApprovalFindsTheStepItBelongsTo(t *testing.T) {
	db := open(t)
	id := aTask(t, db)

	conv, _ := db.NewConversation("t")

	if err := db.AddSteps(id, []TaskStep{{Instruction: "write the summary"}}); err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(id)

	invocation, err := db.RecordInvocation(conv, "write_file", `{"path":"/tmp/x"}`, "Write /tmp/x", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	// The conversation is stored now too, so an approval can be traced back to
	// what was being discussed when it was proposed.
	var storedConv int64

	if err := db.sql().QueryRow(
		`SELECT COALESCE(conversation_id,0) FROM tool_invocations WHERE id = ?`, invocation,
	).Scan(&storedConv); err != nil {
		t.Fatal(err)
	}

	if storedConv != conv {
		t.Errorf("the invocation stored conversation %d, want %d", storedConv, conv)
	}

	if err := db.LinkInvocationToStep(invocation, id, steps[0].ID); err != nil {
		t.Fatal(err)
	}

	found, err := db.StepWaitingOn(invocation)
	if err != nil {
		t.Fatal(err)
	}

	if found == nil || found.ID != steps[0].ID {
		t.Fatalf("the approval did not lead back to its step: %+v", found)
	}

	waiting, _ := db.TaskPendingCount(id)

	if waiting != 1 {
		t.Errorf("the task is waiting on %d decisions, want 1", waiting)
	}

	// And an ordinary approval, with no task behind it, leads nowhere.
	plain, _ := db.RecordInvocation(conv, "write_file", "{}", "Write something", "mutating")

	if step, _ := db.StepWaitingOn(plain); step != nil {
		t.Errorf("an ordinary approval was taken for task work: %+v", step)
	}
}

// A task that was running when the program closed is parked, not resumed.
// Something halfway through changing files should not carry on the moment
// somebody opens their assistant.
func TestAnInterruptedTaskIsParkedRatherThanResumed(t *testing.T) {
	db := open(t)
	id := aTask(t, db)

	if err := db.SetTaskState(id, TaskWorking, ""); err != nil {
		t.Fatal(err)
	}

	n, err := db.InterruptWorkingTasks("PN Scripts Assistant was closed while this was running")
	if err != nil {
		t.Fatal(err)
	}

	if n != 1 {
		t.Fatalf("parked %d tasks", n)
	}

	got, _ := db.Task(id)

	if got.State != TaskWaiting || got.Because == "" {
		t.Errorf("came back as %q because %q", got.State, got.Because)
	}
}

/*
 * A brain that already has things in it is upgraded without losing them.
 *
 * The tests above all build a database from nothing, which is the one case
 * that cannot go wrong. What matters is the other one: somebody's real brain,
 * months of conversations and approvals in it, opened by a version that wants
 * two new tables and four new columns. ALTER TABLE ADD COLUMN is the only form
 * SQLite will do without rewriting the table, and it refuses that form with a
 * NOT NULL default — so this is the test that would have caught it.
 */
func TestAnOlderBrainIsUpgradedWithoutLosingAnything(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.sqlite")

	whole := migrations

	t.Cleanup(func() { migrations = whole })

	// Version 3: the shape a brain in use today has.
	migrations = whole[:3]

	old, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	conv, err := old.NewConversation("months of this")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := old.AddMessage(conv, "user", "", "", "what did we decide about the invoices"); err != nil {
		t.Fatal(err)
	}

	// Written the way version 3 wrote it — the new code would reach for a
	// column that does not exist yet, which is the whole point of the upgrade.
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := old.sql().Exec(`
		INSERT INTO tool_invocations (tool, arguments, summary, risk, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`,
		"send_email", `{"to":"a@b.c"}`, "Send it", "mutating", InvocationPending, now, now)
	if err != nil {
		t.Fatal(err)
	}

	invocation, _ := res.LastInsertId()

	old.Close()

	migrations = whole

	db, err := Open(path)
	if err != nil {
		t.Fatalf("upgrading a brain that already had things in it: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	var version int

	if err := db.sql().QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}

	if version != len(whole) {
		t.Errorf("schema version is %d, want %d", version, len(whole))
	}

	// Everything that was there is still there.
	history, err := db.History(conv)
	if err != nil || len(history) != 1 {
		t.Fatalf("the conversation did not survive: %d messages, %v", len(history), err)
	}

	pending, err := db.PendingInvocations()
	if err != nil || len(pending) != 1 || pending[0].ID != invocation {
		t.Fatalf("the waiting approval did not survive: %+v %v", pending, err)
	}

	// And the new tables work on the upgraded file, not only on a fresh one.
	id, err := db.NewTask(Task{Name: "a task", Goal: "something", CallsLeft: 5})
	if err != nil {
		t.Fatalf("tasks are unusable after an upgrade: %v", err)
	}

	if err := db.LinkInvocationToStep(invocation, id, 0); err != nil {
		t.Errorf("the new invocation columns are missing: %v", err)
	}
}

/*
 * A replan puts the rest of the old plan aside, and only that.
 *
 * Without it a replan was not a replan: the steps nobody had started stayed
 * waiting, so a task worked through the plan that had just got stuck and only
 * then reached the new one. What is running, parked or already finished is not
 * the caller's to withdraw.
 */
func TestOnlyTheStepsNobodyStartedArePutAside(t *testing.T) {
	db := open(t)

	conv, _ := db.NewConversation("t")

	id, err := db.NewTask(Task{
		Name: "A job", Goal: "do the thing", ConversationID: conv,
		WorkConversationID: conv, StepsLeft: 12, CallsLeft: 40, ReplansLeft: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = db.AddSteps(id, []TaskStep{
		{Instruction: "one", Kind: StepLook},
		{Instruction: "two", Kind: StepDo},
		{Instruction: "three", Kind: StepWrite},
		{Instruction: "four", Kind: StepDo},
	})
	if err != nil {
		t.Fatal(err)
	}

	steps, err := db.Steps(id)
	if err != nil || len(steps) != 4 {
		t.Fatalf("%d steps: %v", len(steps), err)
	}

	// One finished, one failed, one still running, one never begun.
	steps[0].State, steps[0].Verdict = StepDone, Verified
	steps[1].State, steps[1].Verdict = StepFailed, Unmet

	for _, s := range steps[:2] {
		if err := db.FinishStep(s); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.SetStepState(steps[2].ID, StepRunning); err != nil {
		t.Fatal(err)
	}

	put, err := db.SetAsideUnstartedSteps(id, "planned again")
	if err != nil {
		t.Fatal(err)
	}

	if put != 1 {
		t.Errorf("%d steps were put aside, want the one nobody had started", put)
	}

	after, err := db.Steps(id)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{StepDone, StepFailed, StepRunning, StepSkipped}

	for i, s := range after {
		if s.State != want[i] {
			t.Errorf("step %d is %q, want %q", i+1, s.State, want[i])
		}
	}

	if after[3].Why == "" {
		t.Error("a step that was put aside does not say why")
	}

	// And the next step to run is the one that is running, not the one that
	// was withdrawn.
	next, err := db.NextStep(id)
	if err != nil {
		t.Fatal(err)
	}

	if next == nil || next.Instruction != "three" {
		t.Errorf("the next step is %+v", next)
	}
}
