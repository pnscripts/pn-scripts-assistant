package tools

import (
	"testing"
	"time"
)

/*
 * The words people actually say to an assistant.
 *
 * A reminder set for the wrong time is worse than one that was not set,
 * because its owner is relying on it — so anything not understood has to be
 * refused rather than guessed at.
 */
func TestReadingATimeOutOfWhatSomebodySaid(t *testing.T) {
	// A Wednesday evening.
	now := time.Date(2026, 8, 30, 19, 30, 0, 0, time.UTC)

	cases := []struct {
		said string
		want time.Time
	}{
		{"in 20 minutes", now.Add(20 * time.Minute)},
		{"in 2 hours", now.Add(2 * time.Hour)},
		{"in a minute", now.Add(time.Minute)},
		{"in 3 days", now.AddDate(0, 0, 3)},
		{"tomorrow at 9", time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)},
		{"tomorrow", time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)},
		{"at 21:15", time.Date(2026, 8, 30, 21, 15, 0, 0, time.UTC)},
		{"2026-09-01 08:00", time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)},
	}

	for _, c := range cases {
		got, err := ParseWhen(c.said, now)
		if err != nil {
			t.Errorf("%q was not understood: %v", c.said, err)

			continue
		}

		if !got.Equal(c.want) {
			t.Errorf("%q gave %s, want %s", c.said, got, c.want)
		}
	}
}

/*
 * An hour that has already gone means tomorrow.
 *
 * "Remind me at nine" said in the evening does not mean nine o'clock this
 * morning. Setting it in the past is the same as not setting it, except that
 * its owner believes it is set.
 */
func TestATimeAlreadyPastMeansTomorrow(t *testing.T) {
	evening := time.Date(2026, 8, 30, 19, 30, 0, 0, time.UTC)

	got, err := ParseWhen("at 9", evening)
	if err != nil {
		t.Fatal(err)
	}

	if !got.After(evening) {
		t.Errorf("a reminder was set for %s, which is already past", got)
	}

	if got.Day() != 31 {
		t.Errorf("expected tomorrow morning, got %s", got)
	}
}

// What it cannot read, it refuses rather than guessing.
func TestAnUnreadableTimeIsRefused(t *testing.T) {
	now := time.Now()

	for _, said := range []string{"", "soon", "later", "when I get back", "in a bit"} {
		if at, err := ParseWhen(said, now); err == nil {
			t.Errorf("%q was taken to mean %s", said, at)
		}
	}
}
