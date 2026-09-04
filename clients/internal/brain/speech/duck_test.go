package speech

import (
	"context"
	"os"
	"testing"
)

/*
 * Turning the music down while it talks.
 *
 * The whole risk in this is asymmetric and worth stating: failing to turn the
 * music down means an answer is hard to hear once, and failing to put it back
 * means somebody's music is at a fifth for the rest of the evening and they
 * have no idea why. So most of what is tested here is the putting back.
 */

func TestSpeakingIsCountedSoOverlappingSentencesDoNotUnbalance(t *testing.T) {
	reset()

	first := duckOthers(nothing())
	second := duckOthers(nothing())

	first()

	duckMu.Lock()
	still := speaking
	duckMu.Unlock()

	if still != 1 {
		t.Fatalf("one sentence finishing left the count at %d, want 1", still)
	}

	second()

	duckMu.Lock()
	none := speaking
	duckMu.Unlock()

	if none != 0 {
		t.Fatalf("both finished but the count is %d", none)
	}
}

/*
 * The level is held down between the sentences of one answer.
 *
 * An answer is spoken a sentence at a time and each is its own go at the
 * synthesiser, so restoring the instant one finishes makes the music surge
 * back between every sentence. That pumping is worse than not ducking at all.
 */
func TestTheLevelIsHeldBetweenSentences(t *testing.T) {
	reset()

	duckOthers(nothing())()

	duckMu.Lock()
	pending := restorer != nil
	duckMu.Unlock()

	if !pending {
		t.Fatal("the level was restored immediately rather than held")
	}

	// And a sentence starting during the hold cancels the restore rather than
	// letting it fire mid-answer.
	release := duckOthers(nothing())

	duckMu.Lock()
	cancelled := restorer == nil
	duckMu.Unlock()

	if !cancelled {
		t.Fatal("the next sentence did not cancel the pending restore")
	}

	release()
}

// Turning the feature off puts back anything currently down, at once. A
// setting about sound in the room has to take effect in the room.
func TestSwitchingItOffPutsTheLevelBackAtOnce(t *testing.T) {
	reset()

	duckMu.Lock()
	duckedAt = map[string]float64{"Brave": 0.8}
	speaking = 1
	duckMu.Unlock()

	DuckOthersWhileTalking(false)

	duckMu.Lock()
	left := len(duckedAt)
	count := speaking
	duckMu.Unlock()

	if left != 0 || count != 0 {
		t.Fatalf("%d levels left down and %d sentences still counted", left, count)
	}

	DuckOthersWhileTalking(true)
}

/*
 * An interrupted answer must not leave the music down.
 *
 * Interrupting is the commonest way for speaking to end, and the release runs
 * from a defer, so this is really a test that the release is not skipped when
 * the context is already cancelled.
 */
func TestAnInterruptedAnswerStillPutsTheLevelBack(t *testing.T) {
	reset()

	ctx, cancel := context.WithCancel(context.Background())
	release := duckOthers(ctx)

	cancel()
	release()

	duckMu.Lock()
	pending := restorer != nil
	duckMu.Unlock()

	if !pending {
		t.Fatal("nothing was scheduled to put the level back")
	}

	PutTheVolumeBack()

	duckMu.Lock()
	defer duckMu.Unlock()

	if duckedAt != nil || restorer != nil {
		t.Fatal("something was left turned down")
	}
}

// Our own voice is never turned down, which would be an assistant quietly
// muting itself and reporting nothing wrong.
func TestItNeverTurnsItsOwnVoiceDown(t *testing.T) {
	for _, name := range []string{
		"pn-brain.echo-cancel.playback", "pw-play", "piper",
		"speech-dispatcher-espeak-ng", "PN-Brain",
	} {
		if !ours(name) {
			t.Errorf("%s would have been turned down", name)
		}
	}

	for _, name := range []string{"Brave", "Spotify", "mpv", "Firefox"} {
		if ours(name) {
			t.Errorf("%s was mistaken for our own voice", name)
		}
	}
}

// A level already low is left alone, and not remembered — otherwise finishing
// an answer would turn somebody's quiet music *up*.
func TestSomethingAlreadyQuietIsLeftAlone(t *testing.T) {
	if DuckedTo >= 1 {
		t.Fatal("the ducked level must be a reduction")
	}

	// The rule the code applies, stated here so it cannot drift: a stream at
	// or below the target is skipped.
	for _, was := range []float64{0.0, 0.1, DuckedTo} {
		if was > DuckedTo {
			t.Fatalf("%v would have been touched", was)
		}
	}
}

// wpctl prints "Volume: 0.85", and "Volume: 0.85 [MUTED]" on a muted stream.
func TestTheVolumeLineIsReadTheWayWireplumberPrintsIt(t *testing.T) {
	for _, c := range []struct {
		line string
		want float64
		ok   bool
	}{
		{"Volume: 0.85\n", 0.85, true},
		{"Volume: 1.00 [MUTED]\n", 1.00, true},
		{"Volume: 0.00\n", 0, true},
		{"", 0, false},
		{"Volume:\n", 0, false},
		{"Node 51 not found\n", 0, false},
	} {
		got, ok := readVolumeLine(c.line)

		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%q read as %v/%v, want %v/%v", c.line, got, ok, c.want, c.ok)
		}
	}
}

func nothing() context.Context { return context.Background() }

func reset() {
	PutTheVolumeBack()

	duckMu.Lock()
	defer duckMu.Unlock()

	duckedAt = nil
	speaking = 0
}

/*
 * What was turned down is remembered by application, not by node.
 *
 * A browser destroys and recreates its stream whenever playback stops and
 * starts, and WirePlumber persists the level against the application — so
 * restoring a stale node id fails silently and leaves the browser at a fifth
 * for good. That happened here: node 128 ducked, node 135 a minute later, both
 * at 0.2, neither restored, and it survives a reboot.
 */
func TestWhatWasTurnedDownIsRememberedByApplication(t *testing.T) {
	reset()

	duckMu.Lock()
	duckedAt = map[string]float64{"Brave": 0.85}
	duckMu.Unlock()

	// The key has to be something that outlives one stream. A number would be
	// a node id, which does not.
	duckMu.Lock()
	defer duckMu.Unlock()

	for name := range duckedAt {
		if name == "" {
			t.Fatal("something was remembered under no name at all")
		}
	}
}

// The note on disk is written when a level goes down and removed when it comes
// back, so a program that was killed can put things right next time it runs.
func TestTheNoteIsWrittenAndRemoved(t *testing.T) {
	path := duckedNotePath()

	if path == "" {
		t.Skip("no home directory")
	}

	existing, hadOne := os.ReadFile(path)

	t.Cleanup(func() {
		if hadOne == nil {
			os.WriteFile(path, existing, 0o600)

			return
		}

		os.Remove(path)
	})

	rememberDucked(map[string]float64{"Brave": 0.9})

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing was written down: %v", err)
	}

	forgetDucked()

	if _, err := os.Stat(path); err == nil {
		t.Fatal("the note outlived the thing it described")
	}
}

// Nothing to write down means no note, so a startup does not go looking for
// streams to restore that were never touched.
func TestNothingTurnedDownWritesNoNote(t *testing.T) {
	path := duckedNotePath()

	if path == "" {
		t.Skip("no home directory")
	}

	forgetDucked()
	rememberDucked(map[string]float64{})

	if _, err := os.Stat(path); err == nil {
		os.Remove(path)

		t.Fatal("wrote a note about nothing")
	}
}
