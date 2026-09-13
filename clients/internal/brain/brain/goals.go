package brain

import (
	"context"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Goals that come round on their own.
 *
 * Only the ones told to. A goal that says it is due and a goal that begins
 * work while nobody is at the machine are different things, and the second
 * must never become the first by accident — so this starts nothing unless
 * somebody set starts_itself on that particular goal.
 *
 * What a task may do once started is unchanged either way. Everything still
 * goes through the same gate, so the decision here is only about whether work
 * begins unasked, never about what it is allowed to be.
 */
const howOftenToCheckGoals = 10 * time.Minute

func (b *Brain) watchGoals(ctx context.Context) {
	ticker := time.NewTicker(howOftenToCheckGoals)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		b.startDueGoals(ctx)
	}
}

func (b *Brain) startDueGoals(ctx context.Context) {
	if b.Tasks == nil {
		return
	}

	due, err := b.DB.DueGoals(time.Now())
	if err != nil {
		b.Log.Warn("could not read what is due", "error", err)

		return
	}

	for _, goal := range due {
		if ctx.Err() != nil {
			return
		}

		if !goal.StartsItself {
			// It says it is due, in the room. Somebody decides.
			continue
		}

		/*
		 * One at a time, and never beside a task already running.
		 *
		 * This is one processor. Two plans running against each other on four
		 * cores is not concurrency, it is two things being slow — and the one
		 * nobody asked for should be the one that waits.
		 */
		if live, err := b.DB.LiveTasks(); err != nil || len(live) > 0 {
			return
		}

		task, started, err := b.Tasks.TakeForGoal(ctx, goal, b.latestConversation(), b.Cfg.DefaultProvider)
		if err != nil {
			b.Log.Warn("could not start work on a goal", "goal", goal.ID, "error", err)

			continue
		}

		if !started {
			b.Log.Info("a goal produced no work this time", "goal", goal.Name)

			continue
		}

		b.Log.Info("started work on a goal", "goal", goal.Name, "task", task.ID)

		b.SayInto(task.ConversationID, "Starting on "+goal.Name+
			", which was due. I'll tell you how it goes.")

		return
	}
}

// latestConversation is where a goal's work reports back to: the thread
// somebody was last using, or none, in which case one is started for it.
func (b *Brain) latestConversation() int64 {
	latest, err := b.DB.LatestConversation()
	if err != nil || latest == nil {
		return 0
	}

	return latest.ID
}

// WorkOnGoal starts a piece of work towards a goal now, because somebody asked.
func (b *Brain) WorkOnGoal(ctx context.Context, id int64) (*store.Task, error) {
	goal, err := b.DB.Goal(id)
	if err != nil {
		return nil, err
	}

	if goal == nil {
		return nil, errNoSuchGoal
	}

	task, _, err := b.Tasks.TakeForGoal(ctx, *goal, b.latestConversation(), b.Cfg.DefaultProvider)

	return task, err
}

type noSuchGoal struct{}

func (noSuchGoal) Error() string { return "there is no goal with that number" }

var errNoSuchGoal = noSuchGoal{}
