package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

/*
 * Work that outlives the sentence that asked for it.
 *
 * The source of truth for all of this is the tasks and task_steps rows, not
 * anything held in memory — so the view is right after a reload, after a
 * restart, and while a task is parked waiting for a decision. Polling progress
 * could never have been any of those: it holds one current step for the whole
 * program, and a task is several steps that have to still be there tomorrow.
 */
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if s.brain.Tasks == nil {
		ok(w, map[string]any{"tasks": []any{}})

		return
	}

	live, err := s.brain.Tasks.Live()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Recently finished ones too. A task that ended thirty seconds ago is the
	// thing somebody is most likely to be looking for, and a list that drops
	// it the moment it finishes looks like it lost the work.
	recent, err := s.brain.DB.Tasks(nil, 12)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"tasks": live, "recent": recent})
}

// handleTask is one task with every step, its evidence and its verdicts.
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not a task number")

		return
	}

	task, err := s.brain.DB.TaskWithSteps(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if task == nil {
		fail(w, http.StatusNotFound, "there is no task with that number")

		return
	}

	ok(w, task)
}

/*
 * handleStartTask begins a piece of work from the view rather than from a
 * sentence.
 *
 * The route that does not depend on a guess. Whatever the planner eventually
 * recognises, somebody who wants a job done needs a way to say so that cannot
 * be talked out of it.
 */
func (s *Server) handleStartTask(w http.ResponseWriter, r *http.Request) {
	if s.brain.Tasks == nil {
		fail(w, http.StatusServiceUnavailable, "tasks are not available")

		return
	}

	var body struct {
		Request        string `json:"request"`
		ConversationID int64  `json:"conversation_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	if body.Request == "" {
		fail(w, http.StatusBadRequest, "say what the job is")

		return
	}

	conversation := body.ConversationID

	if conversation == 0 {
		// Somewhere to report back to. A task with nowhere to say it finished
		// is a task nobody hears about.
		latest, err := s.brain.DB.LatestConversation()
		if err == nil && latest != nil {
			conversation = latest.ID
		}
	}

	task, started, err := s.brain.Tasks.Take(r.Context(), conversation, body.Request,
		s.brain.Cfg.DefaultProvider, true)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if !started {
		fail(w, http.StatusBadRequest, "that reads like a question rather than a job")

		return
	}

	ok(w, task)
}

// handleStopTask ends a task at the next thing it does.
func (s *Server) handleStopTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not a task number")

		return
	}

	if s.brain.Tasks == nil {
		fail(w, http.StatusServiceUnavailable, "tasks are not available")

		return
	}

	if err := s.brain.Tasks.Stop(id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"stopped": id})
}

// handleResumeTask picks up a task that was parked — by a shutdown, or by a
// machine too busy to start it at the time.
func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not a task number")

		return
	}

	if s.brain.Tasks == nil {
		fail(w, http.StatusServiceUnavailable, "tasks are not available")

		return
	}

	if err := s.brain.Tasks.Resume(id); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"resumed": id})
}
