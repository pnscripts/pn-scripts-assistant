package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/caldav"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Planning: to-do lists, an online calendar, and "what is next".
 *
 * The diary, the reminders and the to-do lists are three drawers of one
 * question — what do I have to do, and when — and somebody asking it should
 * not have to know which drawer each thing went into. what_is_next opens all
 * of them, and the online calendar when there is one, and answers in time
 * order.
 *
 * Ticking off a to-do or adding one changes only the assistant's own lists,
 * so it is Safe like a reminder. Adding to the online calendar puts an
 * appointment on somebody's server where other people may see it, so it is
 * shown before it happens.
 */

// TodoBook is the to-do lists, as much of the store as these need.
type TodoBook interface {
	AddTodo(list, text string, due *time.Time) (store.Todo, error)
	Todos(list string, withDone bool) ([]store.Todo, error)
	TodoLists() (map[string]int, error)
	TickTodo(id int64, done bool) (store.Todo, error)
	RemoveTodo(id int64) error
}

// Planner is where the planning tools look.
type Planner struct {
	Diary     Calendar
	Reminders Diary
	Todos     TodoBook

	// Online is the owner's online calendar, read when used; not configured
	// means none.
	Online func() caldav.Calendar

	// Now is the clock, replaceable in tests.
	Now func() time.Time
}

func (p Planner) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}

	return time.Now()
}

func (p Planner) online() (caldav.Calendar, bool) {
	if p.Online == nil {
		return caldav.Calendar{}, false
	}

	c := p.Online()

	return c, c.Configured()
}

var planCues = []string{
	"to do", "todo", "to-do", "list", "tick", "done", "task", "next", "today", "tomorrow", "week",
	"calendar", "agenda", "schedule", "plan", "busy", "free",
	"задач", "списък", "отметни", "свърш", "предстои", "днес", "утре", "седмица", "календар",
	"график", "план", "зает", "свободен", "купя", "пазар",
}

// TodoTool adds, ticks off or removes to-do items.
type TodoTool struct{ Plan Planner }

func (TodoTool) Name() string   { return "todo" }
func (TodoTool) Risk() Risk     { return Safe }
func (TodoTool) Cues() []string { return planCues }

func (TodoTool) Description() string {
	return "Keep the owner's to-do lists: add an item (to a named list such as shopping, home or work, " +
		"optionally due on a day), tick one off as done, undo that, or remove it. Items are named by " +
		"their number from list_todos, or by a few of their words. For things with an hour use the diary " +
		"or a reminder instead."
}

func (TodoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["add", "done", "undo", "remove"]},
			"text": {"type": "string", "description": "The item, for add; or words of it, to find one"},
			"list": {"type": "string", "description": "Which list: shopping, home, work… Empty is the general list."},
			"due": {"type": "string", "description": "When it is due, for add: today, tomorrow, friday, 2026-11-03"},
			"id": {"type": "integer", "description": "The item's number from list_todos"}
		},
		"required": ["action"]
	}`)
}

type todoArgs struct {
	Action string `json:"action"`
	Text   string `json:"text"`
	List   string `json:"list"`
	Due    string `json:"due"`
	ID     int64  `json:"id"`
}

func (TodoTool) Summarize(raw json.RawMessage) string {
	var a todoArgs

	_ = json.Unmarshal(raw, &a)

	return strings.TrimSpace(fmt.Sprintf("To-do %s: %s %s", a.Action, a.Text, a.List))
}

func (t TodoTool) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a todoArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if t.Plan.Todos == nil {
		return "", errors.New("there are no to-do lists here")
	}

	switch a.Action {
	case "add":
		var due *time.Time

		if strings.TrimSpace(a.Due) != "" {
			at, err := dayFrom(a.Due, t.Plan.now())
			if err != nil {
				return "", err
			}

			due = &at
		}

		item, err := t.Plan.Todos.AddTodo(a.List, a.Text, due)
		if err != nil {
			return "", err
		}

		return "On the " + item.List + " list: " + todoLine(item, t.Plan.now()), nil

	case "done", "undo", "remove":
		item, err := t.find(a)
		if err != nil {
			return "", err
		}

		if a.Action == "remove" {
			if err := t.Plan.Todos.RemoveTodo(item.ID); err != nil {
				return "", err
			}

			return "Removed from the " + item.List + " list: " + item.Text, nil
		}

		item, err = t.Plan.Todos.TickTodo(item.ID, a.Action == "done")
		if err != nil {
			return "", err
		}

		if a.Action == "done" {
			return "Ticked off: " + item.Text, nil
		}

		return "Back on the " + item.List + " list: " + item.Text, nil
	}

	return "", fmt.Errorf("the action is add, done, undo or remove, not %q", a.Action)
}

// find is the item named by number, or by words that match exactly one.
func (t TodoTool) find(a todoArgs) (store.Todo, error) {
	all, err := t.Plan.Todos.Todos(a.List, a.Action == "undo" || a.Action == "remove")
	if err != nil {
		return store.Todo{}, err
	}

	if a.ID > 0 {
		for _, item := range all {
			if item.ID == a.ID {
				return item, nil
			}
		}

		return store.Todo{}, fmt.Errorf("there is no item %d on the lists", a.ID)
	}

	words := strings.ToLower(strings.TrimSpace(a.Text))
	if words == "" {
		return store.Todo{}, errors.New("say which item: its number from list_todos, or its words")
	}

	var found []store.Todo

	for _, item := range all {
		if strings.Contains(strings.ToLower(item.Text), words) {
			found = append(found, item)
		}
	}

	switch len(found) {
	case 0:
		return store.Todo{}, fmt.Errorf("nothing on the lists matches %q", a.Text)
	case 1:
		return found[0], nil
	}

	var names []string
	for _, item := range found {
		names = append(names, fmt.Sprintf("[%d] %s", item.ID, item.Text))
	}

	return store.Todo{}, fmt.Errorf("%d items match %q — say which by number: %s", len(found), a.Text, strings.Join(names, "; "))
}

// dayFrom reads a day: today, tomorrow, a weekday, a date.
func dayFrom(text string, now time.Time) (time.Time, error) {
	t := strings.ToLower(strings.TrimSpace(text))

	for i, names := range [][]string{
		{"sunday", "неделя"}, {"monday", "понеделник"}, {"tuesday", "вторник"}, {"wednesday", "сряда"},
		{"thursday", "четвъртък"}, {"friday", "петък"}, {"saturday", "събота"},
	} {
		for _, n := range names {
			if t == n || t == "on "+n || t == "в "+n || t == "във "+n {
				ahead := (i - int(now.Weekday()) + 7) % 7
				if ahead == 0 {
					ahead = 7
				}

				return startOfDay(now).AddDate(0, 0, ahead), nil
			}
		}
	}

	switch t {
	case "днес":
		return startOfDay(now), nil
	case "утре":
		return startOfDay(now).AddDate(0, 0, 1), nil
	case "вдругиден":
		return startOfDay(now).AddDate(0, 0, 2), nil
	}

	at, err := whenFrom(text, now)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a day I can read; say today, tomorrow, friday or 2026-11-03", text)
	}

	return startOfDay(at), nil
}

func todoLine(item store.Todo, now time.Time) string {
	out := fmt.Sprintf("[%d] %s", item.ID, item.Text)

	if item.Due != nil {
		switch days := int(startOfDay(*item.Due).Sub(startOfDay(now)).Hours() / 24); {
		case days < 0:
			out += fmt.Sprintf(" — overdue since %s", item.Due.Format("Mon 2 Jan"))
		case days == 0:
			out += " — due today"
		case days == 1:
			out += " — due tomorrow"
		default:
			out += " — due " + item.Due.Format("Mon 2 Jan")
		}
	}

	if item.DoneAt != nil {
		out += " (done)"
	}

	return out
}

// ListTodos shows the to-do lists.
type ListTodos struct{ Plan Planner }

func (ListTodos) Name() string   { return "list_todos" }
func (ListTodos) Risk() Risk     { return Safe }
func (ListTodos) Cues() []string { return planCues }

func (ListTodos) Description() string {
	return "Show the owner's to-do lists, or one list, with what is still open (due soonest first) and, " +
		"if asked, what was ticked off lately."
}

func (ListTodos) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"list": {"type": "string", "description": "One list, or empty for all"},
			"with_done": {"type": "boolean", "description": "Also show what was ticked off in the last fortnight"}
		}
	}`)
}

func (ListTodos) Summarize(json.RawMessage) string { return "Show the to-do lists" }

func (t ListTodos) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		List     string `json:"list"`
		WithDone bool   `json:"with_done"`
	}

	_ = json.Unmarshal(raw, &a)

	if t.Plan.Todos == nil {
		return "", errors.New("there are no to-do lists here")
	}

	items, err := t.Plan.Todos.Todos(a.List, a.WithDone)
	if err != nil {
		return "", err
	}

	if len(items) == 0 {
		if a.List != "" {
			return "Nothing on the " + store.CleanList(a.List) + " list.", nil
		}

		return "The to-do lists are empty.", nil
	}

	byList := map[string][]store.Todo{}

	var order []string

	for _, item := range items {
		if _, seen := byList[item.List]; !seen {
			order = append(order, item.List)
		}

		byList[item.List] = append(byList[item.List], item)
	}

	sort.Strings(order)

	var b strings.Builder

	now := t.Plan.now()

	for _, list := range order {
		name := []rune(list)
		fmt.Fprintf(&b, "%s:\n", strings.ToUpper(string(name[:1]))+string(name[1:]))

		for _, item := range byList[list] {
			b.WriteString("- " + todoLine(item, now) + "\n")
		}
	}

	return strings.TrimSpace(b.String()), nil
}

// WhatIsNext answers "what do I have coming up" from every drawer.
type WhatIsNext struct{ Plan Planner }

func (WhatIsNext) Name() string   { return "what_is_next" }
func (WhatIsNext) Risk() Risk     { return Safe }
func (WhatIsNext) Cues() []string { return planCues }

func (WhatIsNext) Description() string {
	return "Say what is coming up, in time order, from everything at once: appointments in the diary and " +
		"in the online calendar, reminders, and to-dos that are due or overdue — then the to-dos with no " +
		"date. Use for \"what is next\", \"what do I have today\", \"what is my week like\"."
}

func (WhatIsNext) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"days": {"type": "integer", "description": "How many days ahead; default 7"}}
	}`)
}

func (WhatIsNext) Summarize(json.RawMessage) string { return "Say what is coming up" }

// Coming is one thing on the way, from whichever drawer it came.
type Coming struct {
	At     time.Time `json:"at"`
	Ends   time.Time `json:"ends,omitempty"`
	AllDay bool      `json:"all_day"`
	Kind   string    `json:"kind"` // event, online, reminder, todo
	What   string    `json:"what"`
	Place  string    `json:"place,omitempty"`
	ID     int64     `json:"id,omitempty"`
}

// Agenda is what is coming, and what could not be looked at.
type Agenda struct {
	Coming  []Coming     `json:"coming"`
	Undated []store.Todo `json:"undated"`
	Overdue []store.Todo `json:"overdue"`
	Missing []string     `json:"missing,omitempty"`
}

// Ahead gathers the agenda for the next days.
func (p Planner) Ahead(ctx context.Context, days int) Agenda {
	if days <= 0 {
		days = 7
	}

	now := p.now()
	from, to := now, startOfDay(now).AddDate(0, 0, days)

	a := Agenda{Coming: []Coming{}, Undated: []store.Todo{}, Overdue: []store.Todo{}}

	if p.Diary != nil {
		if events, err := p.Diary.WhatIsOn(startOfDay(now), to); err == nil {
			for _, e := range events {
				if e.Ends.Before(from) {
					continue
				}

				a.Coming = append(a.Coming, Coming{At: e.Starts.Local(), Ends: e.Ends.Local(), AllDay: e.AllDay, Kind: "event",
					What: e.Title, Place: e.Place, ID: e.ID})
			}
		} else {
			a.Missing = append(a.Missing, "the diary: "+err.Error())
		}
	}

	if cal, ok := p.online(); ok {
		ask, stop := context.WithTimeout(ctx, 15*time.Second)
		events, err := cal.Events(ask, startOfDay(now), to)

		stop()

		if err != nil {
			a.Missing = append(a.Missing, "the online calendar: "+err.Error())
		}

		for _, e := range events {
			if e.Ends.Before(from) {
				continue
			}

			a.Coming = append(a.Coming, Coming{At: e.Starts.Local(), Ends: e.Ends.Local(), AllDay: e.AllDay, Kind: "online",
				What: e.Title, Place: e.Place})
		}
	}

	if p.Reminders != nil {
		if list, err := p.Reminders.ListReminders(false); err == nil {
			for _, r := range list {
				if r.At.Before(to) {
					a.Coming = append(a.Coming, Coming{At: r.At.Local(), Kind: "reminder", What: r.What, ID: r.ID})
				}
			}
		}
	}

	if p.Todos != nil {
		if items, err := p.Todos.Todos("", false); err == nil {
			for _, item := range items {
				switch {
				case item.Due == nil:
					a.Undated = append(a.Undated, item)
				case item.Due.Before(startOfDay(now)):
					a.Overdue = append(a.Overdue, item)
				case item.Due.Before(to):
					a.Coming = append(a.Coming, Coming{At: *item.Due, AllDay: true, Kind: "todo",
						What: item.Text, ID: item.ID, Place: item.List})
				}
			}
		}
	}

	sort.SliceStable(a.Coming, func(i, j int) bool {
		x, y := a.Coming[i], a.Coming[j]

		if !startOfDay(x.At).Equal(startOfDay(y.At)) {
			return x.At.Before(y.At)
		}

		// Within a day: whole-day things first, then by the hour.
		if x.AllDay != y.AllDay {
			return x.AllDay
		}

		return x.At.Before(y.At)
	})

	return a
}

func (t WhatIsNext) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Days int `json:"days"`
	}

	_ = json.Unmarshal(raw, &a)

	agenda := t.Plan.Ahead(ctx, a.Days)
	now := t.Plan.now()

	var b strings.Builder

	if len(agenda.Overdue) > 0 {
		b.WriteString("Overdue:\n")

		for _, item := range agenda.Overdue {
			b.WriteString("- " + todoLine(item, now) + " (" + item.List + ")\n")
		}
	}

	day := time.Time{}

	for _, c := range agenda.Coming {
		if d := startOfDay(c.At); !d.Equal(day) {
			day = d
			fmt.Fprintf(&b, "\n%s:\n", dayWords(d, now))
		}

		b.WriteString("- " + comingLine(c) + "\n")
	}

	if len(agenda.Coming) == 0 {
		fmt.Fprintf(&b, "\nNothing in the diary, the calendar or the reminders for the next %d days.\n", daysOr(a.Days))
	}

	if n := len(agenda.Undated); n > 0 {
		fmt.Fprintf(&b, "\nTo-dos with no date (%d):\n", n)

		for i, item := range agenda.Undated {
			if i == 8 {
				fmt.Fprintf(&b, "- and %d more (list_todos)\n", n-8)

				break
			}

			b.WriteString("- " + todoLine(item, now) + " (" + item.List + ")\n")
		}
	}

	for _, m := range agenda.Missing {
		b.WriteString("\nCould not look in " + m + "\n")
	}

	return strings.TrimSpace(b.String()), nil
}

func daysOr(d int) int {
	if d <= 0 {
		return 7
	}

	return d
}

func dayWords(d, now time.Time) string {
	switch int(d.Sub(startOfDay(now)).Hours() / 24) {
	case 0:
		return "Today, " + d.Format("Mon 2 Jan")
	case 1:
		return "Tomorrow, " + d.Format("Mon 2 Jan")
	}

	return d.Format("Monday 2 Jan")
}

func comingLine(c Coming) string {
	switch c.Kind {
	case "reminder":
		return fmt.Sprintf("%s reminder: %s", c.At.Format("15:04"), c.What)
	case "todo":
		return fmt.Sprintf("due: %s (%s list)", c.What, c.Place)
	}

	when := c.At.Format("15:04") + "–" + c.Ends.Format("15:04")
	if c.AllDay {
		when = "all day"
	}

	out := when + " " + c.What

	if c.Place != "" {
		out += " (" + c.Place + ")"
	}

	if c.Kind == "online" {
		out += " — online calendar"
	}

	return out
}

// ReadCalendar reads the owner's online calendar.
type ReadCalendar struct{ Plan Planner }

func (ReadCalendar) Name() string   { return "read_calendar" }
func (ReadCalendar) Risk() Risk     { return Safe }
func (ReadCalendar) Cues() []string { return planCues }

// LeavesTheMachine: it asks the owner's calendar server, with their login.
func (ReadCalendar) LeavesTheMachine() bool { return true }

func (ReadCalendar) Description() string {
	return "Read the owner's online calendar (CalDAV: Nextcloud, Fastmail, iCloud and others): what is on " +
		"from a day for a number of days. what_is_next already includes it; use this for a calendar question " +
		"about further ahead or a specific period."
}

func (ReadCalendar) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"from": {"type": "string", "description": "From when: today, tomorrow, 2026-11-03. Empty is today."},
			"days": {"type": "integer", "description": "How many days; default 7"}
		}
	}`)
}

func (ReadCalendar) Summarize(json.RawMessage) string { return "Read the online calendar" }

func (t ReadCalendar) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		From string `json:"from"`
		Days int    `json:"days"`
	}

	_ = json.Unmarshal(raw, &a)

	cal, ok := t.Plan.online()
	if !ok {
		return "", caldav.ErrNotSetUp
	}

	now := t.Plan.now()
	from := startOfDay(now)

	if strings.TrimSpace(a.From) != "" {
		at, err := dayFrom(a.From, now)
		if err != nil {
			return "", err
		}

		from = at
	}

	events, err := cal.Events(ctx, from, from.AddDate(0, 0, daysOr(a.Days)))
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return fmt.Sprintf("Nothing in the online calendar %s.", spanWords(from, daysOr(a.Days))), nil
	}

	var b strings.Builder

	for _, e := range events {
		e.Starts, e.Ends = e.Starts.Local(), e.Ends.Local()

		when := e.Starts.Format("Mon 2 Jan 15:04") + "–" + e.Ends.Format("15:04")
		if e.AllDay {
			when = e.Starts.Format("Mon 2 Jan") + ", all day"
		}

		b.WriteString("- " + when + " " + e.Title)

		if e.Place != "" {
			b.WriteString(" (" + e.Place + ")")
		}

		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String()), nil
}

// WriteCalendar adds an appointment to the owner's online calendar.
type WriteCalendar struct{ Plan Planner }

func (WriteCalendar) Name() string   { return "write_calendar" }
func (WriteCalendar) Risk() Risk     { return Mutating }
func (WriteCalendar) Cues() []string { return planCues }

// LeavesTheMachine: the appointment goes to the owner's calendar server.
func (WriteCalendar) LeavesTheMachine() bool { return true }

func (WriteCalendar) Description() string {
	return "Put a new appointment in the owner's online calendar, where their phone and anybody they " +
		"share it with will see it. Never changes or deletes anything there. For the assistant's own diary " +
		"use put_in_the_diary."
}

func (WriteCalendar) Parameters() json.RawMessage { return PutInTheDiary{}.Parameters() }

func (WriteCalendar) Summarize(raw json.RawMessage) string {
	return strings.Replace(PutInTheDiary{}.Summarize(raw), "Put in the diary", "Put in your online calendar", 1)
}

func (t WriteCalendar) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a eventArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read the appointment: %w", err)
	}

	cal, ok := t.Plan.online()
	if !ok {
		return "", caldav.ErrNotSetUp
	}

	event, err := eventFrom(a)
	if err != nil {
		return "", err
	}

	if _, err := cal.Add(ctx, event); err != nil {
		return "", err
	}

	return "In the online calendar: " + strings.TrimPrefix(line(event), "[0] "), nil
}
