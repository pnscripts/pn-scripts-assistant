package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The diary, asked about and written to.
 *
 * Three tools rather than five. Reading it is Safe; putting something in and
 * changing something are Mutating, and the summary shows the whole event —
 * somebody approving "put something in the diary" has agreed to nothing, where
 * "Thursday 14:00–15:00, dentist" is a thing they can check.
 *
 * Importing a calendar file is deliberately not here. It is a rare, deliberate
 * act that belongs to a panel, and a tool for it would sit in the list on every
 * turn being not-chosen.
 */

// Calendar is what the diary tools need from the brain. Named for what it
// holds rather than for the drawer it is in: Diary in this package already
// means the reminders, which are a different thing wearing a similar hat.
type Calendar interface {
	WhatIsOn(from, to time.Time) ([]store.Event, error)
	PutInTheDiary(e store.Event) (int64, error)
	Event(id int64) (*store.Event, error)
	ChangeTheDiary(e store.Event) error
	CancelIt(id int64) error
}

type WhatIsOn struct {
	DB Calendar
}

func (WhatIsOn) Name() string { return "what_is_on" }

func (WhatIsOn) Description() string {
	return "Look in the diary: what is on today, tomorrow, this week, or between two " +
		"dates. Use when asked what is on, whether a time is free, or when something is."
}

func (WhatIsOn) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"from": {"type": "string", "description": "When to look from: today, tomorrow, a date like 2026-09-14, or a time. Empty means today."},
			"days": {"type": "integer", "description": "How many days to look at from there. Empty means one."}
		},
		"required": []
	}`)
}

func (WhatIsOn) Risk() Risk { return Safe }

type lookArgs struct {
	From string `json:"from"`
	Days int    `json:"days"`
}

func (WhatIsOn) Summarize(raw json.RawMessage) string {
	var a lookArgs

	json.Unmarshal(raw, &a)

	if a.From == "" {
		return "Look at today's diary"
	}

	return "Look at the diary from " + a.From
}

func (t WhatIsOn) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a lookArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to look at: %w", err)
	}

	now := time.Now()
	from := startOfDay(now)

	if a.From != "" {
		at, err := whenFrom(a.From, now)
		if err != nil {
			return "", err
		}

		from = startOfDay(at)
	}

	days := a.Days
	if days < 1 {
		days = 1
	}

	to := from.AddDate(0, 0, days)

	events, err := t.DB.WhatIsOn(from, to)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "Nothing in the diary " + spanWords(from, days) + ".", nil
	}

	var b strings.Builder

	b.WriteString(spanWords(from, days) + ":\n")

	for _, e := range events {
		b.WriteString("  " + line(e) + "\n")
	}

	return b.String(), nil
}

// line is one event as somebody would read it out.
func line(e store.Event) string {
	when := e.Starts.Format("Mon 2 Jan")

	if e.AllDay {
		when += ", all day"
	} else {
		when += " " + e.Starts.Format("15:04") + "–" + e.Ends.Format("15:04")
	}

	out := fmt.Sprintf("[%d] %s — %s", e.ID, when, e.Title)

	if e.Place != "" {
		out += " (" + e.Place + ")"
	}

	return out
}

func spanWords(from time.Time, days int) string {
	today := startOfDay(time.Now())

	switch {
	case days == 1 && from.Equal(today):
		return "today"
	case days == 1 && from.Equal(today.AddDate(0, 0, 1)):
		return "tomorrow"
	case days == 1:
		return "on " + from.Format("Mon 2 Jan")
	default:
		return fmt.Sprintf("from %s over %d days", from.Format("Mon 2 Jan"), days)
	}
}

func startOfDay(at time.Time) time.Time {
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
}

/*
 * whenFrom reads a day or a moment.
 *
 * Reuses the reminder parser, which already covers what is actually said to an
 * assistant, and adds the two words a diary needs that a reminder does not:
 * today, and a bare date.
 */
func whenFrom(text string, now time.Time) (time.Time, error) {
	trimmed := strings.ToLower(strings.TrimSpace(text))

	switch trimmed {
	case "today", "now":
		return now, nil
	case "this week":
		return now, nil
	}

	if at, err := time.ParseInLocation("2006-01-02", trimmed, now.Location()); err == nil {
		return at, nil
	}

	return ParseWhen(text, now)
}

/*
 * PutInTheDiary writes something in.
 *
 * Mutating. It changes a record its owner relies on to know where they are
 * supposed to be, and an appointment invented at the wrong hour is worse than
 * one that was never made — because they will act on it.
 */
type PutInTheDiary struct {
	DB Calendar
}

func (PutInTheDiary) Name() string { return "put_in_the_diary" }

func (PutInTheDiary) Description() string {
	return "Put something in the diary: a meeting, an appointment, a day off. Use when " +
		"told to book, schedule or note something at a time. Not for reminders, which " +
		"speak at a moment and are a different tool."
}

func (PutInTheDiary) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "What it is, in a few words."},
			"starts": {"type": "string", "description": "When it starts: \"tomorrow at 14:00\", \"2026-09-14 09:30\"."},
			"minutes": {"type": "integer", "description": "How long, in minutes. Empty means an hour."},
			"all_day": {"type": "boolean", "description": "True for a whole day rather than a time."},
			"place": {"type": "string", "description": "Where, if it was said."},
			"notes": {"type": "string", "description": "Anything else worth keeping with it."}
		},
		"required": ["title", "starts"]
	}`)
}

func (PutInTheDiary) Risk() Risk { return Mutating }

type eventArgs struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Starts  string `json:"starts"`
	Minutes int    `json:"minutes"`
	AllDay  bool   `json:"all_day"`
	Place   string `json:"place"`
	Notes   string `json:"notes"`
	Cancel  bool   `json:"cancel"`
}

// Summarize shows the whole event. "Put something in the diary" is a thing
// nobody can check; a day, a time and a name is.
func (PutInTheDiary) Summarize(raw json.RawMessage) string {
	var a eventArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Put something in the diary"
	}

	at, err := ParseWhen(a.Starts, time.Now())
	when := a.Starts

	if err == nil {
		when = at.Format("Mon 2 Jan 15:04")

		if a.AllDay {
			when = at.Format("Mon 2 Jan") + ", all day"
		} else if a.Minutes > 0 {
			when += "–" + at.Add(time.Duration(a.Minutes)*time.Minute).Format("15:04")
		}
	}

	out := "Put in the diary: " + when + " — " + a.Title

	if a.Place != "" {
		out += " (" + a.Place + ")"
	}

	return out
}

func (t PutInTheDiary) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a eventArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read the event: %w", err)
	}

	event, err := eventFrom(a)
	if err != nil {
		return "", err
	}

	id, err := t.DB.PutInTheDiary(event)
	if err != nil {
		return "", err
	}

	event.ID = id

	return "In the diary: " + line(event), nil
}

func eventFrom(a eventArgs) (store.Event, error) {
	at, err := ParseWhen(a.Starts, time.Now())
	if err != nil {
		return store.Event{}, err
	}

	if a.AllDay {
		at = startOfDay(at)
	}

	length := time.Duration(a.Minutes) * time.Minute

	switch {
	case a.AllDay:
		length = 24 * time.Hour
	case length <= 0:
		// An hour, which is what somebody means when they say a time and not
		// a length.
		length = time.Hour
	}

	return store.Event{
		Title:  a.Title,
		Starts: at,
		Ends:   at.Add(length),
		AllDay: a.AllDay,
		Place:  a.Place,
		Notes:  a.Notes,
	}, nil
}

/*
 * ChangeTheDiary moves or cancels something already in it.
 *
 * Mutating, and cancelling more obviously so: an appointment quietly removed
 * is an appointment somebody misses, and they find out by not being there.
 */
type ChangeTheDiary struct {
	DB Calendar
}

func (ChangeTheDiary) Name() string { return "change_the_diary" }

func (ChangeTheDiary) Description() string {
	return "Move or cancel something already in the diary, by its number from what_is_on. " +
		"Use when told something has moved, been put off, or is no longer happening."
}

func (ChangeTheDiary) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "integer", "description": "The number shown in square brackets by what_is_on."},
			"cancel": {"type": "boolean", "description": "True to take it out of the diary altogether."},
			"starts": {"type": "string", "description": "The new start, when moving it."},
			"minutes": {"type": "integer", "description": "The new length in minutes, if it changed."},
			"title": {"type": "string", "description": "A new name, if it changed."},
			"place": {"type": "string", "description": "A new place, if it changed."}
		},
		"required": ["id"]
	}`)
}

func (ChangeTheDiary) Risk() Risk { return Mutating }

func (t ChangeTheDiary) Summarize(raw json.RawMessage) string {
	var a eventArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Change the diary"
	}

	existing, _ := t.DB.Event(a.ID)

	what := fmt.Sprintf("event %d", a.ID)
	if existing != nil {
		what = "\"" + existing.Title + "\" on " + existing.Starts.Format("Mon 2 Jan 15:04")
	}

	if a.Cancel {
		return "Take " + what + " out of the diary"
	}

	if a.Starts != "" {
		return "Move " + what + " to " + a.Starts
	}

	return "Change " + what
}

func (t ChangeTheDiary) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a eventArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to change: %w", err)
	}

	existing, err := t.DB.Event(a.ID)
	if err != nil {
		return "", err
	}

	if existing == nil {
		return "", fmt.Errorf("there is nothing in the diary with the number %d", a.ID)
	}

	if a.Cancel {
		if err := t.DB.CancelIt(a.ID); err != nil {
			return "", err
		}

		return "Taken out of the diary: " + existing.Title + " on " +
			existing.Starts.Format("Mon 2 Jan"), nil
	}

	changed := *existing

	if a.Title != "" {
		changed.Title = a.Title
	}

	if a.Place != "" {
		changed.Place = a.Place
	}

	length := existing.Length()

	if a.Minutes > 0 {
		length = time.Duration(a.Minutes) * time.Minute
	}

	if a.Starts != "" {
		at, err := ParseWhen(a.Starts, time.Now())
		if err != nil {
			return "", err
		}

		changed.Starts = at
	}

	changed.Ends = changed.Starts.Add(length)

	if err := t.DB.ChangeTheDiary(changed); err != nil {
		return "", err
	}

	return "Changed: " + line(changed), nil
}
