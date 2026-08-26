package learning

import (
	"context"
	"fmt"
)

// IngestReport says what a scan actually changed.
type IngestReport struct {
	Seen       int
	Recorded   int
	Rejected   int
	Promoted   int
	Duplicates int

	// Waiting is what was recorded but needs a person before it becomes
	// knowledge.
	Waiting int
	Failed  int
}

// Ingest records observations and runs them through the pipeline.
//
// A scanned observation names a path, so the Validator can recheck it against
// disk and it can be promoted without a person — unlike a claim a model made
// up, which waits. That distinction is the reason the pipeline has stages at
// all, and it is why this can run over thousands of files unattended.
//
// progress is called after each observation so a long scan can say where it is;
// nil is fine.
func (w *Worker) Ingest(ctx context.Context, observations Observations, progress func(IngestReport)) (IngestReport, error) {
	var rep IngestReport

	rep.Seen = len(observations)

	for _, o := range observations {
		if err := ctx.Err(); err != nil {
			return rep, err
		}

		status := w.Validator.StatusFor(o.Source)

		// A path that vanished between the scan and now is recorded as
		// rejected rather than dropped, so the history shows what was seen.
		if status == StatusRejected {
			rep.Rejected++

			continue
		}

		id, err := w.DB.AddLesson(0, o.Content, status, "high", o.Source)
		if err != nil {
			return rep, fmt.Errorf("recording an observation: %w", err)
		}

		rep.Recorded++

		if status != StatusValidated {
			// Recorded but not promoted: it is waiting for a person. Counted
			// separately, because a report saying "learned 0, known 0" when 33
			// things went into a review queue is a report that lies by omission.
			rep.Waiting++

			continue
		}

		lesson, err := w.DB.Lesson(id)
		if err != nil || lesson == nil {
			rep.Failed++

			continue
		}

		factID, err := w.Curator.Promote(ctx, *lesson)
		if err != nil {
			// One unembeddable observation must not end a scan of thousands.
			w.Log.Warn("could not promote an observation", "lesson", id, "error", err)
			rep.Failed++

			continue
		}

		if factID == 0 {
			rep.Duplicates++

			continue
		}

		rep.Promoted++

		if progress != nil {
			progress(rep)
		}
	}

	return rep, nil
}
