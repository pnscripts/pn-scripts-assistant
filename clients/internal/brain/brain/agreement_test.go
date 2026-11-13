package brain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/terms"
)

// starts counts how often the background work was started, in place of the
// real start, which reaches the machine's sound and microphone.
func starts(b *Brain) *atomic.Int32 {
	var n atomic.Int32

	b.begin = func(context.Context) { n.Add(1) }

	return &n
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}

	t.Fatalf("%s did not happen within 2 s", what)
}

// Nothing of its own starts before its terms are accepted, and everything
// starts, once, when they are.
func TestNothingStartsBeforeTheTermsAreAccepted(t *testing.T) {
	b := quietBrain(t)
	n := starts(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	if n.Load() != 0 || b.Running() {
		t.Fatal("background work started before the terms were accepted")
	}

	if _, err := terms.Accept(b.Root, terms.Version, "en", time.Now()); err != nil {
		t.Fatal(err)
	}

	b.Agreed()
	b.Agreed()

	waitFor(t, "the start after acceptance", func() bool { return n.Load() == 1 && b.Running() })

	time.Sleep(50 * time.Millisecond)

	if n.Load() != 1 {
		t.Fatalf("started %d times, want once", n.Load())
	}
}

// A brain agreed to before starts at once, as it always did.
func TestAnAgreedBrainStartsAtOnce(t *testing.T) {
	b := quietBrain(t)
	n := starts(b)

	if _, err := terms.Accept(b.Root, terms.Version, "bg", time.Now()); err != nil {
		t.Fatal(err)
	}

	b.Start(context.Background())

	if n.Load() != 1 || !b.Running() {
		t.Fatalf("an agreed brain started %d times", n.Load())
	}
}

// Closing the program before anybody accepted starts nothing on the way out.
func TestClosingBeforeAcceptingStartsNothing(t *testing.T) {
	for _, how := range []string{"stopped", "cancelled"} {
		t.Run(how, func(t *testing.T) {
			b := quietBrain(t)
			n := starts(b)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			b.Start(ctx)

			if how == "stopped" {
				b.Stop()
			} else {
				cancel()
				time.Sleep(20 * time.Millisecond)
			}

			b.Agreed()
			time.Sleep(50 * time.Millisecond)

			if n.Load() != 0 || b.Running() {
				t.Fatalf("%s before acceptance, and still started", how)
			}
		})
	}
}

// Agreed with no Start waiting (the one-off commands, the tests) does nothing.
func TestAgreedWithoutAStartDoesNothing(t *testing.T) {
	b := quietBrain(t)
	n := starts(b)

	b.Agreed()
	time.Sleep(20 * time.Millisecond)

	if n.Load() != 0 {
		t.Fatal("Agreed started work nobody asked to start")
	}
}
