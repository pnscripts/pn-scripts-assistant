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
