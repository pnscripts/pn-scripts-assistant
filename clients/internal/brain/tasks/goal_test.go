package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Work started for a goal is tied to it and counted as having been done.
 *
 * The tie is what makes a goal more than a note: "what has been done towards
 * this, and how did it go" is the only useful question about something that
 * never finishes.
 */
func TestWorkForAGoalIsTiedToIt(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "Nothing to report."}}}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	goalID, err := db.NewGoal(store.Goal{
		Name: "Keep my projects building", Why: "check each one still opens", EveryDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}

	goal, _ := db.Goal(goalID)

	task, started, err := c.TakeForGoal(context.Background(), *goal, conv, "scripted")
	if err != nil || !started {
		t.Fatalf("the work did not start: %v %v", started, err)
	}

	settled(t, db, task.ID)

	done, _ := db.TasksForGoal(goalID, 5)

	if len(done) != 1 || done[0].ID != task.ID {
		t.Fatalf("%d tasks are tied to the goal", len(done))
	}

	// And the goal is no longer due, so it does not come round immediately.
	after, _ := db.Goal(goalID)

	if after.Due {
		t.Error("the goal is still due after being worked on")
	}

	if after.LastWorked.IsZero() {
		t.Error("nothing recorded that it was worked on")
	}
}

// The goal's own words are the job — what somebody wrote when they were
// thinking about it, rather than about this particular Tuesday.
func TestTheGoalsOwnWordsBecomeTheJob(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "Done."}}}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	goalID, _ := db.NewGoal(store.Goal{
		Name: "Keep my projects building",
		Why:  "check each one still opens and exports, and tell me what broke",
	})

	goal, _ := db.Goal(goalID)

	task, _, err := c.TakeForGoal(context.Background(), *goal, conv, "scripted")
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	if !strings.Contains(task.Goal, "still opens and exports") {
		t.Errorf("the goal's words were lost: %q", task.Goal)
	}

	if !strings.Contains(task.Goal, "Keep my projects building") {
		t.Errorf("the goal's name was lost: %q", task.Goal)
	}
}

/*
 * A goal is marked as worked on even when the attempt produced nothing.
 *
 * Otherwise a goal whose last look found nothing to do stays due, comes round
 * immediately, finds nothing again, and does that forever.
 */
func TestAGoalThatProducedNothingStillComesRoundLater(t *testing.T) {
	// A model that refuses to plan, so nothing is started at all.
	model := &scripted{plan: "I am not going to answer with JSON."}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	goalID, _ := db.NewGoal(store.Goal{Name: "A standing thing", EveryDays: 7})
	goal, _ := db.Goal(goalID)

	if !goal.Due {
		t.Fatal("it was not due to begin with")
	}

	if _, _, err := c.TakeForGoal(context.Background(), *goal, conv, "scripted"); err != nil {
		t.Fatal(err)
	}

	after, _ := db.Goal(goalID)

	if after.Due {
		t.Error("a goal that produced no work is still due, so it will loop")
	}

	if after.NextDue.Before(time.Now().AddDate(0, 0, 6)) {
		t.Errorf("it comes round at %v, which is too soon", after.NextDue)
	}
}
