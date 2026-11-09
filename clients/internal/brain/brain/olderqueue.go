package brain

import (
	"path"
	"strings"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/store"
)

// tidiedThe013Queue names the one-time tidy below in the brain's chores.
const tidiedThe013Queue = "1.0: readings of the knowledge folder queued by 0.13"

/*
 * tidyTheQueueOlderVersionsFilled clears what 0.13 left in the review queue,
 * once per brain.
 *
 * 0.13 never finished its own knowledge folder: it read the same guides every
 * ninety seconds and stored each reading again as a lesson waiting for a
 * person, with no check for one it already had. Past two hundred waiting,
 * reading stops on every drive (AsMuchAsAnyoneWillReview), and 1.0 stops new
 * ones but cannot unread the old: an upgraded brain would never read again.
 *
 * Only lessons still waiting whose source is a reading of the knowledge folder
 * are touched. Of those:
 *   - a repeat (same source and same words as one before it) is removed;
 *   - a reading of a skill, built in (pack-skills) or imported (skills), is
 *     removed: 1.0 does not read skills into memory at all (places);
 *   - each one left is filed the way 1.0 files that reading now
 *     (learning.Validator): validated when it is the brain's own guide or
 *     note and the file is there, rejected when the file is gone, still
 *     waiting only if 1.0 would ask a person too.
 * Everything else in the queue, a document's or a conversation's, still waits
 * for its owner exactly as it did.
 */
func (b *Brain) tidyTheQueueOlderVersionsFilled() {
	if b.DB == nil {
		return
	}

	seen := map[string]bool{}

	var v learning.Validator

	done, err := b.DB.TidyLessonsOnce(tidiedThe013Queue, "reading:"+learning.OwnKnowledge, func(l store.Lesson) string {
		rest := strings.TrimPrefix(strings.TrimPrefix(l.Source, "reading:"), learning.OwnKnowledge)
		clean := strings.TrimPrefix(path.Clean("/"+rest), "/")

		for _, theirs := range learning.NotOwnKnowledge {
			if strings.HasPrefix(clean, theirs) {
				return store.TidyDelete
			}
		}

		key := l.Source + "\x00" + l.Content
		if seen[key] {
			return store.TidyDelete
		}

		seen[key] = true

		if status := v.StatusFor(l.Source); status != learning.StatusProposed {
			return status
		}

		return ""
	})
	if err != nil {
		b.Log.Warn("could not tidy the review queue an older version filled", "error", err)

		return
	}

	if len(done) > 0 {
		b.Log.Info("tidied the review queue an older version filled",
			"removed", done[store.TidyDelete],
			"filed as checked", done[learning.StatusValidated],
			"gone", done[learning.StatusRejected])
	}
}
