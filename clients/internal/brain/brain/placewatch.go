package brain

import (
	"context"
	"fmt"
	"time"

	"pn-brain/internal/brain/places"
	"pn-brain/internal/brain/progress"
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
