package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The brain has been carried to another machine.
 *
 * The drive lands at a different path everywhere it goes — /media/you/LABEL on
 * one Linux machine, /media/someone-else/LABEL on the next, /Volumes/LABEL on
 * a Mac, E:\ on Windows. Everything the brain has learned about files records
 * where those files were, so after the journey a thousand memories are
 * accurate and useless at the same time: they name real files by a path that
 * does not exist here.
 *
 * This is the failure the whole project has already been through once, when
 * every memory recorded a path that only existed inside a container. It was
 * invisible: recall worked, the sentences were right, and every path in them
 * pointed nowhere.
 *
 * So it is noticed and offered, never done. Rewriting a thousand memories
 * without asking is not a thing to do to somebody, and the person is the only
 * one who knows whether this is the same disk at a new mount point or a
 * different arrangement entirely.
 */

// Journey is what changed about where the brain lives, and what it means for
// what it knows.
type Journey struct {
	// From and To are where the brain was and where it is.
	From string `json:"from"`
	To   string `json:"to"`

	// OldPrefix and NewPrefix are the substitution that would repair the
	// memories — usually the mount point rather than the brain's own folder,
	// because what the memories name is everything else on that disk.
	OldPrefix string `json:"old_prefix"`
	NewPrefix string `json:"new_prefix"`

	// Affected is how many things it knows still name the old place.
	Affected int `json:"affected"`
}

// JourneyFile is where a journey waits to be dealt with, in the brain's own
// folder so it travels with the brain rather than with the machine.
const JourneyFile = "journey.json"

/*
 * NoteJourney records that this run found the brain somewhere new.
 *
 * Written down rather than held in memory, and that is the whole of why this
 * is trustworthy. A journey is noticed exactly once — the moment the pointer
 * is updated there is nothing left to compare against — so an offer that lived
 * only in the running process would be gone for good the first time somebody
 * closed the window without acting on it, while the memories stayed broken
 * with nothing anywhere to say so.
 */
func (b *Brain) NoteJourney(from string) {
	if from == "" || from == b.Root {
		return
	}

	/*
	 * The first place, not the last one.
	 *
	 * A drive can be carried twice before anybody deals with it: from the
	 * machine it was written on, to a second one where the offer was ignored,
	 * to a third. What the memories name is still the first path — so
	 * recording the second journey as "from the second machine" would offer a
	 * substitution that matches nothing, and quietly throw away the one that
	 * would have worked.
	 */
	if earlier := b.noted(); earlier != "" {
		from = earlier
	}

	b.mu.Lock()
	b.journeyFrom = from
	b.mu.Unlock()

	raw, err := json.Marshal(map[string]string{"from": from, "to": b.Root})
	if err != nil {
		return
	}

	if err := os.WriteFile(filepath.Join(b.Root, JourneyFile), raw, 0o644); err != nil {
		b.Log.Warn("the drive has moved but that could not be written down", "error", err)
	}
}

/*
 * noted is the origin already written down, whatever this run found.
 *
 * Read straight from the file rather than through pendingJourney, because that
 * one throws the note away when the destination has moved on — which is
 * exactly the case this exists to preserve.
 */
func (b *Brain) noted() string {
	raw, err := os.ReadFile(filepath.Join(b.Root, JourneyFile))
	if err != nil {
		return ""
	}

	var was struct {
		From string `json:"from"`
	}

	if err := json.Unmarshal(raw, &was); err != nil {
		return ""
	}

	return was.From
}

/*
 * pendingJourney is a journey from an earlier run that nobody has dealt with.
 *
 * Read on every ask rather than cached, because the file is what makes the
 * offer survive a restart and the two must not be able to disagree.
 */
func (b *Brain) pendingJourney() string {
	b.mu.Lock()
	from := b.journeyFrom
	b.mu.Unlock()

	if from != "" {
		return from
	}

	raw, err := os.ReadFile(filepath.Join(b.Root, JourneyFile))
	if err != nil {
		return ""
	}

	var noted struct {
		From string `json:"from"`
		To   string `json:"to"`
	}

	if err := json.Unmarshal(raw, &noted); err != nil {
		return ""
	}

	/*
	 * A note whose destination is out of date is corrected, not dropped.
	 *
	 * The brain having moved again since does not make the first journey
	 * untrue — the memories still name where they were written — so what is
	 * stale is the "to", and the "to" is simply wherever the brain is now.
	 */
	if noted.To != b.Root {
		b.NoteJourney(noted.From)
	}

	return noted.From
}

/*
 * ForgetJourney puts the offer away without doing anything.
 *
 * For somebody who knows the old paths do not matter — a brain that learned
 * from folders it has no interest in any more, or one where the drive letter
 * changing was the point. Being asked the same question at every start, with
 * no way to answer "no", is its own kind of broken.
 */
func (b *Brain) ForgetJourney() { b.forgetJourney() }

func (b *Brain) forgetJourney() {
	b.mu.Lock()
	b.journeyFrom = ""
	b.mu.Unlock()

	os.Remove(filepath.Join(b.Root, JourneyFile))
}

/*
 * Travelled describes the journey, or nothing if there was none.
 *
 * Counted rather than assumed: a drive that moved but whose memories do not
 * mention the old path — a brain that has only ever been talked to, never
 * pointed at a folder — needs no repair, and offering one would be inventing
 * work.
 */
func (b *Brain) Travelled() *Journey {
	from := b.pendingJourney()

	if from == "" {
		return nil
	}

	j := Journey{From: from, To: b.Root}
	j.OldPrefix, j.NewPrefix = prefixes(from, b.Root)

	n, err := b.DB.CountContaining(j.OldPrefix)
	if err != nil {
		b.Log.Warn("could not count what the journey affected", "error", err)

		return nil
	}

	j.Affected = n

	/*
	 * A journey that broke nothing needs no offer, and the note goes with it.
	 *
	 * A brain that has only ever been talked to — never pointed at a folder —
	 * has no memory naming a path, so the drive moving cost it nothing.
	 */
	if n == 0 {
		b.forgetJourney()

		return nil
	}

	return &j
}

/*
 * prefixes works out what to replace with what.
 *
 * The brain's own folder is rarely what its memories talk about. They talk
 * about the rest of the disk — the projects and documents beside it — so when
 * the folder kept its name and only the path in front of it changed, which is
 * what a drive appearing at a new mount point looks like, the substitution
 * that repairs anything is the one on the mount point.
 *
 * When the folder itself was renamed or moved somewhere unrelated, there is no
 * safe wider claim to make, and only the root's own path is offered.
 */
func prefixes(from, to string) (string, string) {
	if filepath.Base(from) == filepath.Base(to) {
		return filepath.Dir(from), filepath.Dir(to)
	}

	return from, to
}

/*
 * RepairJourney rewrites the paths in what it knows, and re-embeds what
 * changed.
 *
 * The re-embedding is not optional and not a detail. Changing the text without
 * recomputing the vector leaves the two describing different things, and the
 * mismatch is invisible: recall carries on working, slightly wrong, forever.
 */
func (b *Brain) RepairJourney(ctx context.Context, j Journey) (changed, reEmbedded int, err error) {
	rewritten, err := b.DB.RewritePaths([]store.PathRewrite{{From: j.OldPrefix, To: j.NewPrefix}})
	if err != nil {
		return 0, 0, err
	}

	if len(rewritten) == 0 {
		b.clearJourney()

		return 0, 0, nil
	}

	embedder, err := b.Router.Embedder()
	if err != nil {
		return len(rewritten), 0, fmt.Errorf(
			"the paths were repaired but cannot be re-embedded yet: %w", err)
	}

	for _, r := range rewritten {
		vec, embedErr := embedder.Embed(ctx, r.New)
		if embedErr != nil {
			b.Log.Warn("could not re-embed a repaired memory",
				"table", r.Table, "id", r.ID, "error", embedErr)

			continue
		}

		if err := b.DB.SetEmbedding(r.Table, r.ID, vec); err != nil {
			b.Log.Warn("could not store a repaired memory's vector",
				"table", r.Table, "id", r.ID, "error", err)

			continue
		}

		reEmbedded++
	}

	b.clearJourney()

	b.Log.Info("repaired the paths after the brain was carried to another machine",
		"from", j.OldPrefix, "to", j.NewPrefix,
		"rewritten", len(rewritten), "re-embedded", reEmbedded)

	return len(rewritten), reEmbedded, nil
}

func (b *Brain) clearJourney() { b.forgetJourney() }

// JourneyLine is what the greeting says about it, or nothing.
func (b *Brain) journeyLine() string {
	j := b.Travelled()

	if j == nil {
		return ""
	}

	return fmt.Sprintf(
		"I have been opened somewhere new: this drive was at %s and is now at %s. "+
			"%d of the things I know still name the old place, so I cannot open those "+
			"files until they are repaired — there is a button for it in the storage panel.",
		short(j.OldPrefix), short(j.NewPrefix), j.Affected)
}

// short trims a path to something sayable, since this line is read aloud.
func short(path string) string {
	if len(path) <= 40 {
		return path
	}

	return "…" + path[len(path)-39:]
}
