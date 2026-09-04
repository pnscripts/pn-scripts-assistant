package pace

import (
	"testing"
	"time"
)

/*
 * Timing an answer, stage by stage.
 *
 * The arithmetic looks trivial and is the whole value of the package: if the
 * stages are measured from the wrong instants, the numbers still look
 * plausible and point at the wrong part of the program. The first version of
 * this measured from when the model was asked, which made every spoken turn
 * look fast and hid both stages the program is actually responsible for.
 */

// A turn assembled at known instants, so the expected numbers are arithmetic
// rather than opinion.
func madeUp(spoken bool) Turn {
	at := time.Now().Add(-time.Minute)

	return Turn{
		Ended:         at,
		Transcribed:   at.Add(600 * time.Millisecond),
		Asked:         at.Add(650 * time.Millisecond),
		FirstToken:    at.Add(4 * time.Second),
		FirstSentence: at.Add(5 * time.Second),
		FirstSound:    at.Add(6 * time.Second),
		Finished:      at.Add(9 * time.Second),
		Spoken:        spoken,
		Model:         "qwen3:8b",
		Tools:         true,
		Words:         7,
	}
}

func TestEachWaitIsMeasuredFromTheOneBeforeIt(t *testing.T) {
	m := madeUp(true).Milestones()

	for _, check := range []struct {
		what string
		got  int
		want int
	}{
		{"hearing", m.Hearing, 600},
		{"thinking", m.Thinking, 3400},
		{"writing", m.Writing, 1000},
		{"speaking", m.Speaking, 1000},
		{"to the first sound", m.ToFirst, 6000},
		{"altogether", m.Altogether, 9000},
	} {
		if check.got != check.want {
			t.Errorf("%s: %dms, want %dms", check.what, check.got, check.want)
		}
	}
}

/*
 * The clock starts when somebody stops talking, not when the model is asked.
 *
 * This is the point of the whole package. Six seconds passed before a sound
 * came out; timing from the model would have called it 2.6 and left the two
 * seconds that belong to the program — waiting out the silence, starting a
 * synthesiser — invisible.
 */
func TestTheWaitIncludesTheStagesBeforeTheModel(t *testing.T) {
	m := madeUp(true).Milestones()

	if m.ToFirst <= m.Thinking+m.Writing {
		t.Fatalf("the wait (%dms) leaves out the stages around the model", m.ToFirst)
	}

	if m.Hearing+m.Speaking != 1600 {
		t.Fatalf("the program's own share came to %dms, want 1600",
			m.Hearing+m.Speaking)
	}
}

// A turn that stopped early still describes what happened up to that point:
// heard, understood, and then not answered.
func TestAnUnfinishedTurnStillReportsWhatHappened(t *testing.T) {
	at := time.Now()

	m := Turn{Ended: at, Transcribed: at.Add(700 * time.Millisecond), Spoken: true}.Milestones()

	if m.Hearing != 700 {
		t.Fatalf("hearing was %dms, want 700", m.Hearing)
	}

	for _, later := range []int{m.Writing, m.Speaking, m.Altogether} {
		if later != 0 {
			t.Fatalf("a stage that never happened was reported as %dms", later)
		}
	}
}

// Nothing at all, which is what a turn looks like before it starts.
func TestAnEmptyTurnIsAllZeroes(t *testing.T) {
	m := Turn{}.Milestones()

	if m.Hearing+m.Thinking+m.Writing+m.Speaking+m.ToFirst+m.Altogether != 0 {
		t.Fatalf("an empty turn reported timings: %+v", m)
	}
}

// Clocks go backwards — a machine waking from sleep, a time correction — and a
// negative wait is worse than no number, because it looks like a fault in the
// stage rather than in the clock.
func TestATimeGoingBackwardsIsNotReportedAsNegative(t *testing.T) {
	at := time.Now()

	m := Turn{Ended: at, Transcribed: at.Add(-2 * time.Second)}.Milestones()

	if m.Hearing != 0 {
		t.Fatalf("a backwards clock produced %dms", m.Hearing)
	}
}

func TestTheStagesAreRecordedInOrderAndOnlyOnce(t *testing.T) {
	Forget()

	Begin(true, time.Now().Add(-2*time.Second))
	Transcribed(4)
	FirstToken()
	FirstToken() // a second tool round; the wait being measured is the first
	FirstSentence()
	FirstSound()
	Finish()

	recent := Recent()

	if len(recent) != 1 {
		t.Fatalf("expected one turn kept, got %d", len(recent))
	}

	m := recent[0].Milestones()

	if m.Words != 4 {
		t.Fatalf("words said: %d, want 4", m.Words)
	}

	if m.ToFirst < 1900 {
		t.Fatalf("the wait was measured as %dms, but the turn began 2s ago", m.ToFirst)
	}
}

/*
 * A turn abandoned halfway is dropped rather than kept.
 *
 * Interrupting is the common way for that to happen, and an interrupted turn's
 * timings say how long somebody was willing to wait, not how long the machine
 * took. Averaged in with the rest they would make the program look faster
 * exactly when it was being too slow to sit through.
 */
func TestAnAbandonedTurnIsNotKept(t *testing.T) {
	Forget()

	Begin(true, time.Now())
	Transcribed(3)

	// Somebody cut in: a new turn starts with the old one unfinished.
	Begin(true, time.Now())
	Transcribed(5)
	FirstSound()
	Finish()

	recent := Recent()

	if len(recent) != 1 {
		t.Fatalf("expected only the finished turn, got %d", len(recent))
	}

	if recent[0].Words != 5 {
		t.Fatal("the abandoned turn was the one kept")
	}
}

// Marking a stage when no turn is running must not panic or invent one — the
// speaking path also says the greeting, which belongs to no turn at all.
func TestMarkingWithNoTurnRunningIsHarmless(t *testing.T) {
	Forget()

	FirstSound()
	FirstToken()
	Transcribed(2)
	Finish()

	if len(Recent()) != 0 {
		t.Fatal("a turn was invented out of a stray mark")
	}
}

// Only so many are kept: this is a diagnostic, not a record of somebody's day.
func TestOnlyTheRecentTurnsAreKept(t *testing.T) {
	Forget()

	for i := 0; i < Kept+20; i++ {
		Begin(true, time.Now())
		FirstSound()
		Finish()
	}

	if got := len(Recent()); got != Kept {
		t.Fatalf("kept %d turns, want %d", got, Kept)
	}
}

/*
 * The typical turn is the median, and only of spoken ones.
 *
 * One turn that waited on a tool, or on a model being read off the disk, moves
 * a mean by more than everything else put together — and a typed turn has no
 * microphone and no voice in it, so averaging the two describes neither.
 */
func TestTheTypicalTurnIgnoresTypedOnesAndOutliers(t *testing.T) {
	Forget()

	add := func(spoken bool, wait time.Duration) {
		at := time.Now().Add(-wait)

		mu.Lock()
		turns = append(turns, Turn{
			Ended: at, FirstSound: at.Add(wait), Finished: at.Add(wait), Spoken: spoken,
		})
		mu.Unlock()
	}

	add(true, 2*time.Second)
	add(true, 3*time.Second)
	add(true, 4*time.Second)
	add(true, 90*time.Second) // one that went off to use a tool
	add(false, 1*time.Millisecond)

	typical, over := Typical()

	if over != 4 {
		t.Fatalf("averaged over %d turns, want the 4 spoken ones", over)
	}

	if typical.ToFirst < 3000 || typical.ToFirst > 4000 {
		t.Fatalf("the typical wait came out at %dms, which the outlier moved",
			typical.ToFirst)
	}
}

func TestNothingMeasuredYetSaysSo(t *testing.T) {
	Forget()

	typical, over := Typical()

	if over != 0 || typical.ToFirst != 0 {
		t.Fatalf("invented a timing from nothing: %+v over %d", typical, over)
	}
}

// Forgetting is what happens when a conversation is cleared, and it has to
// take the turn in progress with it.
func TestForgettingLeavesNothingBehind(t *testing.T) {
	Begin(true, time.Now())
	Transcribed(3)

	Forget()

	Finish()

	if len(Recent()) != 0 {
		t.Fatal("a turn survived being forgotten")
	}
}
