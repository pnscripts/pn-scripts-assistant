package server

import (
	"net/http"

	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/provision"
)

/*
 * How work is done, for the Organisation view: what is on this machine and
 * how each thing stands — signed in or not, how much of its allowance is
 * left or that nobody says — the owner's policy, what would be chosen to
 * write code now and why, and what came of the last pieces of work.
 */

// handleResources is everything the orchestrator sees, and its choice now.
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	o := s.brain.Orchestrator()

	list, machine := o.Resources(r.Context())

	decision := o.Decide(r.Context(), orchestrator.Need{Work: orchestrator.Code, Files: true}, orchestrator.Override{})

	runs, _ := s.brain.DB.Runs("", 30)

	ok(w, map[string]any{
		"machine": machine, "resources": list, "policy": o.Policy(),
		"coding": map[string]any{"decision": decision, "explained": decision.Explain()},
		"runs":   runs,
	})
}

// handlePolicy saves the owner's policy — a decision, so from the desk only.
func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	var p orchestrator.Policy

	if !decode(w, r, &p) {
		return
	}

	if err := orchestrator.SavePolicy(s.brain.Root, p); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	s.brain.Orchestrator().Forget()

	ok(w, map[string]any{"policy": p})
}

// handleRefresh forgets what was found, so the next look finds it afresh —
// after its owner has signed in to something, say.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	s.brain.Orchestrator().Forget()
	provision.ForgetUnityLicence()

	s.handleResources(w, r)
}
