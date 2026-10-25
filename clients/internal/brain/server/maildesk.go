package server

import (
	"errors"
	"net/http"
	"strconv"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Mail page: new mail sorted, and replies waiting for a yes.
 *
 * Reading is from the headers only and opens nothing. A draft is changed or
 * discarded freely; sending is its owner pressing Send on that one draft,
 * at the desk only (guard.go), and is recorded in What I did like any other
 * message sent in their name.
 */

func (s *Server) handleMailDigest(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 || hours > 24*31 {
		hours = 24
	}

	d, source, err := s.brain.Desk.Digest(r.Context(), hours)
	if errors.Is(err, tools.ErrNoMail) {
		ok(w, map[string]any{"none": true})

		return
	}

	if err != nil {
		fail(w, http.StatusBadGateway, "The mail could not be read: "+err.Error())

		return
	}

	ok(w, map[string]any{"digest": d, "source": source, "hours": hours})
}

func (s *Server) handleDrafts(w http.ResponseWriter, r *http.Request) {
	all, err := s.brain.DB.Drafts("", 100)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	waiting, done := []store.Draft{}, []store.Draft{}

	for _, d := range all {
		switch d.Status {
		case store.DraftWaiting, store.DraftFailed, "sending":
			waiting = append(waiting, d)
		default:
			done = append(done, d)
		}
	}

	canSend := s.brain.Desk.Account != nil && s.brain.Desk.Account().Configured()

	ok(w, map[string]any{"waiting": waiting, "done": done, "can_send": canSend, "morning": s.brain.Cfg.MailMorning})
}

// handleMailMorning switches the morning mail summary on or off. A setting,
// so at the desk only (guard.go, /api/settings).
func (s *Server) handleMailMorning(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	s.brain.Cfg.MailMorning = body.On

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "That could not be saved: "+err.Error())

		return
	}

	ok(w, map[string]any{"morning": s.brain.Cfg.MailMorning})
}

func (s *Server) handleDraftChange(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	var body struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if !decodeUpTo(w, r, &body, 1<<20) {
		return
	}

	dr, err := s.brain.DB.ChangeDraft(id, body.To, body.Subject, body.Body)
	if errors.Is(err, store.ErrNoDraft) {
		fail(w, http.StatusNotFound, "That draft is no longer waiting.")

		return
	}

	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, dr)
}

func (s *Server) handleDraftDiscard(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.SetDraftStatus(id, store.DraftDiscarded, ""); err != nil {
		fail(w, http.StatusNotFound, "That draft is no longer waiting.")

		return
	}

	ok(w, map[string]any{"discarded": id})
}

// handleDraftSend is its owner pressing Send on one draft: that press is the
// yes, for that message only.
func (s *Server) handleDraftSend(w http.ResponseWriter, r *http.Request) {
	letItThink(w)

	id, good := idOf(w, r)
	if !good {
		return
	}

	dr, _ := s.brain.DB.Draft(id)

	said, err := tools.SendWaitingDraft(s.brain.Desk, id)

	deed := store.Deed{
		Tool: "send_draft", Summary: "Send draft to " + dr.To + ": " + dr.Subject,
		Asked: "id: " + strconv.FormatInt(id, 10), Changes: true, Leaves: "mail",
		Specialist: "you, from the Mail page", Outcome: "done", Result: said,
	}

	if err != nil {
		deed.Outcome, deed.Result = "failed", err.Error()
	}

	_, _ = s.brain.DB.AddDeed(deed)

	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"sent": id, "said": said})
}
