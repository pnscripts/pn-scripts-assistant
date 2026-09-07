package brain

import (
	"context"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * What is waiting, in the terms the tools need.
 *
 * The queue tools take an interface rather than the brain itself, because the
 * brain builds the registry and so the tools cannot import it back. This is the
 * other side of that interface, and it is deliberately thin: listing, and
 * deciding one item, with the result described rather than counted.
 */

// PendingActions lists proposed actions waiting for a yes.
func (b *Brain) PendingActions() ([]tools.QueueItem, error) {
	rows, err := b.DB.PendingInvocations()
	if err != nil {
		return nil, err
	}

	out := make([]tools.QueueItem, 0, len(rows))

	for _, row := range rows {
		summary := strings.TrimSpace(row.Summary)
		if summary == "" {
			summary = row.Tool
		}

		out = append(out, tools.QueueItem{ID: row.ID, Summary: summary})
	}

	return out, nil
}

// PendingLessons lists things the brain wants to remember, waiting for a yes.
func (b *Brain) PendingLessons() ([]tools.QueueItem, error) {
	rows, err := b.DB.LessonsByStatus("proposed", 100)
	if err != nil {
		return nil, err
	}

	out := make([]tools.QueueItem, 0, len(rows))

	for _, row := range rows {
		out = append(out, tools.QueueItem{ID: row.ID, Summary: strings.TrimSpace(row.Content)})
	}

	return out, nil
}

// DecideAction answers one proposed action and says what became of it.
func (b *Brain) DecideAction(ctx context.Context, id int64, approve bool) (string, error) {
	invocation, err := b.Decide(ctx, id, approve)
	if err != nil {
		return "", err
	}

	what := strings.TrimSpace(invocation.Summary)
	if what == "" {
		what = invocation.Tool
	}

	if !approve {
		return fmt.Sprintf("refused: %s", what), nil
	}

	// Decide runs the action, and an action that failed must not be reported as
	// done — the whole value of this is that the sentence read back is true.
	if strings.TrimSpace(invocation.Result) != "" && invocation.Status != "approved" {
		return fmt.Sprintf("tried %s: %s", what, invocation.Result), nil
	}

	return fmt.Sprintf("did: %s", what), nil
}

// DecideLessonItem keeps or discards one thing the brain wanted to remember.
func (b *Brain) DecideLessonItem(ctx context.Context, id int64, keep bool) (string, error) {
	decision, err := b.DecideLesson(ctx, id, keep)
	if err != nil {
		return "", err
	}

	if !keep {
		return fmt.Sprintf("forgot lesson %d", id), nil
	}

	if decision.Duplicate {
		return fmt.Sprintf("lesson %d was already known", id), nil
	}

	return fmt.Sprintf("remembered lesson %d", id), nil
}

/*
 * queueOf adapts the brain to the queue the tools expect.
 *
 * A named type rather than the brain directly, because DecideLesson already
 * exists with a different shape — it returns the decision for the interface to
 * render — and two methods cannot share one name.
 */
type queueOf struct{ b *Brain }

func (q queueOf) PendingActions() ([]tools.QueueItem, error) { return q.b.PendingActions() }
func (q queueOf) PendingLessons() ([]tools.QueueItem, error) { return q.b.PendingLessons() }

func (q queueOf) DecideAction(ctx context.Context, id int64, approve bool) (string, error) {
	return q.b.DecideAction(ctx, id, approve)
}

func (q queueOf) DecideLesson(ctx context.Context, id int64, keep bool) (string, error) {
	return q.b.DecideLessonItem(ctx, id, keep)
}
