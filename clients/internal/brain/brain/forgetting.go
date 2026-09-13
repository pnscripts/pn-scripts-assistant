package brain

import (
	"os"
	"strings"

	"pn-scripts-assistant/internal/brain/places"
)

/*
 * Forgetting what is no longer there.
 *
 * Half of getting sharper is stopping being wrong, and a memory that only ever
 * grows cannot do that. A document deleted last year is still quoted with
 * complete confidence, because the sentence about it is still the best match
 * for a question about it.
 *
 * The dangerous version of this is obvious and must never happen: the brain
 * lives on a removable drive, and the places it learns from are often on that
 * drive or another one. A sweep that retired everything it could not stat
 * would empty somebody's memory the first time they unplugged a disk, silently,
 * and the only symptom would be an assistant that had forgotten their work.
 *
 * So nothing is retired unless the place it came from is attached right now
 * and says so. A place in a drawer is not a place that is gone.
 */
func (b *Brain) forgetWhatIsGone() {
	watched, err := places.Status(b.Root)
	if err != nil {
		b.Log.Warn("could not check which places are attached", "error", err)

		return
	}

	// Only the ones plugged in. A drive in a drawer tells us nothing about
	// whether the files on it still exist.
	var attached []string

	for _, p := range watched {
		if p.Reachable {
			attached = append(attached, p.Path)
		}
	}

	if len(attached) == 0 {
		return
	}

	sources, err := b.DB.LivingSources()
	if err != nil {
		b.Log.Warn("could not read where its memories came from", "error", err)

		return
	}

	gone := 0

	for _, source := range sources {
		path, ok := pathOf(source)
		if !ok {
			// A conversation, or a website. Neither has a file to have lost.
			continue
		}

		if !under(path, attached) {
			continue
		}

		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Unreadable is not the same as absent — a permission that changed
			// is not a file that was deleted.
			continue
		}

		facts, err := b.DB.FactsFromSource(source)
		if err != nil {
			continue
		}

		for _, fact := range facts {
			if err := b.DB.Retire(fact.ID, "it came from "+path+", which is no longer there"); err != nil {
				b.Log.Warn("could not retire a memory", "fact", fact.ID, "error", err)

				continue
			}

			gone++
		}
	}

	if gone > 0 {
		b.Log.Info("stopped believing things whose source is gone", "memories", gone)
	}
}

// pathOf is the file or folder a source refers to, or false when it refers to
// no file at all.
func pathOf(source string) (string, bool) {
	for _, prefix := range []string{"document:", "project:"} {
		if rest, found := strings.CutPrefix(source, prefix); found {
			return rest, rest != ""
		}
	}

	return "", false
}

// under reports whether a path lies inside one of the attached places, so a
// sweep never reasons about a disk it cannot see.
func under(path string, attached []string) bool {
	for _, place := range attached {
		if path == place || strings.HasPrefix(path, strings.TrimSuffix(place, "/")+"/") {
			return true
		}
	}

	return false
}
