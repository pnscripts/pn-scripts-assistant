package learning

import (
	"context"
	"fmt"
	"strings"
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

// PageChunk is how much text goes to the extractor at once.
//
// The extractor returns one proposal per call, so a page has to be broken up
// or a long article yields a single fact. Small enough that each piece is about
// one thing, large enough that a fact is not split across two.
const PageChunk = 1200

// MaxPageChunks bounds what one page can cost.
//
// Each chunk is a model call, and a model call here is tens of seconds. A long
// article would otherwise take the rest of the afternoon, and the first few
// thousand words of a page are almost always the part worth having.
const MaxPageChunks = 6

// LearnFromText runs arbitrary text through the pipeline.
//
// Used for a web page, where there is no filesystem claim for the Validator to
// check. That is why everything from here waits for a person: the brain has
// read something written by someone else, and the difference between "this page
// says so" and "this is true about my owner" is exactly the judgement the
// review queue exists to hold.
func (w *Worker) LearnFromText(ctx context.Context, text, source string, note func(int, int)) (IngestReport, error) {
	var rep IngestReport

	chunks := splitForExtraction(text)

	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return rep, err
		}

		if note != nil {
			note(i+1, len(chunks))
		}

		rep.Seen++

		proposal, err := w.Extractor.Extract(ctx, chunk)
		if err != nil {
			rep.Failed++

			continue
		}

		// Finding nothing in a passage is the common and correct outcome.
		if proposal == nil {
			continue
		}

		status := w.Validator.StatusFor(source)

		if _, err := w.DB.AddLesson(0, proposal.Lesson, status, proposal.Confidence, source); err != nil {
			rep.Failed++

			continue
		}

		rep.Recorded++

		if status == StatusProposed {
			rep.Waiting++
		}
	}

	return rep, nil
}

// splitForExtraction breaks text into passages, preferring paragraph breaks.
func splitForExtraction(text string) []string {
	text = strings.TrimSpace(text)

	if text == "" {
		return nil
	}

	var chunks []string

	for len(text) > 0 && len(chunks) < MaxPageChunks {
		if len(text) <= PageChunk {
			chunks = append(chunks, text)

			break
		}

		// Cut at the last paragraph or sentence break inside the budget, so a
		// fact is not sliced in half.
		cut := strings.LastIndex(text[:PageChunk], "\n\n")

		if cut < PageChunk/2 {
			cut = strings.LastIndex(text[:PageChunk], ". ")
		}

		if cut < PageChunk/2 {
			cut = PageChunk
		}

		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}

	return chunks
}

/*
 * ManyToLearn is where a folder stops being a quick job.
 *
 * Every observation is embedded on the processor, so the cost is linear and
 * entirely in the waiting. Below this a scan finishes while somebody is still
 * looking at the screen; above it they should be told how long before it
 * starts rather than finding out by watching.
 */
const ManyToLearn = 200

// SecondsEachHere is what one observation costs to embed on a machine with no
// graphics card, measured rather than assumed: sixty-two projects took a little
// over two minutes.
const SecondsEachHere = 2
