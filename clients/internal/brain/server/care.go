package server

import (
	"errors"
	"net/http"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/care"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Customers page: a reply to a customer written from the business's own
 * answers and documents and checked, kept as a mail draft for its owner's
 * yes; the answer book; and how the business wants to sound. Nothing here
 * sends anything.
 */

func (s *Server) handleCare(w http.ResponseWriter, _ *http.Request) {
	c := s.brain.CustomerCare()

	answers, err := s.brain.DB.CareAnswers()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	p := c.Profile()

	files := []string{}
	if p.Folder != "" {
		files = care.Files(c.Folder(p))
	}

	ok(w, map[string]any{"profile": p, "answers": answers, "files": len(files), "kinds": care.Kinds})
}

func (s *Server) handleCareProfile(w http.ResponseWriter, r *http.Request) {
	var p care.Profile

	if !decodeUpTo(w, r, &p, 1<<13) {
		return
	}

	if err := s.brain.CustomerCare().SetProfile(p); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"profile": s.brain.CustomerCare().Profile()})
}

func (s *Server) handleCareAnswer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       int64  `json:"id"`
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}

	if !decodeUpTo(w, r, &body, 1<<15) {
		return
	}

	var (
		a   store.CareAnswer
		err error
	)

	if body.ID > 0 {
		a, err = s.brain.DB.ChangeCareAnswer(body.ID, body.Question, body.Answer)
	} else {
		a, err = s.brain.DB.AddCareAnswer(body.Question, body.Answer)
	}

	switch {
	case errors.Is(err, store.ErrNoCareAnswer):
		fail(w, http.StatusNotFound, "That answer is not there.")
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		ok(w, a)
	}
}

func (s *Server) handleCareAnswerRemove(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.RemoveCareAnswer(id); errors.Is(err, store.ErrNoCareAnswer) {
		fail(w, http.StatusNotFound, "That answer is not there.")

		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": id})
}

func (s *Server) handleCareReply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message  string `json:"message"`
		Language string `json:"language"`
	}

	if !decodeUpTo(w, r, &body, 1<<17) {
		return
	}

	written, err := s.brain.CustomerCare().Reply(r.Context(), body.Message, body.Language)

	switch {
	case errors.Is(err, brain.ErrNoModel), errors.Is(err, tools.ErrNoCareModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to write the reply.")
	case err != nil:
		fail(w, http.StatusBadRequest, err.Error())
	default:
		ok(w, map[string]any{"reply": written, "ready": written.Ready()})
	}
}

// handleCareKeep keeps a reply — as its owner may have changed it — as a
// mail draft waiting for their yes.
func (s *Server) handleCareKeep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if !decodeUpTo(w, r, &body, 1<<17) {
		return
	}

	dr, _, err := s.brain.CustomerCare().Keep(r.Context(), "", body.To, body.Subject, body.Body)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"draft": dr, "gaps": care.Gaps(dr.Body)})
}
