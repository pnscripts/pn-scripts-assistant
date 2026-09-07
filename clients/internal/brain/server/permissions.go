package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * What the brain is allowed to do on this machine.
 *
 * A separate page from Privacy because they are separate questions, and
 * putting them together is what made the only way to let the brain do more be
 * to let more leave. Privacy is what may reach somebody else's computer; this
 * is what may happen on yours.
 */

// handlePermissions lists every capability, what it does, and what has been
// decided about it.
//
// Every capability, not only the ones with a decision: a permissions page that
// shows what you have already agreed to answers the wrong question. The one
// somebody has is "what can this thing do", and that list has to be complete
// or it is reassurance rather than information.
func (s *Server) handlePermissions(w http.ResponseWriter, r *http.Request) {
	granted := map[string]permits.Grant{}

	if s.brain.Permits != nil {
		for _, g := range s.brain.Permits.List() {
			granted[g.Tool] = g
		}
	}

	forThisRun := map[string]bool{}

	if s.brain.Permits != nil {
		for _, name := range s.brain.Permits.OnlyForThisRun() {
			forThisRun[name] = true
		}
	}

	type capability struct {
		Name     string `json:"name"`
		What     string `json:"what"`
		Changes  bool   `json:"changes_something"`
		Decision string `json:"decision"`
		Why      string `json:"why,omitempty"`
		ThisRun  bool   `json:"only_this_run,omitempty"`

		// Hidden is a capability the privacy setting is keeping out of reach,
		// shown greyed rather than omitted — "why can it not do that" is a
		// question with two different answers and a person deserves to know
		// which one they are looking at.
		Hidden bool `json:"hidden_by_privacy,omitempty"`
	}

	out := []capability{}

	for _, t := range s.brain.Agent.Registry.All() {
		c := capability{
			Name:     t.Name(),
			What:     t.Description(),
			Changes:  t.Risk() == tools.Mutating,
			Decision: string(permits.Ask),
			ThisRun:  forThisRun[t.Name()],
		}

		if !c.Changes {
			c.Decision = string(permits.Allow)
		}

		if g, ok := granted[t.Name()]; ok {
			c.Decision = string(g.Answer)
			c.Why = g.Why
		}

		if s.brain.Agent.OffLimits != nil && s.brain.Agent.OffLimits(t.Name()) {
			c.Hidden = true
		}

		out = append(out, c)
	}

	ok(w, map[string]any{
		"freedom":      string(s.brain.Freedom()),
		"privacy":      s.brain.Cfg.Privacy,
		"capabilities": out,
	})
}

/*
 * handleDecide records a standing decision about one capability.
 *
 * From this page only, never from a conversation. A model that can widen its
 * own permissions by being asked to has no permissions at all: anything that
 * can talk to it — including a web page it was told to read — can ask.
 */
func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tool   string `json:"tool"`
		Answer string `json:"answer"`
		Why    string `json:"why"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	if s.brain.Permits == nil {
		fail(w, http.StatusInternalServerError, "There is nowhere to record that.")

		return
	}

	tool := strings.TrimSpace(body.Tool)

	if tool == "" {
		fail(w, http.StatusBadRequest, "Which capability?")

		return
	}

	switch permits.Answer(strings.TrimSpace(body.Answer)) {
	case permits.Allow:
		if err := s.brain.Permits.Remember(tool, permits.Allow, body.Why); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

	case permits.Refuse:
		if err := s.brain.Permits.Remember(tool, permits.Refuse, body.Why); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

	case permits.Ask:
		// Back to asking, which is what removing a decision means.
		if err := s.brain.Permits.Forget(tool); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

	default:
		fail(w, http.StatusBadRequest, "That is allow, refuse or ask.")

		return
	}

	s.brain.Log.Info("permission changed", "tool", tool, "answer", body.Answer)

	ok(w, map[string]any{"tool": tool, "answer": body.Answer})
}

// handleFreedom changes how much may be done without asking each time.
func (s *Server) handleFreedom(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level string `json:"level"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	level, err := s.brain.UseFreedom(body.Level)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"freedom": string(level), "means": permits.Means(level)})
}
