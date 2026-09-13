package speech

import (
	"context"
	"os"
	"testing"
	"time"
)

/*
 * Cutting the voice off actually stops the sound.
 *
 * Skipped unless asked for, because it makes a noise on whoever's machine runs
 * it. It exists because the mechanism is the whole of being interruptible and
 * the unit tests only prove the bookkeeping: that a flag is set and a queue
 * stops filling. Whether the sound in the room stops is a different question,
 * and the answer is either "within a fraction of a second" or "at the end of
 * the sentence", which is the difference between a conversation and a lecture.
 */
func TestInterruptingReallyStopsTheVoice(t *testing.T) {
	if os.Getenv("PN_SCRIPTS_ASSISTANT_LIVE_VOICE") == "" {
		t.Skip("set PN_SCRIPTS_ASSISTANT_LIVE_VOICE=1 to hear this one")
	}

	if Available() == nil {
		t.Skip("no voice installed here")
	}

	ClearInterrupt()

	defer ClearInterrupt()

	// Long enough that finishing it would take many seconds, so the two
	// outcomes cannot be confused.
	const long = "This is a long sentence intended to run for a good while, " +
		"so that stopping it early is obvious, and it carries on for some " +
		"time yet, with several more clauses after this one, and then some."

	done := make(chan error, 1)
	started := time.Now()

	go func() { done <- SpeakAndWait(context.Background(), long) }()

	// Let it get going, then cut in.
	time.Sleep(1500 * time.Millisecond)

	if !Speaking() {
		t.Skip("the voice never started; nothing to interrupt")
	}

	Interrupt()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the voice was still going three seconds after being interrupted")
	}

	took := time.Since(started)

	// Interrupted at 1.5s, so anything near the full utterance means it
	// finished the sentence instead of stopping.
	if took > 4*time.Second {
		t.Errorf("took %s to stop, which is the sentence finishing rather than stopping", took)
	}

	t.Logf("stopped %s after being interrupted (utterance began %s earlier)",
		took-1500*time.Millisecond, took)
}
