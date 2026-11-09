package places

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/learning"
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

/*
 * Seen answers when each thing was last looked at, so it is not looked at
 * twice for nothing — and is looked at again when it has changed.
 *
 * When, rather than whether. Whether was the first version, and it meant a
 * document read once was never read again: every source ever recorded counted
 * as seen, at any status, forever. Memory could grow and could not correct
 * itself, which is most of why it could not get sharper.
 */
type Seen interface {
	SourcesSeenAt(under string) (map[string]time.Time, error)
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

	already, err := seen.SourcesSeenAt(rememberedAs(p.Path))
	if err != nil {
		return out, err
	}

	fresh := worthReading(observations, already)

	out.New = len(fresh)
	out.Place.Waiting = len(fresh)
	out.Finished = len(fresh) == 0

	if len(fresh) == 0 {
		return out, nil
	}

	if len(fresh) > EachPass {
		fresh = fresh[:EachPass]
	}

	/*
	 * And now the documents are actually opened.
	 *
	 * Here rather than in the scan, and after everything already known has
	 * been thrown away, because this is the expensive half: reading a file is
	 * cheap but every line it yields costs an embedding, and a scan that read
	 * every document on a drive to discover it had read them all last week
	 * would be an afternoon of processor for nothing.
	 *
	 * This is what the counting used to hide. Nine hundred and sixty-six
	 * documents "learned" meant nine hundred and sixty-six file names known,
	 * and not one question about what was inside any of them could be
	 * answered.
	 */
	things := readWhole(ctx, fresh, owner)

	var taking learning.Observations
	for _, t := range things {
		taking = append(taking, t...)
	}

	report, err := learner.Ingest(ctx, taking, nil)

	// What was taken in counts even when the pass ended early — the drive was
	// unplugged, the program is shutting down. Those observations are stored,
	// and reporting zero would ask for them to be done again.
	out.Took = takenWhole(things, report.Seen)
	out.Learned = report.Promoted
	out.Known = report.Duplicates

	if report.Recorded > 0 {
		out.Place.LastLearned = time.Now().UTC()
		out.Place.Learned = p.Learned + report.Promoted
	}

	/*
	 * Left is counted in the things that were found, and so is taken.
	 *
	 * A thing found is one observation; read, a document becomes several. Took
	 * used to be the second and New the first, so "left" was a difference of
	 * two units: negative and thrown away for a folder of documents, so the
	 * pass that finished a place never said so (and the next one, which took
	 * nothing, could not), or zero while most of the folder was still unread.
	 */
	out.Place.Waiting = out.New - out.Took
	out.Finished = out.Place.Waiting == 0

	return out, err
}

/*
 * readWhole opens the documents among things and keeps each one whole.
 *
 * One entry per thing found, holding what it became: itself for a listing,
 * the sentences a document says once it is read, nothing for a file that
 * could not be opened this time (it comes round again). Things are taken in
 * order until the next would go past EachPass observations; the first is
 * always taken, however much it says, or a long document would stop a place
 * for good.
 *
 * Whole because of what reading one is: its sentences are filed under one
 * source, and a source on record counts as read. Cutting the pass in the
 * middle of a document kept its first sentences and lost the rest for ever,
 * which is what applying the cap after reading did.
 */
func readWhole(ctx context.Context, things learning.Observations, owner string) []learning.Observations {
	var (
		out   []learning.Observations
		count int
	)

	for _, t := range things {
		if ctx.Err() != nil {
			break
		}

		became := learning.ReadContents(ctx, learning.Observations{t}, owner)

		if len(out) > 0 && count+len(became) > EachPass {
			break
		}

		out = append(out, became)
		count += len(became)
	}

	return out
}

/*
 * takenWhole is how many of things were taken in completely, given that the
 * learner went through the first seen of their observations.
 *
 * A thing that became nothing (a file that would not open) is not taken: it
 * was never recorded, so it is still there to read.
 */
func takenWhole(things []learning.Observations, seen int) int {
	taken := 0

	for _, t := range things {
		if len(t) == 0 {
			continue
		}

		if seen < len(t) {
			break
		}

		seen -= len(t)
		taken++
	}

	return taken
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

		out = append(out, learning.FromDocuments(withoutTheBuiltInSkills(p, found), owner)...)
	}

	return out, nil
}

// withoutTheBuiltInSkills leaves out the skills, the program's own and the
// imported ones, when p is the brain's knowledge folder. See PackSkillsFolder
// and ImportedSkillsFolder.
func withoutTheBuiltInSkills(p Place, docs []learning.Document) []learning.Document {
	if learning.Portable(p.Path) != learning.BrainPrefix+KnowledgeFolder {
		return docs
	}

	var skills []string
	for _, folder := range []string{PackSkillsFolder, ImportedSkillsFolder} {
		skills = append(skills, filepath.Join(p.Path, folder)+string(filepath.Separator))
	}

	kept := docs[:0:0]

	for _, d := range docs {
		skill := false
		for _, folder := range skills {
			skill = skill || strings.HasPrefix(filepath.Clean(d.Path), folder)
		}

		if !skill {
			kept = append(kept, d)
		}
	}

	return kept
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

	already, err := seen.SourcesSeenAt(rememberedAs(p.Path))
	if err != nil {
		return 0, err
	}

	// The same rule as the pass itself, so what it says is waiting and what it
	// then reads are the same set. Two functions deciding this separately is
	// how a place reports nothing to do and then does something.
	return len(worthReading(observations, already)), nil
}

/*
 * rememberedAs is a place's folder as the sources read from it are written.
 *
 * A folder inside the brain, which is what the knowledge folder is, is
 * remembered relative to the brain ("@brain/knowledge/…", learning.Portable),
 * so that it is the same folder wherever the brain is plugged in. Asking for
 * what was seen under its absolute path found nothing, every time: the brain
 * read its own guides again every ninety seconds, sixty at a pass, for as
 * long as it ran, never getting past the first bite and competing with every
 * question for the processor while it did.
 */
func rememberedAs(path string) string {
	return learning.Portable(path)
}

/*
 * worthReading is everything not seen, plus everything seen that has changed
 * since.
 *
 * The second half is the whole of what lets memory correct itself. An
 * observation that cannot say when its source changed is treated as unchanged
 * — the safe way round, because reading one thing twice costs seconds and
 * reading everything twice costs an evening.
 */
func worthReading(observations learning.Observations, seenAt map[string]time.Time) learning.Observations {
	fresh := make(learning.Observations, 0, len(observations))

	for _, o := range observations {
		seen, known := seenAt[o.Source]

		switch {
		case !known:
			fresh = append(fresh, o)
		case o.Changed.IsZero():
			// Nothing to compare against, so nothing has visibly changed.
		case o.Changed.After(seen):
			fresh = append(fresh, o)
		}
	}

	return fresh
}
