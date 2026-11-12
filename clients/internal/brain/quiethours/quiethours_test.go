package quiethours

import (
	"strings"
	"testing"
	"time"
)

func at(t *testing.T, loc *time.Location, s string) time.Time {
	t.Helper()

	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}

	return v
}

func TestAWindowIsReadTheWayPeopleWriteIt(t *testing.T) {
	cases := map[string]string{
		"22:00-07:00":    "22:00-07:00",
		" 22:00 - 7:00 ": "22:00-07:00",
		"01:30-05:00":    "01:30-05:00",
		"23:00 – 06:30":  "23:00-06:30",
		"9:05-17:45":     "09:05-17:45",
		"00:00-23:59":    "00:00-23:59",
	}

	for in, want := range cases {
		w, err := Parse(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}

		if !w.Set() || w.String() != want {
			t.Errorf("%q read as %q (set %v), want %q", in, w.String(), w.Set(), want)
		}
	}
}

func TestNoWindowIsNoWindow(t *testing.T) {
	w, err := Parse("  ")
	if err != nil || w.Set() || w.String() != "" {
		t.Fatalf("empty: %v %v %q", err, w.Set(), w.String())
	}

	// With no window nothing is ever held, however fast the allowance goes.
	if held, _ := Hold(w, true, time.Now()); held {
		t.Fatal("held with no quiet hours set")
	}
}

func TestAWindowThatCannotBeMeantIsRefused(t *testing.T) {
	for _, in := range []string{"22:00", "22-07", "25:00-07:00", "22:60-07:00", "22:00-22:00",
		"ten-seven", "22:00-07:00-08:00", "-1:00-07:00", "22:00-07:00x", "2200-0700"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestAWindowAcrossMidnightHoldsBothSides(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	w, _ := Parse("22:00-07:00")

	in := []string{"2026-11-11 22:00", "2026-11-11 23:59", "2026-11-12 00:00", "2026-11-12 06:59"}
	out := []string{"2026-11-11 21:59", "2026-11-12 07:00", "2026-11-12 12:00"}

	for _, s := range in {
		if !w.Contains(at(t, loc, s)) {
			t.Errorf("%s should be inside 22:00-07:00", s)
		}
	}

	for _, s := range out {
		if w.Contains(at(t, loc, s)) {
			t.Errorf("%s should be outside 22:00-07:00", s)
		}
	}
}

func TestAWindowWithinOneDay(t *testing.T) {
	loc := time.UTC
	w, _ := Parse("01:30-05:00")

	if !w.Contains(at(t, loc, "2026-11-12 01:30")) || !w.Contains(at(t, loc, "2026-11-12 04:59")) {
		t.Error("inside not inside")
	}

	if w.Contains(at(t, loc, "2026-11-12 05:00")) || w.Contains(at(t, loc, "2026-11-12 01:29")) ||
		w.Contains(at(t, loc, "2026-11-12 23:00")) {
		t.Error("outside counted inside")
	}
}

func TestTheNextOpeningIsTodayOrTomorrow(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	w, _ := Parse("22:00-07:00")

	if got := w.Opens(at(t, loc, "2026-11-11 15:10")); !got.Equal(at(t, loc, "2026-11-11 22:00")) {
		t.Errorf("afternoon: opens %v", got)
	}

	// Inside the window it is open now.
	now := at(t, loc, "2026-11-12 03:00")
	if got := w.Opens(now); !got.Equal(now) {
		t.Errorf("inside: opens %v, want now", got)
	}

	w2, _ := Parse("01:30-05:00")
	if got := w2.Opens(at(t, loc, "2026-11-11 05:00")); !got.Equal(at(t, loc, "2026-11-12 01:30")) {
		t.Errorf("after the window: opens %v, want tomorrow 01:30", got)
	}
}

func TestTheOpeningFollowsTheClockOverASummerTimeChange(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		t.Skip("no time zone data:", err)
	}

	// 2026-10-25 04:00 Sofia goes back to 03:00: the day has 25 hours.
	w, _ := Parse("22:00-07:00")
	got := w.Opens(at(t, loc, "2026-10-25 12:00"))

	if got.Hour() != 22 || got.Minute() != 0 || got.Day() != 25 {
		t.Errorf("opens %v, want 22:00 on the 25th by the wall clock", got)
	}
}

func TestOnlyWorkAheadOfItsAllowanceIsHeldAndOnlyOutsideTheWindow(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	w, _ := Parse("22:00-07:00")
	day := at(t, loc, "2026-11-11 15:10")
	night := at(t, loc, "2026-11-11 23:10")

	if held, _ := Hold(w, false, day); held {
		t.Error("an allowance on pace was held")
	}

	if held, _ := Hold(w, true, night); held {
		t.Error("held inside the quiet hours")
	}

	held, opens := Hold(w, true, day)
	if !held || !opens.Equal(at(t, loc, "2026-11-11 22:00")) {
		t.Fatalf("ahead of pace in the day: held %v until %v", held, opens)
	}

	s := Say(w, opens, day)
	for _, part := range []string{"22:00 to 07:00", "22:00 today", "start it now anyway"} {
		if !strings.Contains(s, part) {
			t.Errorf("%q does not say %q", s, part)
		}
	}

	if strings.ContainsAny(s, "–—") {
		t.Errorf("%q has a dash", s)
	}

	if s := Say(w, at(t, loc, "2026-11-12 22:00"), day); !strings.Contains(s, "22:00 tomorrow") {
		t.Errorf("%q does not say tomorrow", s)
	}
}

// Held is the whole question in one call, from the saved setting.
func TestHeldReadsTheSavedSetting(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	day := at(t, loc, "2026-11-11 15:10")

	if s := Held("22:00-07:00", true, day); !strings.Contains(s, "waits until 22:00 today") {
		t.Errorf("%q", s)
	}

	for _, setting := range []string{"", "nonsense"} {
		if s := Held(setting, true, day); s != "" {
			t.Errorf("%q held work: %q", setting, s)
		}
	}

	if s := Held("22:00-07:00", false, day); s != "" {
		t.Errorf("on pace held: %q", s)
	}
}

// Work nobody asked for at that moment (a goal that starts itself, reading a
// place) has nobody to say yes, so outside the window it waits for it,
// whatever the allowance, and starts in it.
func TestUnaskedWorkWaitsForTheWindowAndStartsInIt(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)

	waits, opens := Waits("22:00-07:00", at(t, loc, "2026-11-11 15:10"))
	if !waits || !opens.Equal(at(t, loc, "2026-11-11 22:00")) {
		t.Fatalf("afternoon: waits %v until %v, want until 22:00 today", waits, opens)
	}

	for _, s := range []string{"2026-11-11 22:00", "2026-11-12 03:00", "2026-11-12 06:59"} {
		if waits, _ := Waits("22:00-07:00", at(t, loc, s)); waits {
			t.Errorf("%s is inside the window and still waits", s)
		}
	}

	// The minute the window closes, the next opening is the same evening.
	waits, opens = Waits("22:00-07:00", at(t, loc, "2026-11-12 07:00"))
	if !waits || !opens.Equal(at(t, loc, "2026-11-12 22:00")) {
		t.Errorf("at 07:00: waits %v until %v", waits, opens)
	}

	// No window, or one that cannot be read, holds nothing back.
	for _, setting := range []string{"", "  ", "nonsense", "22:00-22:00"} {
		if waits, _ := Waits(setting, at(t, loc, "2026-11-11 15:10")); waits {
			t.Errorf("%q made work wait", setting)
		}
	}
}

// A loop that checks now and then wakes at the opening, not up to a whole
// round after it: "starts in it by itself" means at the start of the window.
func TestALoopWakesWhenTheWindowOpens(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	round := 10 * time.Minute

	if got := Sleep("22:00-07:00", at(t, loc, "2026-11-11 21:55"), round); got != 5*time.Minute {
		t.Errorf("five minutes before the opening: sleeps %v", got)
	}

	if got := Sleep("22:00-07:00", at(t, loc, "2026-11-11 15:10"), round); got != round {
		t.Errorf("hours before: sleeps %v, want a whole round", got)
	}

	if got := Sleep("22:00-07:00", at(t, loc, "2026-11-11 23:00"), round); got != round {
		t.Errorf("inside: sleeps %v, want a whole round", got)
	}

	if got := Sleep("", at(t, loc, "2026-11-11 21:55"), round); got != round {
		t.Errorf("no window: sleeps %v", got)
	}

	// Seconds before the opening it still sleeps a moment, never zero or
	// less, or the loop would spin.
	almost := at(t, loc, "2026-11-11 22:00").Add(-200 * time.Millisecond)
	if got := Sleep("22:00-07:00", almost, round); got < time.Second || got > 2*time.Second {
		t.Errorf("a moment before: sleeps %v", got)
	}
}

// The sentence for work that waits says the window, when it starts and that
// it can be started at once.
func TestWaitingIsSaidWithTheWayToStartItNow(t *testing.T) {
	loc := time.FixedZone("here", 2*3600)
	day := at(t, loc, "2026-11-11 15:10")

	s := Waiting("22:00-07:00", day)
	for _, part := range []string{"22:00 to 07:00", "22:00 today", "by itself"} {
		if !strings.Contains(s, part) {
			t.Errorf("%q does not say %q", s, part)
		}
	}

	if strings.ContainsAny(s, "–—") {
		t.Errorf("%q has a dash", s)
	}

	if s := Waiting("22:00-07:00", at(t, loc, "2026-11-11 23:00")); s != "" {
		t.Errorf("inside the window: %q", s)
	}

	if s := Waiting("01:30-05:00", at(t, loc, "2026-11-11 06:00")); !strings.Contains(s, "01:30 tomorrow") {
		t.Errorf("after a morning window: %q", s)
	}
}

/*
 * Held questions answer themselves only while a window is open: no window,
 * one that cannot be read, or a shut one answers nothing.
 */
func TestHeldQuestionsAnswerThemselvesOnlyInAnOpenWindow(t *testing.T) {
	loc := time.UTC

	cases := []struct {
		setting, now string
		open         bool
	}{
		{"", "2026-11-12 23:00", false},
		{"not a window", "2026-11-12 23:00", false},
		{"22:00-22:00", "2026-11-12 22:00", false},
		{"22:00-07:00", "2026-11-12 21:59", false},
		{"22:00-07:00", "2026-11-12 22:00", true},
		{"22:00-07:00", "2026-11-13 06:59", true},
		{"22:00-07:00", "2026-11-13 07:00", false},
		{"09:00-17:00", "2026-11-12 12:00", true},
		{"09:00-17:00", "2026-11-12 17:30", false},
	}

	for _, c := range cases {
		if got := OpenNow(c.setting, at(t, loc, c.now)); got != c.open {
			t.Errorf("%q at %s: open %v, want %v", c.setting, c.now, got, c.open)
		}
	}
}

// The held sentence no longer tells its user to ask again: whether it
// starts by itself is said beside it, by whoever knows.
func TestTheHeldSentenceLeavesWhatHappensThenToItsCaller(t *testing.T) {
	loc := time.UTC
	w, _ := Parse("22:00-07:00")
	now := at(t, loc, "2026-11-12 15:00")

	s := Say(w, w.Opens(now), now)
	if strings.Contains(s, "Ask again") || !strings.HasSuffix(s, "waits until 22:00 today. Say yes to start it now anyway.") {
		t.Errorf("%q", s)
	}

	if strings.ContainsAny(StartsItself+WaitsForYou, "–—") {
		t.Error("a dash in a sentence people read")
	}
}
