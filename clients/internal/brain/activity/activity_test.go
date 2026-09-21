package activity

import (
	"path/filepath"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/protocol"
)

func kept(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return db
}

/*
 * The whole point, in one test: somebody listening hears it as it happens,
 * and somebody who was away is told what they missed — the same events, in
 * the same order, with the same numbers.
 */
func TestWhatHappensIsHeardNowAndAfterwards(t *testing.T) {
	bus := New(kept(t))

	live, stop := bus.Watch(8)
	defer stop()

	first := bus.Say(protocol.New(protocol.TaskStarted, "Catch falling stars").About(1, 0))
	second := bus.Say(protocol.New(protocol.StepStarted, "write the game").About(1, 7))

	if first.Seq == 0 || second.Seq != first.Seq+1 {
		t.Fatalf("the order is not a count: %d then %d", first.Seq, second.Seq)
	}

	for _, want := range []protocol.Envelope{first, second} {
		select {
		case got := <-live:
			if got.Seq != want.Seq || got.Type != want.Type {
				t.Errorf("heard %+v, expected %+v", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("a listener that was there heard nothing")
		}
	}

	// And somebody who arrived late asks for everything after the first.
	missed, err := bus.Since(first.Seq, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(missed) != 1 || missed[0].Seq != second.Seq || missed[0].Said != "write the game" {
		t.Errorf("catching up gave %+v", missed)
	}

	if bus.Latest() != second.Seq {
		t.Errorf("the last number is %d, not %d", bus.Latest(), second.Seq)
	}
}

/*
 * One slow listener must not hold up the work.
 *
 * A phone on a bad connection is the case this is written for: its events are
 * dropped rather than blocking the task that produced them, and the gap in
 * the numbers is how it knows to ask for what it missed.
 */
func TestASlowListenerIsPassedOverRatherThanWaitedFor(t *testing.T) {
	bus := New(kept(t))

	slow, stop := bus.Watch(1)
	defer stop()

	done := make(chan int64, 1)

	go func() {
		var last int64

		for i := 0; i < 50; i++ {
			last = bus.Say(protocol.New(protocol.ProgressUpdate, "working").About(1, 0)).Seq
		}

		done <- last
	}()

	select {
	case last := <-done:
		// Nothing was lost from the record, whatever the listener saw.
		caught, err := bus.Since(0, 100)
		if err != nil || len(caught) != 50 || caught[len(caught)-1].Seq != last {
			t.Errorf("the record is not complete: %d rows, err %v", len(caught), err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a listener that stopped reading held up the work")
	}

	if len(slow) == 0 {
		t.Error("the slow listener was given nothing at all")
	}
}

// A bus with nothing behind it still carries events, and a nil bus is usable
// so that nothing has to check before saying what it did.
func TestABusWithNoRecordAndNoBusAtAll(t *testing.T) {
	bus := New(nil)

	live, stop := bus.Watch(2)
	defer stop()

	said := bus.Say(protocol.New(protocol.TaskCompleted, "done").About(2, 0))

	if said.Seq != 0 {
		t.Errorf("an unrecorded event was given a number: %d", said.Seq)
	}

	select {
	case got := <-live:
		if got.Type != protocol.TaskCompleted {
			t.Errorf("heard the wrong thing: %+v", got)
		}
	case <-time.After(time.Second):
		t.Error("an unrecorded event was not carried")
	}

	var none *Bus

	none.Say(protocol.New(protocol.TaskFailed, "nothing here"))

	if got, cancel := none.Watch(1); got == nil || cancel == nil {
		t.Error("a nil bus is not usable")
	}
}

// The record is trimmed to what a client could plausibly still need, and the
// newest is what survives.
func TestTheRecordIsTrimmedNewestFirst(t *testing.T) {
	db := kept(t)
	bus := New(db)

	for i := 0; i < 20; i++ {
		bus.Say(protocol.New(protocol.ProgressUpdate, "tick").About(3, 0))
	}

	gone, err := db.TrimHappenings(5)
	if err != nil {
		t.Fatal(err)
	}

	left, err := db.Happenings(0, 100)
	if err != nil {
		t.Fatal(err)
	}

	if gone != 15 || len(left) != 5 || left[len(left)-1].Seq != bus.Latest() {
		t.Errorf("trimming left %d rows and dropped %d", len(left), gone)
	}
}
