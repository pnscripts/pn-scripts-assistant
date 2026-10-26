package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/sheet"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Tables page: a table looked over, a question answered from it with
 * every total checked, and the result saved as a new workbook with its
 * checks beside it. Saving works the recipe out again rather than writing
 * what the page sends, so the file holds exactly the checked numbers; and
 * it never writes over a file that is there.
 */

type tableBody struct {
	Path     string        `json:"path"`
	Sheet    string        `json:"sheet"`
	Question string        `json:"question"`
	Recipe   *sheet.Recipe `json:"recipe"`
	Out      string        `json:"out"`
}

func tableFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, brain.ErrNoModel), errors.Is(err, tools.ErrNoTableModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to turn the question into a calculation.")
	case errors.Is(err, sheet.ErrColumn):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		fail(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) handleTablesLook(w http.ResponseWriter, r *http.Request) {
	var body tableBody

	if !decodeUpTo(w, r, &body, 1<<13) {
		return
	}

	looked, err := s.brain.Tables().Look(r.Context(), strings.TrimSpace(body.Path), body.Sheet)
	if err != nil {
		tableFailed(w, err)

		return
	}

	ok(w, looked)
}

func (s *Server) handleTablesCalculate(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	var body tableBody

	if !decodeUpTo(w, r, &body, 1<<16) {
		return
	}

	done, err := s.brain.Tables().Calculate(r.Context(), strings.TrimSpace(body.Path), body.Sheet, body.Question, body.Recipe)
	if err != nil {
		tableFailed(w, err)

		return
	}

	ok(w, done)
}

func (s *Server) handleTablesSave(w http.ResponseWriter, r *http.Request) {
	var body tableBody

	if !decodeUpTo(w, r, &body, 1<<16) {
		return
	}

	out := strings.TrimSpace(body.Out)

	if body.Recipe == nil {
		fail(w, http.StatusBadRequest, "Work something out first; then it can be saved.")

		return
	}

	if !filepath.IsAbs(out) || !strings.EqualFold(filepath.Ext(out), ".xlsx") {
		fail(w, http.StatusBadRequest, "Give the full path of a new .xlsx file to save to.")

		return
	}

	if err := tools.GuardSensitive(r.Context(), out); err != nil {
		fail(w, http.StatusForbidden, err.Error())

		return
	}

	if _, err := os.Stat(out); err == nil {
		fail(w, http.StatusConflict, "There is already a file at that path; choose another name.")

		return
	}

	done, err := s.brain.Tables().Calculate(r.Context(), strings.TrimSpace(body.Path), body.Sheet, "", body.Recipe)
	if err != nil {
		tableFailed(w, err)

		return
	}

	book, err := tools.Workbook(done)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// O_EXCL: a file that appeared since the check above is not replaced.
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if _, err := f.Write(book); err != nil {
		f.Close()
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if err := f.Close(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"saved": out})
}
