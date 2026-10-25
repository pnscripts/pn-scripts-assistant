package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/caldav"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The Plan page: what is next across the diary, the reminders, the to-do
 * lists and the online calendar; the to-do lists themselves; and setting up
 * the online calendar, which is a setting and so at the desk only.
 */

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 62 {
		days = 7
	}

	agenda := s.brain.Planner().Ahead(r.Context(), days)
	cal := s.brain.OnlineCalendar()

	ok(w, map[string]any{
		"agenda": agenda,
		"days":   days,
		"calendar": map[string]any{
			"configured": cal.Configured(), "url": cal.URL, "user": cal.User,
			"has_password": cal.Password != "",
		},
	})
}

func (s *Server) handleTodos(w http.ResponseWriter, r *http.Request) {
	items, err := s.brain.DB.Todos("", true)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	lists, _ := s.brain.DB.TodoLists()

	ok(w, map[string]any{"items": items, "lists": lists})
}

func (s *Server) handleTodoAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
		List string `json:"list"`
		Due  string `json:"due"`
	}

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	var due *time.Time

	if strings.TrimSpace(body.Due) != "" {
		at, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(body.Due), time.Local)
		if err != nil {
			fail(w, http.StatusBadRequest, "That is not a date.")

			return
		}

		due = &at
	}

	item, err := s.brain.DB.AddTodo(body.List, body.Text, due)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, item)
}

func (s *Server) handleTodoTick(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	var body struct {
		Done bool `json:"done"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	item, err := s.brain.DB.TickTodo(id, body.Done)
	if errors.Is(err, store.ErrNoTodo) {
		fail(w, http.StatusNotFound, "That item is not on the lists.")

		return
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, item)
}

func (s *Server) handleTodoRemove(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.RemoveTodo(id); err != nil {
		fail(w, http.StatusNotFound, "That item is not on the lists.")

		return
	}

	ok(w, map[string]any{"removed": id})
}

/*
 * handleCalendarSettings sets up the online calendar, or removes it. The
 * address is asked before it is kept, so a wrong address or password is
 * said now rather than the first time somebody asks what is on. An empty
 * password keeps the one already saved.
 */
func (s *Server) handleCalendarSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string `json:"url"`
		User     string `json:"user"`
		Password string `json:"password"`
		Remove   bool   `json:"remove"`
	}

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	cfg := &s.brain.Cfg

	if body.Remove {
		cfg.CalendarURL, cfg.CalendarUser, cfg.CalendarPassword = "", "", ""

		if err := cfg.Save(s.brain.Root); err != nil {
			fail(w, http.StatusInternalServerError, "That could not be saved: "+err.Error())

			return
		}

		s.brain.CalendarSetUp(false)
		ok(w, map[string]any{"configured": false})

		return
	}

	password := body.Password
	if password == "" {
		password = cfg.CalendarPassword
	}

	cal := caldav.Calendar{URL: strings.TrimSpace(body.URL), User: strings.TrimSpace(body.User), Password: password}

	ctx, stop := context.WithTimeout(r.Context(), 25*time.Second)
	defer stop()

	name, err := cal.Check(ctx)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	cfg.CalendarURL, cfg.CalendarUser, cfg.CalendarPassword = cal.URL, cal.User, cal.Password

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "That could not be saved: "+err.Error())

		return
	}

	s.brain.CalendarSetUp(true)

	ok(w, map[string]any{"configured": true, "name": name})
}
