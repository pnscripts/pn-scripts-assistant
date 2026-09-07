package brain

import (
	"context"
	"fmt"
	"time"

	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/progress"
)

/*
 * Working through the drives and folders it looks after.
 *
 * The rhythm is the point. One bite at a time, with a gap between them, so a
 * drive holding two thousand documents is learned over an evening without the
 * machine ever being unusable and without any single job that cannot be
 * interrupted. Unplugging the drive in the middle costs the current bite and
 * nothing else.
 *
 * Nothing here asks. A place was added on purpose, and coming back later to
 * approve each folder again would make the feature a chore rather than the
 * thing that removes one.
 */

// BetweenBites is the pause between one bite of a place and the next.
//
// Long enough that the machine is not permanently busy embedding, short enough
// that a drive plugged in for an afternoon gets properly read.
const BetweenBites = 90 * time.Second

// BetweenRounds is the pause when there was nothing to do anywhere — every
// place either finished or not attached.
const BetweenRounds = 10 * time.Minute

// SettleBeforeLooking keeps this out of the way of starting up, where the
// models are loading and the disk is busy.
const SettleBeforeLooking = 3 * time.Minute

/*
 * AsMuchAsAnyoneWillReview is where the brain stops asking and waits.
 *
 * The rule that matters most, and the only one here that does not depend on
 * guessing whose writing a file is. Every other rule in this program is a
 * judgement about content, and judgements are wrong sometimes — the queue went
 * to 9,199, then to 2,895, each time because a rule that was right about
 * yesterday's disk met a folder nobody had thought of. On a billion different
 * machines it will keep meeting them, and no list of folder names written here
 * will ever have seen what is on somebody else's.
 *
 * So the queue is bounded by what a person can actually do, rather than by how
 * clever the filtering was. Two hundred is a long evening's work and a number
 * somebody can imagine finishing. Nine thousand is not a queue, it is a wall,
 * and the difference between them is not nine thousand — it is that one of
 * them gets looked at.
 *
 * When it is full the reading stops rather than the proposing: work that will
 * be thrown away is not worth the processor on a machine where an answer takes
 * a minute. It starts again by itself the moment the queue comes down.
 */
const AsMuchAsAnyoneWillReview = 200

func (b *Brain) keepPlaces(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(SettleBeforeLooking):
	}

	for {
		did := b.lookAtOnePlace(ctx)

		wait := BetweenRounds
		if did {
			wait = BetweenBites
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

/*
 * lookAtOnePlace takes one bite out of whichever place needs it.
 *
 * One, not all of them: a machine with four drives attached should not be
 * embedding four folders at once on four cores it also needs for answering.
 * Round by round, every place gets its turn.
 */
func (b *Brain) lookAtOnePlace(ctx context.Context) bool {
	if gone, _ := b.DriveGone(); gone {
		// Nothing learned now could be saved, and the learning is the
		// expensive half.
		return false
	}

	if b.Learner == nil {
		return false
	}

	/*
	 * Nothing is read while the person already has more than they will get
	 * through. See AsMuchAsAnyoneWillReview.
	 *
	 * Checked before the work rather than after it: reading a folder is
	 * minutes of processor on this machine, and producing things nobody will
	 * ever look at is the most expensive way to do nothing.
	 */
	if waiting, err := b.DB.CountPendingLessons(); err == nil && waiting >= AsMuchAsAnyoneWillReview {
		b.sayTheQueueIsFull(waiting)

		return false
	}

	list, err := places.Status(b.Root)
	if err != nil || len(list) == 0 {
		return false
	}

	for _, p := range list {
		if !p.Reachable {
			continue
		}

		/*
		 * Said while it happens, not after.
		 *
		 * A brain quietly embedding for four minutes looks like a brain doing
		 * nothing, and this is work nobody asked for at that moment — so it
		 * has to account for itself on the screen. As background, so the core
		 * does not turn the colour it uses for answering a question.
		 */
		progress.SetBackground("learning", "Reading "+p.Name)

		pass, err := places.Look(ctx, b.Learner, b.DB, b.Cfg.Owner, p)

		// Written back whatever happened: a pass that ended early still saw
		// things, and what is left to read is still worth keeping.
		places.Note(b.Root, pass.Place)

		if err != nil {
			progress.SetBackground("learning", "Could not read "+p.Name)
			progress.Done()

			b.Log.Warn("could not read a place", "place", p.Path, "error", err)

			return false
		}

		if pass.Took == 0 {
			progress.Done()

			continue
		}

		progress.SetBackground("learning", describePass(p.Name, pass))
		progress.Done()

		b.Log.Info("read some of a place",
			"place", p.Path, "took", pass.Took,
			"learned", pass.Learned, "left", pass.Place.Waiting)

		// Reading at all means the queue came down, so the explanation for
		// having stopped is due to be given afresh if it fills up once more.
		b.mu.Lock()
		b.saidQueueIsFull = false
		b.mu.Unlock()

		/*
		 * Said once, at the end, rather than on every bite.
		 *
		 * A place is read a few files at a time over hours, and each bite was
		 * a fresh step for the page to narrate — so the brain said "I am
		 * learning from that" every few seconds for an afternoon. Announcing
		 * the middle of a long job is the one thing worse than announcing
		 * nothing: it talks over the person the work was meant to stay out of
		 * the way of.
		 *
		 * So the middle is silent on the screen only, and this is the sentence
		 * — when the folder is finished, saying what came out of it. See
		 * finishedReading.
		 */
		if pass.Finished {
			b.sayWhenFinished(b.finishedReading(p, pass))
		}

		return true
	}

	return false
}

// describePass is what to put on the screen about a bite that was taken.
//
// In the terms somebody cares about: what it now knows that it did not, and
// how much of that drive is left — not how many rows were written.
func describePass(name string, pass places.Pass) string {
	learned := "1 new thing"
	if pass.Learned != 1 {
		learned = fmt.Sprintf("%d new things", pass.Learned)
	}

	if pass.Place.Waiting > 0 {
		return fmt.Sprintf("%s from %s, %d still to read", learned, name, pass.Place.Waiting)
	}

	return learned + " from " + name + " — that is all of it"
}

/*
 * finishedReading is the sentence said when a folder has been read to the end.
 *
 * What Petar asked for, in his words: not "I am learning from that" over and
 * over, but "I learned …" once, when the queue for that thing is finished,
 * with everything about it.
 *
 * Three numbers, and they answer three different questions. How much it now
 * knows from there, because that is what the reading was for. How much of it
 * turned out to be already known, because a folder that produced nothing new
 * is not the same as a folder that produced nothing and somebody who is told
 * only "finished" cannot tell those apart. And how much is waiting on them,
 * because that is the only part that is now their job rather than the brain's.
 */
func (b *Brain) finishedReading(p places.Place, pass places.Pass) string {
	waiting, err := b.DB.CountPendingUnder(p.Path)
	if err != nil {
		b.Log.Debug("could not count what is waiting from a place", "error", err)
	}

	line := fmt.Sprintf("I have finished reading %s.", p.Name)

	// The total after this pass, not before it. p is the copy the loop was
	// given at the top; pass.Place is the one that has just been written back.
	learned := pass.Place.Learned

	switch {
	case learned == 0:
		line += " There was nothing in it worth remembering."
	case learned == 1:
		line += " I learned one thing from it."
	default:
		line += fmt.Sprintf(" I learned %d things from it.", learned)
	}

	if pass.Known > 0 {
		line += fmt.Sprintf(" %s already knew.",
			count(pass.Known, "One more I", fmt.Sprintf("%d more I", pass.Known)))
	}

	switch {
	case waiting == 1:
		line += " One of them is waiting for you to keep or discard."
	case waiting > 1:
		line += fmt.Sprintf(" %d of them are waiting for you to keep or discard.", waiting)
	}

	return line
}

/*
 * sayTheQueueIsFull explains the pause, once.
 *
 * Once, because this is checked every ten minutes for as long as the queue
 * stays full, and a program that repeats itself every ten minutes is a program
 * somebody turns off. The flag clears when the reading starts again, so it is
 * said afresh the next time it happens.
 *
 * Said at all because a brain that has quietly stopped reading looks exactly
 * like a brain that has finished, and those mean opposite things — one is
 * waiting for its owner and the other is done. Somebody who is not told will
 * conclude the feature is broken.
 */
func (b *Brain) sayTheQueueIsFull(waiting int) {
	b.mu.Lock()
	said := b.saidQueueIsFull
	b.saidQueueIsFull = true
	b.mu.Unlock()

	if said {
		return
	}

	b.Log.Info("paused reading: the review queue is full", "waiting", waiting)

	b.sayWhenFinished(fmt.Sprintf(
		"There are %d things waiting for you to keep or discard, which is as many "+
			"as I will put in front of anybody at once. I have stopped reading until "+
			"you have been through some of them — say \"remember everything in the "+
			"waiting list\" and I will take them all, or go through them on the "+
			"command centre.", waiting))
}
