package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"pn-scripts-assistant/internal/brain/diary"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The diary, which lives here rather than in an account.
 *
 * Importing and exporting are on this page rather than being tools, because
 * both are rare and deliberate. A tool for them would sit in the list on every
 * turn being not-chosen, and the cost of a tool is paid by every turn whether
 * or not it is used.
 */
func (s *Server) handleDiary(w http.ResponseWriter, r *http.Request) {
	days := 14

	if given := r.URL.Query().Get("days"); given != "" {
		if n, err := strconv.Atoi(given); err == nil && n > 0 && n <= 400 {
			days = n
		}
	}

	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	events, err := s.brain.DB.WhatIsOn(from, from.AddDate(0, 0, days))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	total, _ := s.brain.DB.CountEvents()

	ok(w, map[string]any{"events": events, "days": days, "total": total})
}

func (s *Server) handleSaveEvent(w http.ResponseWriter, r *http.Request) {
	var event store.Event

	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	if event.ID != 0 {
		if err := s.brain.DB.ChangeTheDiary(event); err != nil {
			fail(w, http.StatusBadRequest, err.Error())

			return
		}

		ok(w, map[string]any{"id": event.ID})

		return
	}

	id, err := s.brain.DB.PutInTheDiary(event)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"id": id})
}

func (s *Server) handleCancelEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not an event number")

		return
	}

	if err := s.brain.DB.CancelIt(id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"cancelled": id})
}

/*
 * handleImportDiary reads a calendar file that is already on this machine.
 *
 * By path rather than by upload, because the file is already here: somebody
 * exporting from whatever they use now saves it to their downloads and says
 * where. Sending it through the browser to a server on the same machine would
 * be a round trip that achieves nothing.
 */
func (s *Server) handleImportDiary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	raw, err := os.ReadFile(body.Path)
	if err != nil {
		fail(w, http.StatusBadRequest, plainFileError(err))

		return
	}

	events, err := diary.Read(string(raw), body.Path)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	var added, failed int

	for _, event := range events {
		if _, err := s.brain.DB.PutInTheDiary(event); err != nil {
			failed++

			continue
		}

		added++
	}

	// Said rather than assumed. A file where half the events had no name is a
	// file somebody should look at, not one to report as a clean import.
	message := fmt.Sprintf("%d events read in", added)

	if failed > 0 {
		message += fmt.Sprintf(", %d skipped because they could not be read", failed)
	}

	ok(w, map[string]any{"added": added, "skipped": failed, "said": message})
}

// handleExportDiary writes everything out as a file any calendar will read.
func (s *Server) handleExportDiary(w http.ResponseWriter, r *http.Request) {
	// Everything, not a window: an export is for keeping or for moving, and
	// one that quietly held back last year is not either.
	events, err := s.brain.DB.WhatIsOn(
		time.Now().AddDate(-20, 0, 0), time.Now().AddDate(20, 0, 0))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="diary.ics"`)

	fmt.Fprint(w, diary.Write(events))
}

func plainFileError(err error) string {
	switch {
	case os.IsNotExist(err):
		return "there is no file at that path"
	case os.IsPermission(err):
		return "that file cannot be read"
	default:
		return err.Error()
	}
}
