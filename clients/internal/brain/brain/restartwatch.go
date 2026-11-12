package brain

import (
	"context"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/machinecare"
)

/*
 * "Tell me when I can restart" (machinecare.Watch).
 *
 * One watch at a time: asked again while one is going, the same one is
 * returned, never a second waiter on the same condition. It reads the
 * machine at once and then every few minutes, ends the moment a restart is
 * safe or at the deadline it named, and says the outcome where its user
 * will hear and see it: out loud as a reminder (the same voice and the same
 * list as every reminder) and in the conversation it was asked in. It
 * stops with the program. It never restarts anything.
 */

type restartWatching struct {
	mu     sync.Mutex
	watch  *machinecare.Watch
	cancel context.CancelFunc

	// read and every replace the machine and the clock in tests.
	read  func(context.Context) machinecare.Readiness
	every time.Duration
}

// RestartWatch is the watch going now, or the last one, or nil.
func (b *Brain) RestartWatch() *machinecare.Watch {
	b.restart.mu.Lock()
	defer b.restart.mu.Unlock()

	if b.restart.watch == nil {
		return nil
	}

	w := *b.restart.watch

	return &w
}

/*
 * WatchForRestart starts watching for d (machinecare.DefaultWatch when
 * zero), saying the outcome in conversation when it is not zero. already
 * is true when a watch was going: that one is returned and d is ignored.
 */
func (b *Brain) WatchForRestart(conversation int64, d time.Duration) (w machinecare.Watch, already bool, err error) {
	b.restart.mu.Lock()

	if b.restart.watch != nil && !b.restart.watch.Done() {
		defer b.restart.mu.Unlock()

		return *b.restart.watch, true, nil
	}

	fresh, err := machinecare.NewWatch(time.Now(), d)
	if err != nil {
		b.restart.mu.Unlock()

		return machinecare.Watch{}, false, err
	}

	fresh.Conversation = conversation

	ctx, cancel := context.WithCancel(context.Background())

	b.restart.watch, b.restart.cancel = &fresh, cancel
	b.restart.mu.Unlock()

	// The first look now, so "it is fine already" is said at once.
	if !b.lookForRestart(ctx) {
		go b.keepWatchingForRestart(ctx)
	}

	b.restart.mu.Lock()
	defer b.restart.mu.Unlock()

	return *b.restart.watch, false, nil
}

// StopRestartWatch ends a watch going now; it reports whether there was one.
func (b *Brain) StopRestartWatch() (machinecare.Watch, bool) {
	b.restart.mu.Lock()
	defer b.restart.mu.Unlock()

	if b.restart.watch == nil || b.restart.watch.Done() {
		return machinecare.Watch{}, false
	}

	b.restart.watch.Stop(time.Now())
	b.restart.cancel()

	return *b.restart.watch, true
}

func (b *Brain) keepWatchingForRestart(ctx context.Context) {
	every := b.restart.every
	if every <= 0 {
		every = machinecare.HowOftenToLook
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if b.lookForRestart(ctx) {
			return
		}
	}
}

/*
 * lookForRestart reads the machine once and applies it to the watch going
 * now; it reports whether the watch has ended, and says the outcome when
 * it ended with this look. The reading (git in up to thirty folders) is
 * done without the lock, so the page asking how the watch stands never
 * waits for it.
 */
func (b *Brain) lookForRestart(ctx context.Context) bool {
	read := b.restart.read
	if read == nil {
		read = func(ctx context.Context) machinecare.Readiness {
			return machinecare.Read(ctx, b.busyWithWhat(), b.workFolders())
		}
	}

	r := read(ctx)

	b.restart.mu.Lock()

	w := b.restart.watch
	if w == nil || w.Done() || ctx.Err() != nil {
		b.restart.mu.Unlock()

		return true
	}

	if !w.Look(r, time.Now()) {
		b.restart.mu.Unlock()

		return false
	}

	b.restart.cancel()

	text, conversation, state := w.Said(b.Cfg.Language), w.Conversation, w.State
	b.restart.mu.Unlock()

	b.Log.Info("restart watch ended", "state", state)

	if text == "" {
		return true
	}

	if _, err := b.DB.AddReminder(text, time.Now()); err != nil {
		b.Log.Warn("could not say how the restart watch ended", "error", err)
	}

	if conversation != 0 {
		if _, err := b.DB.AddMessage(conversation, llm.RoleAssistant, "", "", text); err != nil {
			b.Log.Warn("could not say in its conversation how the restart watch ended", "error", err)
		}
	}

	return true
}
