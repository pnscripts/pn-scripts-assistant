package speech

import (
	"testing"
	"time"
)

/*
 * A long stretch of speech that came back as almost nothing was cut off.
 *
 * Whisper answers a fragment with a full stop, or with whichever phrase it has
 * seen most often — "Thank you." — and both of those read as complete
 * sentences. Judged on the text alone the turn looks finished and is thrown
 * away; judged against five seconds of recorded speech it plainly is not,
 * because nobody talks for five seconds and says one word.
 *
 * This was the whole of "nothing is changing in the conversation": the words
 * never got there.
 */
func TestAFragmentIsRecognisedAsCutOff(t *testing.T) {
	for _, c := range []struct {
		text  string
		spoke int
	}{
		{".", 5300},
		{"Thank you.", 4000},
		{"...", 2600},
	} {
		if !NeedsMore(c.text, c.spoke) {
			t.Errorf("%q after %dms of speech was treated as a finished turn",
				c.text, c.spoke)
		}
	}

	// A genuinely short answer is not chased for more.
	for _, c := range []struct {
		text  string
		spoke int
	}{
		{"Yes.", 700},
		{"No.", 600},
		{"what is the weather in Sofia today?", 2800},
	} {
		if NeedsMore(c.text, c.spoke) {
			t.Errorf("%q after %dms was chased for more when it was complete",
				c.text, c.spoke)
		}
	}
}

/*
 * The wait before calling a turn over grows with how long somebody has talked.
 *
 * A fixed gap cannot be right for both. Someone asking a short question stops
 * and wants an answer; someone explaining something pauses mid-thought, and
 * those pauses lengthen the longer they have been going.
 */
func TestPatienceGrowsWithHowLongSomebodyHasBeenTalking(t *testing.T) {
	quick := patienceFor(900 * time.Millisecond)
	long := patienceFor(9 * time.Second)

	if quick != SilenceToEnd {
		t.Errorf("a short question waits %v, want %v", quick, SilenceToEnd)
	}

	if long <= quick {
		t.Errorf("a long answer waits %v, no more than a short one at %v", long, quick)
	}

	// And never so long that it reads as the program having stopped noticing.
	if long > 2500*time.Millisecond {
		t.Errorf("waits %v after a long answer, which reads as a hang", long)
	}
}
