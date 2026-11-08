package server

import (
	"errors"
	"net/http"

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
