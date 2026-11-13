package brain

import (
	"context"
	"errors"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The program inventory (package programs): read once when the brain
 * starts, then kept current by file events, so a program installed,
 * updated or removed is known within seconds and without anybody asking.
 *
 * Not held for the quiet hours: a look is a few milliseconds of reading
 * (dpkg-query about 50 ms) and happens only when something changed, and an
 * inventory that is wrong until the evening is worse than none.
 *
 * Nothing here runs a program it found. Removals are remembered with their
 * time, so "why can't you use Krita?" has an answer.
 */

type programKeeping struct {
	// look serialises the looks: the first one and the watch's.
	look sync.Mutex

	mu       sync.Mutex
	watching *programs.Watching
	cancel   context.CancelFunc

	// scanner and opts replace the machine and the timings in tests; the
	// zero values are this computer and programs.DefaultOptions.
	scanner programs.Scanner
	opts    programs.Options
}

func (b *Brain) programScanner() programs.Scanner {
	if b.programs.scanner.Run != nil {
		return b.programs.scanner
	}

	return programs.FromEnvironment()
}

/*
 * KeepPrograms reads every source and then watches them, in the
 * background; once. StopPrograms ends it.
 */
func (b *Brain) KeepPrograms(ctx context.Context) {
	b.programs.mu.Lock()
	defer b.programs.mu.Unlock()

	if b.programs.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	b.programs.cancel = cancel

	opts := b.programs.opts
	if opts.Debounce <= 0 {
		opts = programs.DefaultOptions()
	}

	scanner := b.programScanner()

	go func() {
		targets := map[programs.Kind][]programs.Target{}

		for _, k := range programs.Kinds {
			if ctx.Err() != nil {
				return
			}

			// Targets first: a change made while the first look runs is
			// then seen by the watch and looked at again.
			targets[k] = scanner.Targets(ctx, k)
		}

		w := programs.Watch(ctx, targets, opts, func(k programs.Kind) {
			if _, err := b.lookAtPrograms(ctx, k); err != nil && ctx.Err() == nil {
				b.Log.Warn("could not read the programs", "source", k, "err", err)
			}
		})

		b.programs.mu.Lock()

		if ctx.Err() != nil {
			b.programs.mu.Unlock()
			w.Stop()

			return
		}

		b.programs.watching = w
		b.programs.mu.Unlock()

		for _, k := range programs.Kinds {
			if _, err := b.lookAtPrograms(ctx, k); err != nil && ctx.Err() == nil &&
				!errors.Is(err, programs.ErrNotHere) {
				b.Log.Warn("could not read the programs", "source", k, "err", err)
			}
		}
	}()
}

// StopPrograms ends the watch; nothing of it runs afterwards.
func (b *Brain) StopPrograms() {
	b.programs.mu.Lock()
	cancel, w := b.programs.cancel, b.programs.watching
	b.programs.watching = nil
	b.programs.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if w != nil {
		w.Stop()
	}
}

/*
 * lookAtPrograms reads one source and writes down what changed. A source
 * that cannot be read changes nothing: a busy dpkg must not remove every
 * package.
 */
func (b *Brain) lookAtPrograms(ctx context.Context, k programs.Kind) ([]programs.Change, error) {
	b.programs.look.Lock()
	defer b.programs.look.Unlock()

	seen, err := b.programScanner().Scan(ctx, k)
	if err != nil {
		return nil, err
	}

	known, err := b.DB.Programs(k)
	if err != nil {
		return nil, err
	}

	save, changes := programs.Reconcile(k, known, seen, time.Now().UTC())

	if err := b.DB.SavePrograms(save); err != nil {
		return nil, err
	}

	// Counts only: which programs somebody has is theirs, and the log is
	// not sealed.
	if len(known) > 0 && len(changes) > 0 {
		by := map[programs.ChangeKind]int{}
		for _, c := range changes {
			by[c.Kind]++
		}

		b.Log.Info("programs changed", "source", k, "added", by[programs.Added], "removed", by[programs.Removed],
			"returned", by[programs.Returned], "updated", by[programs.Updated])
	}

	return changes, nil
}

// ProgramInventory is every remembered program and how each source is kept
// current (nil before the watch started).
func (b *Brain) ProgramInventory() ([]programs.Record, []programs.SourceState, error) {
	rs, err := b.DB.Programs("")
	if err != nil {
		return nil, nil, err
	}

	b.programs.mu.Lock()
	w := b.programs.watching
	b.programs.mu.Unlock()

	var state []programs.SourceState
	if w != nil {
		state = w.State()
	}

	return rs, state, nil
}
