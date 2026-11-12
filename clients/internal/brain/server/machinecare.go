package server

import (
	"errors"
	"math"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/machinecare"
)

// handleMachineCare is the whole look, for the card on the System page.
func (s *Server) handleMachineCare(w http.ResponseWriter, r *http.Request) {
	ok(w, s.brain.MachineCare(r.Context()))
}

// handleStopLeftover stops one process the look listed, by its number and
// the command it showed — pressed by a person at the computer.
func (s *Server) handleStopLeftover(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PID     int    `json:"pid"`
		Command string `json:"command"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if err := s.brain.StopLeftover(body.PID, body.Command); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, machinecare.ErrNotALeftover) {
			code = http.StatusConflict
		}

		fail(w, code, err.Error())

		return
	}

	ok(w, map[string]any{"stopped": body.PID})
}

// handleRestartWatchState is how the watch stands, without reading the
// machine: what the card asks once a minute while one is going.
func (s *Server) handleRestartWatchState(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"watch": s.brain.RestartWatch()})
}

/*
 * handleRestartWatch starts "tell me when I can restart" for the hours
 * given (zero for the default), or ends it with stop. One watch at a time:
 * asked again, the one going is returned. The outcome is said out loud as a
 * reminder; started here, no conversation is told.
 */
func (s *Server) handleRestartWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hours float64 `json:"hours"`
		Stop  bool    `json:"stop"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	if body.Stop {
		watch, was := s.brain.StopRestartWatch()

		ok(w, map[string]any{"watch": watch, "stopped": was})

		return
	}

	if math.IsNaN(body.Hours) || body.Hours < 0 || body.Hours > 12 {
		fail(w, http.StatusBadRequest, machinecare.ErrWatchTooLong.Error())

		return
	}

	watch, already, err := s.brain.WatchForRestart(0, time.Duration(body.Hours*float64(time.Hour)))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"watch": watch, "already": already})
}
