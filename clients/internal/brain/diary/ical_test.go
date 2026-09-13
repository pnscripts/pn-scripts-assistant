package diary

import (
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

const exported = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:abc-123\r\n" +
	"SUMMARY:Dentist\r\n" +
	"DTSTART:20260914T083000Z\r\n" +
	"DTEND:20260914T090000Z\r\n" +
	"LOCATION:Vitosha 12\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:abc-124\r\n" +
	"SUMMARY:Away\r\n" +
	"DTSTART;VALUE=DATE:20260920\r\n" +
	"DTEND;VALUE=DATE:20260922\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestEventsAreReadOutOfWhatCalendarsExport(t *testing.T) {
	events, err := Read(exported, "work.ics")
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("read %d events", len(events))
	}

	dentist := events[0]

	if dentist.Title != "Dentist" || dentist.Place != "Vitosha 12" {
		t.Errorf("read %+v", dentist)
	}

	if dentist.UID != "abc-123" {
		t.Errorf("the identity was lost: %q", dentist.UID)
	}

	if dentist.AllDay {
		t.Error("a half-hour appointment was read as a whole day")
	}

	if got := dentist.Length(); got != 30*time.Minute {
		t.Errorf("it runs for %v", got)
	}

	// A bare date is a day somebody has taken, not an hour they have booked.
	away := events[1]

	if !away.AllDay {
		t.Error("a date with no time was not read as a whole day")
	}

	if away.CameFrom != "work.ics" {
		t.Errorf("it does not say where it came from: %q", away.CameFrom)
	}
}

/*
 * Continued lines are put back together.
 *
 * iCalendar wraps at 75 characters and continues with a leading space, so a
 * description of any length arrives in pieces. Read as separate lines, most of
 * every long field is lost — and lost quietly, which is the worst kind.
 */
func TestWrappedLinesAreReadWhole(t *testing.T) {
	wrapped := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" +
		"SUMMARY:Quarterly review with the whole team about what happened and wha\r\n" +
		" t happens next\r\n" +
		"DTSTART:20260914T090000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"

	events, err := Read(wrapped, "")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(events[0].Title, "what happens next") {
		t.Errorf("the wrapped line was not rejoined: %q", events[0].Title)
	}
}

// An event with no end is legal and common. Left at zero it would land at the
// start of 1970 and look like a parsing bug rather than a missing field.
func TestAnEventWithNoEndStillHasOne(t *testing.T) {
	events, err := Read("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Call\r\n"+
		"DTSTART:20260914T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", "")
	if err != nil {
		t.Fatal(err)
	}

	if got := events[0].Length(); got != time.Hour {
		t.Errorf("an event with no end runs for %v, want an hour", got)
	}
}

// Escaped commas and newlines come back as themselves.
func TestEscapedTextComesBackAsItself(t *testing.T) {
	events, err := Read("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n"+
		`SUMMARY:Lunch\, then the bank`+"\r\n"+
		`DESCRIPTION:Bring:\n - the folder\n - the keys`+"\r\n"+
		"DTSTART:20260914T120000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", "")
	if err != nil {
		t.Fatal(err)
	}

	if events[0].Title != "Lunch, then the bank" {
		t.Errorf("title came back as %q", events[0].Title)
	}

	if !strings.Contains(events[0].Notes, "\n - the keys") {
		t.Errorf("notes came back as %q", events[0].Notes)
	}
}

/*
 * What goes out comes back the same.
 *
 * The round trip is the whole of the interop story: there is no account and no
 * sync, so a file that cannot be read back by the thing that wrote it would
 * mean the diary is a one-way door.
 */
func TestWhatGoesOutComesBackTheSame(t *testing.T) {
	at := time.Date(2026, 9, 14, 8, 30, 0, 0, time.Local)

	out := []store.Event{
		{ID: 1, Title: "Dentist", Starts: at, Ends: at.Add(30 * time.Minute), Place: "Vitosha 12"},
		{ID: 2, Title: "Away", Starts: at.AddDate(0, 0, 6), Ends: at.AddDate(0, 0, 8), AllDay: true},
	}

	back, err := Read(Write(out), "")
	if err != nil {
		t.Fatal(err)
	}

	if len(back) != 2 {
		t.Fatalf("wrote 2 events and read %d back", len(back))
	}

	if back[0].Title != "Dentist" || !back[0].Starts.Equal(at) {
		t.Errorf("the appointment came back as %+v", back[0])
	}

	if !back[1].AllDay {
		t.Error("a whole day came back as an hour")
	}

	// Every event leaves with an identity, so re-importing updates rather
	// than doubling.
	if back[0].UID == "" {
		t.Error("an exported event has no identity to match on")
	}
}

// A file with nothing in it says so, rather than quietly importing nothing.
func TestAFileWithNoEventsSaysSo(t *testing.T) {
	if _, err := Read("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n", ""); err == nil {
		t.Error("an empty calendar was accepted silently")
	}

	if _, err := Read("this is not a calendar at all", ""); err == nil {
		t.Error("a text file was accepted as a calendar")
	}
}
