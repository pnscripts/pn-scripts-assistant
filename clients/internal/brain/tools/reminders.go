package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

/*
 * Things to be reminded of, kept on this machine.
 *
 * A calendar is exactly the kind of thing that quietly ends up on somebody
 * else's server, and the whole promise here is that it does not. These live in
 * the same database as everything else the brain knows, travel with it to
 * another drive, and are visible in an export.
 *
 * Times are given in plain words because that is how they are said out loud.
 * "In twenty minutes" and "tomorrow at nine" are what a person says; making
 * them write RFC3339 to their own assistant would be absurd.
 */

// Diary is what these tools need from the store.
type Diary interface {
	AddReminder(what string, at time.Time) (int64, string, error)
	ListReminders(includePast bool) ([]DiaryItem, error)
	ForgetReminder(id int64) error
}

// DiaryItem is one reminder, as a model should see it.
type DiaryItem struct {
	ID   int64
	What string
	At   time.Time
	Said bool
}

/* ---------- adding ---------- */

// Remind records something to be said at a time.
type Remind struct {
	Diary Diary

	// Now is overridable so the parsing can be tested against a fixed clock.
	Now func() time.Time
}

func (Remind) Name() string { return "remind_me" }

func (Remind) Description() string {
	return "Remind the owner of something at a given time. The brain will say " +
		"it out loud when the time comes. Use for appointments, timers and " +
		"anything they ask to be reminded about."
}

func (Remind) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"what": {"type": "string", "description": "What to say when the time comes"},
			"when": {
				"type": "string",
				"description": "When, in plain words: \"in 20 minutes\", \"in 2 hours\", \"tomorrow at 9\", \"at 18:30\", or a date and time"
			}
		},
		"required": ["what", "when"]
	}`)
}

func (Remind) Risk() Risk { return Safe }

func (Remind) Summarize(raw json.RawMessage) string {
	var a remindArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Remind you %q %s", a.What, a.When)
}

type remindArgs struct {
	What string `json:"what"`
	When string `json:"when"`
}

func (t Remind) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Diary == nil {
		return "", fmt.Errorf("there is nowhere to keep reminders")
	}

	var a remindArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if strings.TrimSpace(a.What) == "" {
		return "", fmt.Errorf("say what to be reminded of")
	}

	now := time.Now
	if t.Now != nil {
		now = t.Now
	}

	at, err := ParseWhen(a.When, now())
	if err != nil {
		return "", err
	}

	id, when, err := t.Diary.AddReminder(a.What, at)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Reminder %d set for %s: %s", id, when, a.What), nil
}

/* ---------- listing and forgetting ---------- */

// ListReminders reports what is coming.
type ListReminders struct{ Diary Diary }

func (ListReminders) Name() string { return "list_reminders" }

func (ListReminders) Description() string {
	return "List the reminders that have not happened yet, soonest first."
}

func (ListReminders) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListReminders) Risk() Risk { return Safe }

func (ListReminders) Summarize(json.RawMessage) string { return "List your reminders" }

func (t ListReminders) Execute(context.Context, json.RawMessage) (string, error) {
	if t.Diary == nil {
		return "", fmt.Errorf("there is nowhere to keep reminders")
	}

	items, err := t.Diary.ListReminders(false)
	if err != nil {
		return "", err
	}

	if len(items) == 0 {
		return "Nothing is coming up.", nil
	}

	var b strings.Builder

	for _, it := range items {
		fmt.Fprintf(&b, "%d. %s — %s\n", it.ID, it.At.Local().Format("Mon 2 Jan 15:04"), it.What)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// ForgetReminder removes one.
type ForgetReminder struct{ Diary Diary }

func (ForgetReminder) Name() string { return "forget_reminder" }

func (ForgetReminder) Description() string {
	return "Cancel a reminder by its number, from list_reminders."
}

func (ForgetReminder) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"id": {"type": "integer", "description": "Which reminder to cancel"}},
		"required": ["id"]
	}`)
}

func (ForgetReminder) Risk() Risk { return Safe }

func (ForgetReminder) Summarize(raw json.RawMessage) string {
	var a struct {
		ID int64 `json:"id"`
	}

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Cancel reminder %d", a.ID)
}

func (t ForgetReminder) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	if t.Diary == nil {
		return "", fmt.Errorf("there is nowhere to keep reminders")
	}

	var a struct {
		ID int64 `json:"id"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := t.Diary.ForgetReminder(a.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("Reminder %d cancelled.", a.ID), nil
}

/* ---------- reading a time out of what somebody said ---------- */

/*
 * ParseWhen turns the words people use into a moment.
 *
 * Not a general date parser, and not trying to be. It covers what is actually
 * said to an assistant out loud, and refuses everything else clearly enough
 * that the model can ask rather than guess — a reminder set for the wrong time
 * is worse than one that was not set, because its owner is relying on it.
 */
func ParseWhen(text string, now time.Time) (time.Time, error) {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.TrimPrefix(s, "at ")

	if s == "" {
		return time.Time{}, fmt.Errorf("say when")
	}

	// A full timestamp, if the model produced one.
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02T15:04"} {
		if at, err := time.ParseInLocation(layout, strings.ToUpper(text), now.Location()); err == nil {
			return at, nil
		}
	}

	// "in 20 minutes", "in 2 hours", "in a minute"
	if rest, ok := strings.CutPrefix(s, "in "); ok {
		return parseIn(rest, now)
	}

	// "tomorrow at 9", "tomorrow 09:30"
	day := now

	if rest, ok := strings.CutPrefix(s, "tomorrow"); ok {
		day = now.AddDate(0, 0, 1)
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "at "))

		if s == "" {
			// A whole day with no hour means the morning, not midnight.
			return time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, now.Location()), nil
		}
	}

	if at, ok := parseClock(s, day, now); ok {
		return at, nil
	}

	return time.Time{}, fmt.Errorf(
		"could not read %q as a time; try \"in 20 minutes\", \"tomorrow at 9\" "+
			"or \"2026-08-30 18:30\"", text)
}

func parseIn(rest string, now time.Time) (time.Time, error) {
	fields := strings.Fields(rest)

	if len(fields) < 2 {
		return time.Time{}, fmt.Errorf("say how long, such as \"in 20 minutes\"")
	}

	n := 0

	switch fields[0] {
	case "a", "an", "one":
		n = 1
	default:
		if _, err := fmt.Sscanf(fields[0], "%d", &n); err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("%q is not a number of minutes or hours", fields[0])
		}
	}

	unit := strings.TrimSuffix(fields[1], "s")

	switch unit {
	case "second", "sec":
		return now.Add(time.Duration(n) * time.Second), nil
	case "minute", "min":
		return now.Add(time.Duration(n) * time.Minute), nil
	case "hour", "hr":
		return now.Add(time.Duration(n) * time.Hour), nil
	case "day":
		return now.AddDate(0, 0, n), nil
	case "week":
		return now.AddDate(0, 0, 7*n), nil
	}

	return time.Time{}, fmt.Errorf("%q is not a length of time I know", fields[1])
}

// parseClock reads "9", "9pm", "18:30" as a time of day.
func parseClock(s string, day, now time.Time) (time.Time, bool) {
	s = strings.TrimSpace(s)

	if s == "" {
		return time.Time{}, false
	}

	pm := strings.HasSuffix(s, "pm")
	am := strings.HasSuffix(s, "am")

	s = strings.TrimSuffix(strings.TrimSuffix(s, "pm"), "am")
	s = strings.TrimSpace(s)

	var hour, minute int

	switch {
	case strings.Contains(s, ":"):
		if _, err := fmt.Sscanf(s, "%d:%d", &hour, &minute); err != nil {
			return time.Time{}, false
		}
	default:
		if _, err := fmt.Sscanf(s, "%d", &hour); err != nil {
			return time.Time{}, false
		}
	}

	if pm && hour < 12 {
		hour += 12
	}

	if am && hour == 12 {
		hour = 0
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, false
	}

	at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())

	/*
	 * A time that has already passed today means tomorrow.
	 *
	 * Somebody saying "remind me at nine" in the evening does not mean nine
	 * o'clock this morning, and setting a reminder in the past is the same as
	 * not setting one — except that they think it is set.
	 */
	if !at.After(now) && !at.Equal(day) {
		at = at.AddDate(0, 0, 1)
	}

	return at, true
}
