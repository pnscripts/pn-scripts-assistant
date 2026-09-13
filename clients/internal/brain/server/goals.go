package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/store"
)

// What the work is for, and how it has actually been going.
func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	goals, err := s.brain.DB.Goals()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"goals": goals})
}

func (s *Server) handleSaveGoal(w http.ResponseWriter, r *http.Request) {
	var goal store.Goal

	if err := json.NewDecoder(r.Body).Decode(&goal); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	goal.Name = strings.TrimSpace(goal.Name)

	if goal.Name == "" {
		fail(w, http.StatusBadRequest, "say what the goal is")

		return
	}

	if goal.EveryDays < 0 {
		goal.EveryDays = 0
	}

	if goal.ID == 0 {
		id, err := s.brain.DB.NewGoal(goal)
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

		ok(w, map[string]any{"id": id})

		return
	}

	if err := s.brain.DB.UpdateGoal(goal); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"id": goal.ID})
}

func (s *Server) handleForgetGoal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not a goal number")

		return
	}

	if err := s.brain.DB.ForgetGoal(id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// The work done towards it stays. It happened, and a record that
	// disappears when somebody tidies up is not a record.
	ok(w, map[string]any{"forgotten": id})
}

// handleWorkOnGoal starts a piece of work towards one, now, because somebody
// asked — which is how every goal starts unless it was told to start itself.
func (s *Server) handleWorkOnGoal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not a goal number")

		return
	}

	task, err := s.brain.WorkOnGoal(r.Context(), id)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	if task == nil {
		fail(w, http.StatusBadRequest,
			"that did not turn into anything that could be worked on")

		return
	}

	ok(w, task)
}

/*
 * handleReview is how the work has actually been going.
 *
 * Over a window, because "ever" flatters: a machine that was set up wrongly
 * for a week and has been fine since should not read as half broken, and one
 * that worked for a month and broke yesterday should not read as fine.
 */
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	days := 7

	if given := r.URL.Query().Get("days"); given != "" {
		if n, err := strconv.Atoi(given); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	review, err := s.brain.DB.HowItHasBeenGoing(time.Now().AddDate(0, 0, -days))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"review": review, "days": days})
}
