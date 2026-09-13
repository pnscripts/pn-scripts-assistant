package store

import (
	"testing"
	"time"
)

/*
 * A goal with a rhythm is due the moment it is set.
 *
 * Somebody who has just said they want this looked at weekly means starting
 * now, not in a week. The other reading makes the first week of every goal the
 * one where nothing happens, which is exactly the week they were thinking
 * about it.
 */
func TestAGoalWithARhythmIsDueAtOnce(t *testing.T) {
	db := open(t)

	id, err := db.NewGoal(Goal{Name: "Keep my projects building", EveryDays: 7})
	if err != nil {
		t.Fatal(err)
	}

	goal, err := db.Goal(id)
	if err != nil || goal == nil {
		t.Fatalf("reading it back: %v %v", goal, err)
	}

	if !goal.Due {
		t.Error("a goal set to be looked at weekly is not due yet")
	}

	due, err := db.DueGoals(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if len(due) != 1 {
		t.Fatalf("%d goals are due", len(due))
	}
}

// A goal with no rhythm is never due on its own: it is a standing intention
// somebody works on when they choose.
func TestAGoalWithNoRhythmIsNeverDueOnItsOwn(t *testing.T) {
	db := open(t)

	id, _ := db.NewGoal(Goal{Name: "Get better at Godot"})

	goal, _ := db.Goal(id)

	if goal.Due {
		t.Error("a goal with no schedule reported itself due")
	}

	due, _ := db.DueGoals(time.Now().AddDate(1, 0, 0))

	if len(due) != 0 {
		t.Errorf("%d goals came due a year later with no schedule", len(due))
	}
}

/*
 * Worked on, so it comes round from now rather than from when it was due.
 *
 * Counted from when it was due, a goal nobody looked at for a month would
 * arrive with four overdue turns queued behind it and start four pieces of
 * work in a row.
 */
func TestComingRoundIsCountedFromWhenItWasDone(t *testing.T) {
	db := open(t)

	id, _ := db.NewGoal(Goal{Name: "Weekly tidy", EveryDays: 7})

	// A month late.
	now := time.Now()

	if err := db.GoalWorkedOn(id, now); err != nil {
		t.Fatal(err)
	}

	goal, _ := db.Goal(id)

	if goal.Due {
		t.Error("it is still due immediately after being worked on")
	}

	days := goal.NextDue.Sub(now).Hours() / 24

	if days < 6.5 || days > 7.5 {
		t.Errorf("it comes round in %.1f days, want about 7", days)
	}

	if goal.LastWorked.IsZero() {
		t.Error("nothing recorded that it was worked on")
	}
}

// Starting itself is its own decision, not something inherited from having a
// schedule.
func TestStartingItselfIsASeparateDecision(t *testing.T) {
	db := open(t)

	id, _ := db.NewGoal(Goal{Name: "Weekly tidy", EveryDays: 7})

	goal, _ := db.Goal(id)

	if goal.StartsItself {
		t.Error("a goal with a schedule started itself without being told to")
	}

	goal.StartsItself = true

	if err := db.UpdateGoal(*goal); err != nil {
		t.Fatal(err)
	}

	again, _ := db.Goal(id)

	if !again.StartsItself {
		t.Error("being told to start itself did not stick")
	}
}

// Taking the rhythm away clears the date, or the goal would be permanently
// and invisibly due — the row saying one thing and the interface another.
func TestRemovingTheRhythmClearsTheDate(t *testing.T) {
	db := open(t)

	id, _ := db.NewGoal(Goal{Name: "Weekly tidy", EveryDays: 7})

	goal, _ := db.Goal(id)
	goal.EveryDays = 0

	if err := db.UpdateGoal(*goal); err != nil {
		t.Fatal(err)
	}

	after, _ := db.Goal(id)

	if after.Due || !after.NextDue.IsZero() {
		t.Errorf("it is still due: %+v", after)
	}
}

// What was done towards a goal outlives the goal. It happened, and a record
// that disappears when somebody tidies up is not a record.
func TestWhatWasDoneOutlivesTheGoal(t *testing.T) {
	db := open(t)

	goalID, _ := db.NewGoal(Goal{Name: "Keep the projects building"})

	taskID, err := db.NewTask(Task{Name: "Checking the projects", Goal: "check them"})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetTaskGoal(taskID, goalID); err != nil {
		t.Fatal(err)
	}

	done, _ := db.TasksForGoal(goalID, 5)

	if len(done) != 1 {
		t.Fatalf("%d tasks towards the goal", len(done))
	}

	if err := db.ForgetGoal(goalID); err != nil {
		t.Fatal(err)
	}

	task, _ := db.Task(taskID)

	if task == nil {
		t.Error("forgetting a goal deleted the work done towards it")
	}
}

/*
 * The review counts verified and claimed apart.
 *
 * A panel reporting how many tasks finished is a panel that always looks good:
 * a task finishes when its plan runs out of steps, whether or not anything in
 * it was ever confirmed.
 */
func TestTheReviewCountsWhatWasCheckedApartFromWhatWasClaimed(t *testing.T) {
	db := open(t)

	taskID, _ := db.NewTask(Task{Name: "A job", Goal: "do it", State: TaskDone})

	err := db.AddSteps(taskID, []TaskStep{
		{Instruction: "look", Kind: StepLook},
		{Instruction: "write", Kind: StepWrite},
		{Instruction: "check", Kind: StepDo},
	})
	if err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(taskID)

	for i, verdict := range []string{Verified, Claimed, Unmet} {
		steps[i].State = StepDone
		steps[i].Verdict = verdict

		if err := db.StartStep(steps[i].ID, "researcher", "ollama", "small:3b"); err != nil {
			t.Fatal(err)
		}

		if err := db.FinishStep(steps[i]); err != nil {
			t.Fatal(err)
		}
	}

	review, err := db.HowItHasBeenGoing(time.Now().AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}

	if review.Steps.Verified != 1 || review.Steps.Claimed != 1 || review.Steps.Unmet != 1 {
		t.Errorf("counted %+v", review.Steps)
	}

	if review.Tasks.Done != 1 {
		t.Errorf("tasks counted %+v", review.Tasks)
	}

	// And per person, so a model that claims everything can be seen doing it.
	if len(review.People) != 1 || review.People[0].Name != "researcher" {
		t.Fatalf("people: %+v", review.People)
	}

	if review.People[0].Verified != 1 || review.People[0].Claimed != 1 {
		t.Errorf("the researcher's record is %+v", review.People[0])
	}

	if len(review.Models) != 1 || review.Models[0].Name != "small:3b" {
		t.Errorf("models: %+v", review.Models)
	}
}

// Over a window, because "ever" flatters: a machine set up wrongly for a week
// and fine since should not read as half broken.
func TestTheReviewOnlyCountsTheWindow(t *testing.T) {
	db := open(t)

	taskID, _ := db.NewTask(Task{Name: "Old job", Goal: "x", State: TaskDone})

	if _, err := db.sql().Exec(
		`UPDATE tasks SET created_at = ? WHERE id = ?`,
		time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339), taskID,
	); err != nil {
		t.Fatal(err)
	}

	review, _ := db.HowItHasBeenGoing(time.Now().AddDate(0, 0, -7))

	if review.Tasks.Done != 0 {
		t.Errorf("a month-old task was counted in the last week: %+v", review.Tasks)
	}
}

/*
 * A week in which nothing could run still has something to report.
 *
 * Counting only steps that reached a verdict, a week where the model was
 * unreachable and every task stopped reported no steps of any kind — which
 * reads as "nothing to report" and is the exact opposite of what happened.
 * Flattery by omission is still flattery.
 */
func TestAWeekWhereNothingRanIsStillReported(t *testing.T) {
	db := open(t)

	taskID, _ := db.NewTask(Task{Name: "A job", Goal: "do it", State: TaskBlocked})

	if err := db.AddSteps(taskID, []TaskStep{{Instruction: "look", Kind: StepLook}}); err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(taskID)

	if err := db.StartStep(steps[0].ID, "researcher", "ollama", "small:3b"); err != nil {
		t.Fatal(err)
	}

	steps[0].State = StepFailed
	steps[0].Why = "the model it was using is not running"

	if err := db.FinishStep(steps[0]); err != nil {
		t.Fatal(err)
	}

	review, err := db.HowItHasBeenGoing(time.Now().AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}

	if review.Steps.Failed != 1 {
		t.Errorf("a step that could not run was not counted: %+v", review.Steps)
	}

	if review.Tasks.Blocked != 1 {
		t.Errorf("the task that stopped was not counted: %+v", review.Tasks)
	}

	// And whoever it was given to is still named, so the record is not silent
	// about which model could not be reached.
	if len(review.People) != 1 || review.People[0].Failed != 1 {
		t.Errorf("people: %+v", review.People)
	}
}
