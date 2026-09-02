package brain

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
 * Noticing that the brain's own drive has been pulled out.
 *
 * Everything the brain does — remembering, recalling, even the record of the
 * conversation on screen — is a write to a file on the drive it lives on. When
 * that drive is a removable one it can leave in the middle of any of them,
 * and what happened then was nothing: no message, no pause, just writes
 * failing one at a time somewhere behind an interface that carried on looking
 * like it was working. The worst of it is that the failure is invisible in the
 * one direction that matters, since a brain that cannot save what it learns
 * still answers perfectly well.
 *
 * Checked rather than waited on. inotify does not fire for a disappearing
 * mount point, and polling a single stat every few seconds costs nothing
 * against being wrong about whether the last hour was recorded.
 */

// HowOftenToCheckTheDrive is the gap between stats. Long enough to be free,
// short enough that nobody has a long conversation with a brain that cannot
// keep it.
const HowOftenToCheckTheDrive = 4 * time.Second

type driveState struct {
	mu    sync.Mutex
	gone  bool
	since time.Time
}

// DriveGone reports whether the place the brain keeps everything has stopped
// being reachable, and since when.
func (b *Brain) DriveGone() (bool, time.Time) {
	b.drive.mu.Lock()
	defer b.drive.mu.Unlock()

	return b.drive.gone, b.drive.since
}

/*
 * rootIsThere asks the only question that matters, in the cheapest way.
 *
 * The marker file rather than the directory: an unmounted drive very often
 * leaves its mount point behind as an empty directory owned by root, so the
 * folder existing proves nothing. The marker is written by this program and is
 * on the drive itself, so it goes when the drive does.
 */
func (b *Brain) rootIsThere() bool {
	if b.Root == "" {
		return true
	}

	_, err := os.Stat(filepath.Join(b.Root, ".brain-root.json"))

	return err == nil
}

func (b *Brain) watchTheDrive(ctx context.Context) {
	tick := time.NewTicker(HowOftenToCheckTheDrive)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-tick.C:
			there := b.rootIsThere()

			b.drive.mu.Lock()
			was := b.drive.gone
			b.drive.gone = !there

			if !there && !was {
				b.drive.since = time.Now()
			}

			b.drive.mu.Unlock()

			if !there && !was {
				b.Log.Error("the drive holding the brain is not there any more",
					"root", b.Root)
			}

			/*
			 * And when it comes back, open it again.
			 *
			 * This used to be a log line and nothing else, on the belief that
			 * SQLite would find the file again by path on the next write. It
			 * does not. A connection holds the file it opened — the inode, not
			 * the name — so after the drive returns the old connection is
			 * still writing to the disk that left, while any connection opened
			 * afterwards writes to the file that is actually there. The brain
			 * then holds two databases and disagrees with itself about what it
			 * knows, which is worse than the outage it was recovering from.
			 *
			 * Measured, not assumed: writes on the surviving connection report
			 * success the whole time the folder is gone.
			 */
			if there && was {
				if err := b.DB.Reopen(); err != nil {
					b.Log.Error("the drive is back but the database would not open",
						"root", b.Root, "error", err)

					continue
				}

				b.Log.Info("the drive is back", "root", b.Root)
			}
		}
	}
}
