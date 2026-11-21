package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/connect"
	"pn-scripts-assistant/internal/brain/mcpimport"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The two things about other programs that are written at the desk and
 * nowhere else (brain/handover.go): the actions they may run, and the
 * entry that connects one of them to the assistant.
 *
 * Both routes are for this computer alone (onThisComputerOnly), their posts
 * take the window's key (fromTheWindowOnly), and neither has a way in
 * through the door other programs ask at: a program can run an action by
 * its name and can never write one.
 */

// actionsState is the actions as the page lists them, with the limits it
// says beside the form.
func (s *Server) actionsState(w http.ResponseWriter) {
	actions, err := s.brain.OutsideActions()
	if err != nil {
		fail(w, http.StatusInternalServerError, "The actions could not be read.")

		return
	}

	ok(w, map[string]any{
		"actions": actions,
		"most":    otherprograms.MostActions,
		"name":    otherprograms.MostActionName,
		"command": otherprograms.MostActionCommand,
		"seconds": int(brain.ActionTimeout().Seconds()),
	})
}

func (s *Server) handleOutsideActions(w http.ResponseWriter, _ *http.Request) { s.actionsState(w) }

// notKept answers an action that cannot be kept, with a code the page says
// in its own language.
func notKept(w http.ResponseWriter, why string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": "That action cannot be kept.", "why": why})
}

func (s *Server) handleOutsideActionAdd(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Name    string `json:"name"`
		Command string `json:"command"`
	}

	if fromAPage(r, false) {
		fail(w, http.StatusForbidden, "that request came from somewhere else")

		return
	}

	if err := strictly(w, r, &asked); err != nil {
		fail(w, http.StatusBadRequest, "An action is a name and a command.")

		return
	}

	_, err := s.brain.AddOutsideAction(asked.Name, asked.Command)

	var not brain.ErrNotAnAction

	switch {
	case errors.As(err, &not):
		notKept(w, not.Why)

		return
	case err != nil:
		s.log.Warn("an action for other programs could not be kept", "error", err)
		fail(w, http.StatusInternalServerError, "That could not be kept.")

		return
	}

	s.actionsState(w)
}

func (s *Server) handleOutsideActionRemove(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		ID int64 `json:"id"`
	}

	if fromAPage(r, false) {
		fail(w, http.StatusForbidden, "that request came from somewhere else")

		return
	}

	if err := strictly(w, r, &asked); err != nil || asked.ID <= 0 {
		fail(w, http.StatusBadRequest, "Say which action, by its number.")

		return
	}

	switch err := s.brain.RemoveOutsideAction(asked.ID); {
	case errors.Is(err, store.ErrNoAction):
		// Gone already: the list as it is now says so.
	case err != nil:
		s.log.Warn("an action for other programs could not be taken away", "error", err)
		fail(w, http.StatusInternalServerError, "That could not be taken away.")

		return
	}

	s.actionsState(w)
}

// connectState is what connecting each program would do, as the page shows
// it before anything is written.
func (s *Server) connectState(w http.ResponseWriter, done *connect.Done) {
	plans, err := s.brain.ConnectPlans()
	if err != nil {
		s.log.Warn("what connecting another program would do could not be worked out", "error", err)
		fail(w, http.StatusInternalServerError, "What connecting would do could not be worked out.")

		return
	}

	body := map[string]any{"programs": plans}
	if done != nil {
		body["done"] = done
	}

	ok(w, body)
}

func (s *Server) handleOutsideConnectPlans(w http.ResponseWriter, _ *http.Request) {
	s.connectState(w, nil)
}

func (s *Server) handleOutsideConnect(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Client string `json:"client"`
		Seen   string `json:"seen"`
	}

	if fromAPage(r, false) {
		fail(w, http.StatusForbidden, "that request came from somewhere else")

		return
	}

	if err := strictly(w, r, &asked); err != nil || !connect.Known(mcpimport.Client(asked.Client)) || len(asked.Seen) != 64 {
		fail(w, http.StatusBadRequest, "Say which program, and which plan you were shown.")

		return
	}

	refuse := func(why, said string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": said, "why": why})
	}

	done, err := s.brain.Connect(mcpimport.Client(asked.Client), asked.Seen)

	switch {
	case errors.Is(err, connect.ErrChanged):
		refuse("changed", "The settings changed since they were shown. Nothing was written; look at them again.")
	case errors.Is(err, connect.ErrUnread):
		refuse("unread", "That settings file is not something the assistant can change. Nothing was written.")
	case errors.Is(err, connect.ErrSame):
		refuse("same", "It is connected already. Nothing was written.")
	case err != nil:
		s.log.Warn("another program could not be connected", "client", asked.Client, "error", err)
		fail(w, http.StatusInternalServerError, "That could not be written. The settings file is as it was.")
	default:
		s.connectState(w, &done)
	}
}
