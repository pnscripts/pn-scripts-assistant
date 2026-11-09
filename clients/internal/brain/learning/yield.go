package learning

import (
	"context"
	"errors"
	"time"

	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * Learning gives way to a question.
 *
 * Measured on a machine with four cores and no graphics card: a question asked
 * while the brain was reading its own knowledge folder waited about fourteen
 * minutes for its answer. The reading embeds one thing after another, two to
 * three seconds each, and the answer's model ran beside it on the same cores,
 * so both crawled and the one somebody was waiting for lost.
 *
 * Three ways of making the answer win were weighed:
 *
 *   - a priority on the model call. The local model server has none, and the
 *     hosted services are not ours to reorder.
 *   - a lower process priority for the reading. The work happens inside the
 *     model server, not in this program, so lowering this program changes
 *     nothing that matters.
 *   - stopping. The reading checks before each thing it takes in, and a long
 *     model call it already started is cancelled the moment a question
 *     arrives and done again afterwards. Nothing is lost: what was stored stays
 *     stored and the interrupted piece is simply repeated.
 *
 * The third is the only one that works on every machine, so it is the one
 * here. It applies to work nobody asked for at that moment, which says so by
 * carrying InBackground on its context: reading a folder because somebody
 * asked for it in the conversation is the question, and must not wait on
 * itself.
 */

// ErrGaveWay is the cause of a background call stopped for a question.
var ErrGaveWay = errors.New("stopped to let a question be answered first")

// GiveWayAtMost bounds how long background learning waits for questions to
// finish. Long, because an answer on a processor can take minutes and
// conversations come in runs; bounded, because a turn that never ends must not
// mean the brain never learns again.
var GiveWayAtMost = 15 * time.Minute

// MostRestarts is how many times one piece of background work is started
// again after giving way, before it is left for another time.
const MostRestarts = 3

type backgroundKey struct{}

// InBackground marks work nobody is waiting on, so it gives way to questions.
func InBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

// IsBackground reports whether ctx carries InBackground.
func IsBackground(ctx context.Context) bool {
	b, _ := ctx.Value(backgroundKey{}).(bool)

	return b
}

/*
 * GiveWay waits while a question is being answered or somebody is speaking.
 *
 * Returns whether it waited at all. A question (a turn) is waited for up to
 * GiveWayAtMost. Speaking, and any other answer on screen that is not a
 * question somebody asked here (a task the agent is working through), up to
 * HoldOff: a stuck recorder is more likely than a minute-long sentence, and a
 * task can run for an hour.
 */
func GiveWay(ctx context.Context) bool {
	waited := false

	start := time.Now()

	for ctx.Err() == nil {
		elapsed := time.Since(start)
		inTurn := progress.InATurn() && elapsed < GiveWayAtMost
		speaking := (speech.Recording() || progress.Answering()) && elapsed < HoldOff

		if !inTurn && !speaking {
			return waited
		}

		waited = true

		// Looked at again every half second, and never past the moment the
		// wait for speaking runs out.
		tick := 500 * time.Millisecond
		if left := HoldOff - elapsed; speaking && left < tick {
			tick = left
		}

		// Woken by the question ending rather than by the tick alone, so the
		// learning resumes the moment it may. A nil channel never fires.
		var over <-chan struct{}
		if inTurn {
			over = progress.NoTurn()
		}

		select {
		case <-ctx.Done():
		case <-over:
		case <-time.After(tick):
		}
	}

	return waited
}

/*
 * untilAQuestion is ctx, cancelled with ErrGaveWay once next is closed.
 *
 * next is progress.NextTurn taken before the caller last looked at InATurn,
 * so a turn starting after that look is never missed. A turn already running
 * when GiveWay stopped waiting for it (it ran past GiveWayAtMost) does not
 * stop the call: only a new question does.
 */
func untilAQuestion(ctx context.Context, next <-chan struct{}) (context.Context, context.CancelFunc) {
	inner, cancel := context.WithCancelCause(ctx)

	go func() {
		select {
		case <-inner.Done():
		case <-next:
			cancel(ErrGaveWay)
		}
	}()

	return inner, func() { cancel(context.Canceled) }
}

/*
 * waitForQuestions gives way, and returns the NextTurn to watch during the
 * call that follows.
 *
 * NextTurn is taken before each look at InATurn, so no question is missed;
 * and taken again after the wait, because one that started during the wait
 * has already closed the first. Watching that one would cancel the call the
 * moment it began, spending an attempt on a question that was waited for, not
 * one that interrupted. So: a question that started since the last look and
 * is still being answered is waited for as well; one that has already been
 * answered is simply past.
 *
 * A turn that was already running when the wait began and outlasted
 * GiveWayAtMost does not close next, so it ends the loop rather than keeping
 * the work waiting for ever (TestATurnThatNeverEndsDoesNotStopLearningForEver).
 */
func waitForQuestions(ctx context.Context) <-chan struct{} {
	next := progress.NextTurn()

	GiveWay(ctx)

	for ctx.Err() == nil {
		select {
		case <-next:
		default:
			// Nothing started since the look before the wait.
			return next
		}

		next = progress.NextTurn()

		if !progress.InATurn() {
			return next
		}

		GiveWay(ctx)
	}

	return next
}

// gaveWay reports whether a call under inner was stopped for a question,
// rather than by its caller.
func gaveWay(outer, inner context.Context) bool {
	return outer.Err() == nil && errors.Is(context.Cause(inner), ErrGaveWay)
}

/*
 * yielding runs call, giving way to questions when ctx is background work.
 *
 * Before the call it waits for any question to finish; during it, a question
 * that starts cancels the call, which is then run again once the question is
 * answered, at most MostRestarts times. Work somebody asked for runs as it is.
 */
func yielding[T any](ctx context.Context, call func(context.Context) (T, error)) (T, error) {
	if !IsBackground(ctx) {
		return call(ctx)
	}

	for restart := 0; ; restart++ {
		next := waitForQuestions(ctx)

		inner, stop := untilAQuestion(ctx, next)
		out, err := call(inner)
		interrupted := err != nil && gaveWay(ctx, inner)

		stop()

		if !interrupted {
			return out, err
		}

		if restart >= MostRestarts {
			return out, errors.Join(ErrGaveWay, err)
		}
	}
}
