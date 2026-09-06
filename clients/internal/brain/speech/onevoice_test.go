package speech

import (
	"context"
	"testing"
	"time"
)

/*
 * Two voices at once.
 *
 * An answer is spoken sentence by sentence from a queue, which is orderly on
 * its own — but the queue is not the only thing that speaks. A greeting on
 * opening, a reminder falling due, a line said from the interface, and the
 * tail of the previous answer all arrive by different paths, and any two
 * landing together put two voices over each other saying different sentences.
 *
 * Which is worse than untidy: the microphone hears the room, so two voices is
 * what the recogniser is handed while somebody talks over them, and the
 * canceller subtracts one copy of what was played rather than two overlapping
 * ones.
 */
func TestOnlyOneThingSpeaksAtATime(t *testing.T) {
	t.Cleanup(drainVoice)

	if !waitToSpeak(context.Background()) {
		t.Fatal("could not take the turn to speak at all")
	}

	// A second caller waits rather than talking over the first.
	quick, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stop()

	if waitToSpeak(quick) {
		doneSpeaking()

		t.Fatal("a second voice started while the first was still speaking")
	}

	doneSpeaking()

	// And once the first has finished, the next one goes.
	if !waitToSpeak(context.Background()) {
		t.Fatal("the turn was not handed on")
	}

	doneSpeaking()
}

/*
 * Waiting a sentence is right; waiting an answer is not.
 *
 * A caller gives up by cancelling, which is what stops a reminder being held
 * for however long an answer takes and then said into a silence nobody is
 * waiting in.
 */
func TestSomethingWaitingToSpeakCanGiveUp(t *testing.T) {
	t.Cleanup(drainVoice)

	waitToSpeak(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)

	go func() { done <- waitToSpeak(ctx) }()

	cancel()

	select {
	case took := <-done:
		if took {
			t.Fatal("a cancelled caller took the turn anyway")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a cancelled caller went on waiting")
	}

	doneSpeaking()
}

/*
 * Releasing twice must not hand the turn out twice.
 *
 * It would mean a bug rather than a race, and the cost of being wrong about
 * that is two voices at once — the exact fault this exists to stop.
 */
func TestReleasingTwiceDoesNotHandOutTheTurnTwice(t *testing.T) {
	t.Cleanup(drainVoice)

	waitToSpeak(context.Background())

	doneSpeaking()
	doneSpeaking()

	if !waitToSpeak(context.Background()) {
		t.Fatal("the turn is gone")
	}

	quick, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()

	if waitToSpeak(quick) {
		doneSpeaking()

		t.Fatal("a double release let two callers hold it at once")
	}

	doneSpeaking()
}

func drainVoice() {
	for len(oneVoice) > 0 {
		doneSpeaking()
	}
}
