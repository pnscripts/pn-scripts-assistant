package store

import (
	"testing"
	"time"
)

func TestSomethingPutInTheDiaryComesBack(t *testing.T) {
	db := open(t)

	at := time.Date(2026, 9, 14, 8, 30, 0, 0, time.Local)

	id, err := db.PutInTheDiary(Event{
		Title: "Dentist", Starts: at, Ends: at.Add(30 * time.Minute), Place: "Vitosha 12",
	})
	if err != nil {
		t.Fatal(err)
	}

	back, err := db.Event(id)
	if err != nil || back == nil {
		t.Fatalf("reading it back: %v %v", back, err)
	}

	if back.Title != "Dentist" || back.Place != "Vitosha 12" {
		t.Errorf("came back as %+v", back)
	}

	if !back.Starts.Equal(at) {
		t.Errorf("it starts at %v, want %v", back.Starts, at)
	}
}

/*
 * What is on today includes what started yesterday and has not finished.
 *
 * Overlapping rather than starting inside the window, which is the difference
 * between "what is on today" and "what starts today". Something running until
 * Friday is very much on today, and a diary that hid it would be answering a
 * question nobody asked.
 */
func TestWhatIsOnIncludesWhatIsStillRunning(t *testing.T) {
	db := open(t)

	today := time.Date(2026, 9, 14, 0, 0, 0, 0, time.Local)

	if _, err := db.PutInTheDiary(Event{
		Title:  "Away all week",
		Starts: today.AddDate(0, 0, -2),
		Ends:   today.AddDate(0, 0, 3),
		AllDay: true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := db.PutInTheDiary(Event{
		Title: "Last month", Starts: today.AddDate(0, -1, 0), Ends: today.AddDate(0, -1, 0).Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	on, err := db.WhatIsOn(today, today.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}

	if len(on) != 1 || on[0].Title != "Away all week" {
		t.Fatalf("today's diary is %+v", on)
	}
}

/*
 * The same file imported twice is one diary, not two.
 *
 * Without matching on the identity, a second import is a diary with everything
 * in it twice and no way to tell which copy to delete.
 */
func TestImportingTheSameThingTwiceDoesNotDoubleIt(t *testing.T) {
	db := open(t)

	at := time.Now()

	event := Event{UID: "abc-123", Title: "Dentist", Starts: at, Ends: at.Add(time.Hour)}

	first, err := db.PutInTheDiary(event)
	if err != nil {
		t.Fatal(err)
	}

	// The same event, moved an hour later, as an updated export would have it.
	event.Starts = at.Add(time.Hour)
	event.Ends = at.Add(2 * time.Hour)

	second, err := db.PutInTheDiary(event)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Errorf("the same event was written twice, as %d and %d", first, second)
	}

	n, _ := db.CountEvents()

	if n != 1 {
		t.Errorf("the diary holds %d events", n)
	}

	// And the update took.
	back, _ := db.Event(first)

	if !back.Starts.Equal(at.Add(time.Hour).Truncate(time.Second).Local()) &&
		back.Starts.Sub(at.Add(time.Hour)).Abs() > time.Second {
		t.Errorf("it did not move: %v", back.Starts)
	}
}

// Two events somebody typed are two events, even with the same name — only a
// shared identity from a file means they are the same thing.
func TestTwoThingsWithTheSameNameAreTwoThings(t *testing.T) {
	db := open(t)

	at := time.Now()

	if _, err := db.PutInTheDiary(Event{Title: "Standup", Starts: at, Ends: at.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	if _, err := db.PutInTheDiary(Event{
		Title: "Standup", Starts: at.AddDate(0, 0, 1), Ends: at.AddDate(0, 0, 1).Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	n, _ := db.CountEvents()

	if n != 2 {
		t.Errorf("the diary holds %d events, want 2", n)
	}
}

// Nonsense is refused rather than written, because an appointment at the wrong
// time is worse than one that was never made.
func TestNonsenseIsRefused(t *testing.T) {
	db := open(t)

	at := time.Now()

	if _, err := db.PutInTheDiary(Event{Title: "  ", Starts: at, Ends: at}); err == nil {
		t.Error("an event with no name was written")
	}

	if _, err := db.PutInTheDiary(Event{
		Title: "Backwards", Starts: at, Ends: at.Add(-time.Hour),
	}); err == nil {
		t.Error("an event that ends before it starts was written")
	}
}

/*
 * Exporting the diary and reading the file straight back in changes nothing.
 *
 * The round trip is the whole interop story — there is no account and no sync
 * — and it was broken in a way that only showed up by doing it: events that
 * arrived in a file matched on their identity and updated, while events
 * somebody had typed were given a fresh identity every time they were written
 * out, so each export-and-import made a second copy of every one of them.
 */
func TestExportingAndReadingBackChangesNothing(t *testing.T) {
	db := open(t)

	at := time.Now()

	// One that came from a file, and one somebody typed.
	if _, err := db.PutInTheDiary(Event{
		UID: "x1", Title: "Dentist", Starts: at, Ends: at.Add(time.Hour), CameFrom: "work.ics",
	}); err != nil {
		t.Fatal(err)
	}

	typed, err := db.PutInTheDiary(Event{
		Title: "Coffee with Anna", Starts: at.AddDate(0, 0, 1), Ends: at.AddDate(0, 0, 1).Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Everything this program writes out carries an identity that is kept.
	mine, _ := db.Event(typed)

	if mine.UID == "" {
		t.Fatal("an event typed in here has no identity, so exporting it loses track of it")
	}

	// The same events again, as a re-import of our own export would present
	// them: same identities, one of them moved.
	everything, _ := db.WhatIsOn(at.AddDate(0, 0, -1), at.AddDate(0, 0, 30))

	for _, e := range everything {
		e.Starts = e.Starts.Add(time.Hour)
		e.Ends = e.Ends.Add(time.Hour)

		if _, err := db.PutInTheDiary(e); err != nil {
			t.Fatal(err)
		}
	}

	n, _ := db.CountEvents()

	if n != 2 {
		t.Errorf("after exporting and reading back, the diary holds %d events, want 2", n)
	}
}
