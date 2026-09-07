package brain

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"pn-scripts-assistant/internal/brain/copies"
)

/*
 * Keeping the copies up to date without being asked.
 *
 * A backup somebody has to remember to make is a backup from whenever they
 * last remembered. The drives this brain gets copied to are plugged in and
 * unplugged all day, so the useful moment is not a schedule — it is whenever
 * one of them is actually there.
 *
 * Cheap enough to do on a timer: the check is a stat of each folder, and a
 * copy is only written when the memory has changed since the last one.
 */

// HowOftenToCopy is the gap between looking at whether a copy is due.
//
// Not the gap between copies. Nothing is written unless the database has
// changed, so on a quiet afternoon this costs one stat per drive.
const HowOftenToCopy = 10 * time.Minute

// SettleBeforeFirstCopy keeps the first copy of a run out of the way of
// starting up, where the models are loading and the disk is busy.
const SettleBeforeFirstCopy = 2 * time.Minute

func (b *Brain) keepCopies(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(SettleBeforeFirstCopy):
	}

	tick := time.NewTicker(HowOftenToCopy)
	defer tick.Stop()

	for {
		b.CopyNowIfDue(ctx)

		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

/*
 * CopyNowIfDue refreshes the copies that are worth refreshing.
 *
 * "Worth" is doing real work: the database is written on every conversation
 * and every fact learned, so on a busy day this copies often, and on a day
 * where the brain sat idle it copies nothing at all rather than rewriting
 * gigabytes to say the same thing.
 */
func (b *Brain) CopyNowIfDue(ctx context.Context) {
	if gone, _ := b.DriveGone(); gone {
		// Copying from a brain whose own drive has left would write whatever
		// the database is at that moment, which is not what it knows.
		return
	}

	places, err := copies.Places(b.Root)
	if err != nil || len(places) == 0 {
		return
	}

	changed := b.memoryChangedAt()

	for _, place := range places {
		if !b.copyIsBehind(place, changed) {
			continue
		}

		made, err := copies.Write(ctx, b.DB, b.Root, place)
		if err != nil {
			b.Log.Warn("could not refresh the copy of the brain",
				"where", place, "error", err)

			continue
		}

		b.Log.Info("copy of the brain refreshed",
			"where", place, "facts", made.Facts, "megabytes", made.Bytes>>20)
	}
}

// memoryChangedAt is when the database was last written, which stands in for
// when the brain last learned or was talked to.
func (b *Brain) memoryChangedAt() time.Time {
	var newest time.Time

	// The write-ahead log as well as the database: on a brain that is running,
	// everything recent is in the log and the database file itself may not
	// have been touched for hours.
	for _, name := range []string{"brain.sqlite", "brain.sqlite-wal"} {
		if info, err := os.Stat(filepath.Join(b.Root, name)); err == nil {
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
	}

	return newest
}

// copyIsBehind reports whether the copy at dir is older than what the brain
// knows now, or is not there at all.
func (b *Brain) copyIsBehind(dir string, changed time.Time) bool {
	raw, err := os.Stat(filepath.Join(dir, copies.Marker))
	if err != nil {
		// No copy yet, or an unreachable drive. Write decides which; asking
		// twice would be a second stat for no gain.
		return true
	}

	if changed.IsZero() {
		return false
	}

	return changed.After(raw.ModTime())
}
