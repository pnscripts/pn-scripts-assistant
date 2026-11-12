package brain

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

func readyMachine(context.Context) machinecare.Readiness {
	return machinecare.Judge(machinecare.Inputs{})
}

func installingMachine(context.Context) machinecare.Readiness {
	return machinecare.Judge(machinecare.Inputs{Installing: []string{"apt"}})
}

func remindersSaid(t *testing.T, b *Brain) []string {
	t.Helper()

	due, err := b.DB.DueReminders(time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, r := range due {
		out = append(out, r.What)
	}

	return out
}

func waitForTheWatch(t *testing.T, b *Brain) machinecare.Watch {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if w := b.RestartWatch(); w != nil && w.Done() {
			return *w
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("the watch did not end: %+v", b.RestartWatch())

	return machinecare.Watch{}
}

/*
 * "Tell me when I can restart", with apt installing: watched, one watch
 * however often it is asked, and said once (out loud as a reminder and in
 * the conversation it was asked in) the moment apt has finished.
 */
func TestTheRestartWatchSaysOnceWhenItIsSafe(t *testing.T) {
	b := quietBrain(t)

	var looks atomic.Int32

	b.restart.every = 10 * time.Millisecond
	b.restart.read = func(ctx context.Context) machinecare.Readiness {
		if looks.Add(1) < 4 {
			return installingMachine(ctx)
		}

		return readyMachine(ctx)
	}

	conv, _ := b.DB.NewConversation("Can I restart?")

	w, already, err := b.WatchForRestart(conv, time.Hour)
	if err != nil || already || w.State != machinecare.Watching || len(w.InTheWay) != 1 {
		t.Fatalf("started: %+v %v %v", w, already, err)
	}

	// Asked again: the same watch, not a second waiter.
	if again, already, _ := b.WatchForRestart(conv, 2*time.Hour); !already || !again.Until.Equal(w.Until) {
		t.Fatalf("a second watch: %+v %v", again, already)
	}

	if ended := waitForTheWatch(t, b); ended.State != machinecare.Safe {
		t.Fatalf("%+v", ended)
	}

	time.Sleep(50 * time.Millisecond)

	if n := looks.Load(); n != 4 {
		t.Errorf("kept looking after it ended: %d looks", n)
	}

	said := remindersSaid(t, b)
	if len(said) != 1 || !strings.HasPrefix(said[0], "It is safe to restart now") {
		t.Fatalf("not said once: %q", said)
	}

	msgs, _ := b.DB.History(conv)
	if len(msgs) == 0 || !strings.HasPrefix(msgs[len(msgs)-1].Content, "It is safe to restart now") {
		t.Fatalf("not said in its conversation: %+v", msgs)
	}
}

// Fine already: said at once, and nothing goes on watching.
func TestARestartThatIsFineAlreadyIsSaidAtOnceInBulgarian(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Language = "bg"
	b.restart.read = readyMachine

	w, _, err := b.WatchForRestart(0, 0)
	if err != nil || w.State != machinecare.Safe {
		t.Fatalf("%+v %v", w, err)
	}

	if said := remindersSaid(t, b); len(said) != 1 || !strings.HasPrefix(said[0], "Вече може да рестартирате") {
		t.Fatalf("%q", said)
	}

	// A new ask after one ended is a new watch.
	b.restart.read = installingMachine

	if w2, already, _ := b.WatchForRestart(0, time.Hour); already || w2.State != machinecare.Watching {
		t.Fatalf("%+v %v", w2, already)
	}

	b.StopRestartWatch()
}

// Stopped by its user, or by the program closing: it ends quietly and
// stops looking.
func TestAStoppedRestartWatchSaysNothing(t *testing.T) {
	b := quietBrain(t)

	var looks atomic.Int32

	b.restart.every = 5 * time.Millisecond
	b.restart.read = func(ctx context.Context) machinecare.Readiness {
		looks.Add(1)

		return installingMachine(ctx)
	}

	if _, _, err := b.WatchForRestart(0, time.Hour); err != nil {
		t.Fatal(err)
	}

	if w, ok := b.StopRestartWatch(); !ok || w.State != machinecare.Stopped {
		t.Fatalf("%+v %v", w, ok)
	}

	time.Sleep(30 * time.Millisecond)
	n := looks.Load()
	time.Sleep(50 * time.Millisecond)

	if looks.Load() != n {
		t.Error("still looking after it was stopped")
	}

	if _, ok := b.StopRestartWatch(); ok {
		t.Error("stopped twice")
	}

	if said := remindersSaid(t, b); len(said) != 0 {
		t.Errorf("a stopped watch said %q", said)
	}

	// The program closing ends one too.
	b.WatchForRestart(0, time.Hour)
	b.Stop()

	if w := b.RestartWatch(); w == nil || w.State != machinecare.Stopped {
		t.Errorf("the program closed and the watch goes on: %+v", w)
	}

	if _, _, err := b.WatchForRestart(0, 13*time.Hour); err == nil {
		t.Error("a watch of 13 hours was accepted")
	}
}
