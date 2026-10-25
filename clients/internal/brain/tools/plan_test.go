package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/caldav"
	"pn-scripts-assistant/internal/brain/store"
)

type fakeReminders struct{ items []DiaryItem }

func (f fakeReminders) AddReminder(string, time.Time) (int64, string, error) { return 0, "", nil }
func (f fakeReminders) ListReminders(bool) ([]DiaryItem, error)              { return f.items, nil }
func (f fakeReminders) ForgetReminder(int64) error                           { return nil }

func planFor(t *testing.T, now time.Time) (Planner, *store.DB) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return Planner{Diary: db, Todos: db, Reminders: fakeReminders{}, Now: func() time.Time { return now }}, db
}

func TestToDosAreAddedTickedAndFoundByTheirWords(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.Local) // a Monday
	plan, _ := planFor(t, now)
	todo := TodoTool{Plan: plan}

	out, err := call(t, todo, map[string]any{"action": "add", "text": "Купи мляко", "list": "Пазар"})
	if err != nil || !strings.Contains(out, "On the пазар list") {
		t.Fatal(out, err)
	}

	out, _ = call(t, todo, map[string]any{"action": "add", "text": "Send the offer", "list": "work", "due": "friday"})
	if !strings.Contains(out, "due Fri 30 Oct") {
		t.Errorf("%s", out)
	}

	_, _ = call(t, todo, map[string]any{"action": "add", "text": "Send the invoice", "list": "work", "due": "утре"})

	if _, err := call(t, todo, map[string]any{"action": "done", "text": "send"}); err == nil || !strings.Contains(err.Error(), "2 items match") {
		t.Errorf("an ambiguous tick: %v", err)
	}

	if out, err = call(t, todo, map[string]any{"action": "done", "text": "мляко"}); err != nil || out != "Ticked off: Купи мляко" {
		t.Errorf("%s %v", out, err)
	}

	list, _ := call(t, ListTodos{Plan: plan}, map[string]any{})
	if !strings.Contains(list, "Work:\n- [3] Send the invoice — due tomorrow\n- [2] Send the offer — due Fri 30 Oct") ||
		strings.Contains(list, "мляко") {
		t.Errorf("%s", list)
	}

	if out, _ = call(t, todo, map[string]any{"action": "undo", "text": "мляко"}); !strings.Contains(out, "Back on the пазар list") {
		t.Errorf("%s", out)
	}

	if _, err := call(t, todo, map[string]any{"action": "add", "text": "x", "due": "someday maybe"}); err == nil {
		t.Error("an unreadable day was accepted")
	}
}

/*
 * "What is next" puts the diary, the online calendar, reminders and due
 * to-dos in one time order, says what is overdue, and lists what has no
 * date after.
 */
func TestWhatIsNextOpensEveryDrawer(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.Local)
	plan, db := planFor(t, now)

	_, _ = db.PutInTheDiary(store.Event{Title: "Dentist", Starts: now.Add(4 * time.Hour), Ends: now.Add(5 * time.Hour), Place: "Sofia"})
	_, _ = db.PutInTheDiary(store.Event{Title: "Yesterday's thing", Starts: now.Add(-26 * time.Hour), Ends: now.Add(-25 * time.Hour)})

	yesterday, tomorrow := now.AddDate(0, 0, -1), now.AddDate(0, 0, 1)
	_, _ = db.AddTodo("work", "Pay the tax", &yesterday)
	_, _ = db.AddTodo("work", "Send the offer", &tomorrow)
	_, _ = db.AddTodo("", "Call grandma", nil)

	plan.Reminders = fakeReminders{items: []DiaryItem{{ID: 7, What: "Take the bins out", At: now.Add(9 * time.Hour)}}}

	calendar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:propstat><d:prop><c:calendar-data>`+
			"BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nSUMMARY:School meeting\r\nDTSTART:"+
			tomorrow.UTC().Format("20060102T150405Z")+"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"+
			`</c:calendar-data></d:prop></d:propstat></d:response></d:multistatus>`)
	}))
	defer calendar.Close()

	plan.Online = func() caldav.Calendar { return caldav.Calendar{URL: calendar.URL + "/cal/"} }

	out, err := call(t, WhatIsNext{Plan: plan}, map[string]any{"days": 3})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"Overdue:\n- [1] Pay the tax — overdue since Sun 25 Oct (work)",
		"Today, Mon 26 Oct:\n- 14:00–15:00 Dentist (Sofia)\n- 19:00 reminder: Take the bins out",
		"Tomorrow, Tue 27 Oct:\n- due: Send the offer (work list)\n- 10:00–11:00 School meeting — online calendar",
		"To-dos with no date (1):\n- [3] Call grandma (to do)",
	}

	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing\n%s\nin\n%s", w, out)
		}
	}

	if strings.Contains(out, "Yesterday's thing") {
		t.Error("a past appointment was listed as coming")
	}

	// A calendar that cannot be reached is said, and the rest still comes.
	plan.Online = func() caldav.Calendar { return caldav.Calendar{URL: "https://127.0.0.1:1/cal/"} }

	out, _ = call(t, WhatIsNext{Plan: plan}, map[string]any{})
	if !strings.Contains(out, "Could not look in the online calendar") || !strings.Contains(out, "Dentist") {
		t.Errorf("%s", out)
	}
}

func TestTheOnlineCalendarIsAskedBeforeItIsWritten(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"title": "Parents' evening", "starts": "2026-11-03 18:00", "place": "School"})

	w := WriteCalendar{}
	if w.Risk() != Mutating || !strings.HasPrefix(w.Summarize(raw), "Put in your online calendar: Tue 3 Nov 18:00 — Parents' evening (School)") {
		t.Errorf("%s", w.Summarize(raw))
	}

	if _, err := (WriteCalendar{Plan: Planner{}}).Execute(context.Background(), raw); err != caldav.ErrNotSetUp {
		t.Errorf("%v", err)
	}

	if leaves, what := Leaves(w); !leaves || !strings.Contains(what, "calendar server") {
		t.Errorf("%v %s", leaves, what)
	}
}
