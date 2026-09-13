package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

/*
 * Importing a job classification, from a button.
 *
 * Never on its own and never from the network — see occupations.Source. What
 * this adds is the part a person sees: it starts in the background, says which
 * pass it is on and how far into it, can be stopped, and says afterwards what
 * it did and what it could not settle. Pressing it again after stopping picks
 * up where it was, because every row already written is found and left.
 */

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
		From   string `json:"from"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	id, err := s.brain.Import(body.Source, body.From)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"id": id})
}

/*
 * handleImports lists the recent runs — and first reads in anything setup
 * downloaded that this brain has not read yet, since opening Organisation
 * after setup is exactly when somebody expects to see it arriving.
 */
func (s *Server) handleImports(w http.ResponseWriter, r *http.Request) {
	s.brain.ImportDownloaded()

	runs, err := s.brain.DB.Imports(5)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"imports": runs})
}

func (s *Server) handleStopImport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not an import")

		return
	}

	if err := s.brain.StopImport(id); err != nil {
		fail(w, http.StatusNotFound, err.Error())

		return
	}

	ok(w, map[string]any{"stopping": id})
}
