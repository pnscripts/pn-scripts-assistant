package places

import (
	"context"
	"fmt"
	"os"
	"time"

	"pn-brain/internal/brain/learning"
)

/*
 * EachPass is how much of a place is taken in at one go.
 *
 * Every observation is embedded on the processor — measured at two to three
 * seconds here — so this is a few minutes of work, repeated until the place is
 * finished. Deliberately not "all of it": a drive can hold thousands, and an
 * hours-long job that cannot be interrupted is how a brain becomes something
 * you avoid plugging drives into.
 *
 * Small enough that unplugging the drive mid-pass costs at most this many
 * things, and they are simply seen again next time.
 */
const EachPass = 60

/*
 * SecondsEach is what one observation costs to take in on this machine.
 *
 * Measured, not guessed: embedding runs on the processor here and lands
 * between two and three seconds. It exists so that the size of a folder can be
 * turned into the only number anybody actually wants — how long this is going
 * to take.
 */
const SecondsEach = 3

// Reads is the part of the learner a pass needs.
//
// An interface rather than the worker, so a place can be looked at without
// this package depending on the whole of the brain — and so a test can watch
// what was handed over without embedding anything.
type Reads interface {
	Ingest(ctx context.Context, obs learning.Observations,
		progress func(learning.IngestReport)) (learning.IngestReport, error)
}

// Seen answers what has already been looked at, so it is not looked at twice.
type Seen interface {
	KnownSources(under string) (map[string]bool, error)
}

// Pass is what one look at a place did.
type Pass struct {
	Place Place

	// New is how much was found that had not been seen, before the cap.
	New int

	// Took is how much of that was actually taken in this time.
	Took int

	// Learned and Known are how it turned out: newly remembered, and already
	// known by content even though the file had not been read before.
	Learned int
	Known   int

	// Finished is whether there is nothing left to look at here.
	Finished bool
}

/*
 * Look takes the next bite out of a place.
 *
 * The order matters and is the whole of why this is affordable: scan the
 * folder, throw away everything already seen, and only then do the expensive
 * work on what is left. Scanning is cheap — it is reading names and a little
 * of each file — and embedding is not.
 */
func Look(ctx context.Context, learner Reads, seen Seen, owner string, p Place) (Pass, error) {
	out := Pass{Place: p}

	if info, err := os.Stat(p.Path); err != nil {
		return out, fmt.Errorf("%s is not there: %w", p.Path, err)
	} else if !info.IsDir() {
		return out, fmt.Errorf("%s is a file, not a folder", p.Path)
	}

	out.Place.LastSeen = time.Now().UTC()

	observations, err := scan(p, owner)
	if err != nil {
		return out, err
	}

	already, err := seen.KnownSources(p.Path)
	if err != nil {
		return out, err
	}

	fresh := make(learning.Observations, 0, len(observations))

	for _, o := range observations {
		if !already[o.Source] {
			fresh = append(fresh, o)
		}
	}

	out.New = len(fresh)
	out.Place.Waiting = len(fresh)
	out.Finished = len(fresh) == 0

	if len(fresh) == 0 {
		return out, nil
	}

	if len(fresh) > EachPass {
		fresh = fresh[:EachPass]
	}

	report, err := learner.Ingest(ctx, fresh, nil)

	// What was taken in counts even when the pass ended early — the drive was
	// unplugged, the program is shutting down. Those observations are stored,
	// and reporting zero would ask for them to be done again.
	out.Took = report.Seen
	out.Learned = report.Promoted
	out.Known = report.Duplicates

	if report.Recorded > 0 {
		out.Place.LastLearned = time.Now().UTC()
		out.Place.Learned = p.Learned + report.Promoted
	}

	if waiting := out.New - out.Took; waiting >= 0 {
		out.Place.Waiting = waiting
		out.Finished = waiting == 0
	}

	return out, err
}

// scan reads a place for whichever kinds it was added for.
func scan(p Place, owner string) (learning.Observations, error) {
	var out learning.Observations

	if p.Kind != Documents {
		found, err := learning.ScanProjects(p.Path)
		if err != nil {
			return nil, fmt.Errorf("reading the projects in %s: %w", p.Path, err)
		}

		out = append(out, learning.FromProjects(found, owner)...)
	}

	if p.Kind != Projects {
		found, err := learning.ScanDocuments(p.Path)
		if err != nil {
			return nil, fmt.Errorf("reading the documents in %s: %w", p.Path, err)
		}

		out = append(out, learning.FromDocuments(found, owner)...)
	}

	return out, nil
}

/*
 * Count says how much of a place has not been looked at, without doing any of
 * the work.
 *
 * Scanning only, so this is seconds rather than hours, and it is what makes
 * "2386 things, about 79 minutes" answerable before anybody commits to it.
 */
func Count(seen Seen, owner string, p Place) (int, error) {
	observations, err := scan(p, owner)
	if err != nil {
		return 0, err
	}

	already, err := seen.KnownSources(p.Path)
	if err != nil {
		return 0, err
	}

	var waiting int

	for _, o := range observations {
		if !already[o.Source] {
			waiting++
		}
	}

	return waiting, nil
}
