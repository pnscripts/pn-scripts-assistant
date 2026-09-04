package server

import (
	"encoding/json"
	"net/http"

	"pn-brain/internal/brain/protect"
)

/*
 * What the brain stops and asks about, and what it has learned.
 *
 * Reachable from the interface and from setup, and from nowhere else. There is
 * deliberately no tool for any of this: a page the brain reads could otherwise
 * ask it to switch off the rule protecting the keys and then read them, which
 * is the entire attack this exists to interrupt.
 *
 * That is also why it is safe for approving a file to be remembered
 * automatically. The remembering is visible here and any of it can be taken
 * back, and nothing but a person can put anything into it.
 */
func (s *Server) handleProtection(w http.ResponseWriter, r *http.Request) {
	chosen := protect.InForce()

	rules := make([]map[string]any, 0, len(protect.BuiltIn))

	for _, rule := range protect.BuiltIn {
		rules = append(rules, map[string]any{
			"id":    rule.ID,
			"what":  rule.What,
			"why":   rule.Why,
			"fixed": rule.Fixed,
			"on":    rule.Fixed || protect.On(rule.ID),
		})
	}

	ok(w, map[string]any{
		"rules": rules,
		"yours": chosen.Extra,
		// What it has been told is fine, which is the half that grows by
		// itself and therefore the half worth being able to see.
		"learned": chosen.Allowed,
	})
}

func (s *Server) handleSetProtection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		// Each is optional; only what is sent is changed, so the panel can
		// send one toggle without having to restate the whole list.
		Off     *[]string `json:"off"`
		Yours   *[]string `json:"yours"`
		Learned *[]string `json:"learned"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "could not read that request")

		return
	}

	chosen := protect.InForce()

	if body.Off != nil {
		chosen.Off = *body.Off
	}

	if body.Yours != nil {
		chosen.Extra = *body.Yours
	}

	if body.Learned != nil {
		chosen.Allowed = *body.Learned
	}

	if err := protect.Save(s.brain.Root, chosen); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	s.log.Info("what the brain asks about was changed",
		"switched off", len(chosen.Off), "yours", len(chosen.Extra), "allowed", len(chosen.Allowed))

	s.handleProtection(w, r)
}
