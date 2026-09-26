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

		/*
		 * Hidden is a capability that is out of reach, and why.
		 *
		 * Shown greyed rather than omitted, because "why can it not do that"
		 * has several different answers and somebody deserves to know which
		 * one they are looking at. It used to have one — privacy — and the
		 * field said so in its name; now it can also be something its owner
		 * switched off, or something whose engine is not installed, so the
		 * reason travels with it rather than being assumed by whoever reads
		 * the field.
		 */
		Hidden bool   `json:"hidden,omitempty"`
		Why2   string `json:"hidden_why,omitempty"`
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
			c.Hidden, c.Why2 = true, s.whyHidden(t.Name())
		}

		out = append(out, c)
	}

	ok(w, map[string]any{
		"freedom": string(s.brain.Freedom()),
		"privacy": s.brain.Cfg.Privacy,

		// Its own switch, on this page, because it is a permission and not a
		// privacy setting: it asks public servers questions that contain
		// nothing of the owner's.
		"look_online":  s.brain.Cfg.LookOnline,
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

/*
 * handleLookOnline switches looking things up on or off.
 *
 * Separate from privacy on purpose and deliberately not part of it. Privacy is
 * where your words go; this is whether the program may ask a public server
 * whether a newer version exists. They were one setting, so keeping a
 * conversation on this machine also meant never being told an update existed —
 * and nobody makes that second decision on purpose.
 */
func (s *Server) handleLookOnline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "I could not read that.")

		return
	}

	cfg := s.brain.Cfg
	cfg.LookOnline = body.On

	if err := cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "I could not write it down: "+err.Error())

		return
	}

	s.brain.Cfg = cfg

	s.brain.Log.Info("looking things up online changed", "on", body.On)

	ok(w, map[string]any{"look_online": body.On})
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

/*
 * whyHidden is the reason a capability is out of reach, in words.
 *
 * Three reasons and they are not alike: a privacy setting that its owner can
 * change in a moment, a decision they made about this machine, and something
 * that is not installed. Telling somebody the wrong one sends them to the
 * wrong panel.
 */
func (s *Server) whyHidden(tool string) string {
	for _, id := range s.brain.Cfg.TurnedOff {
		if strings.EqualFold(strings.TrimSpace(id), tool) ||
			strings.EqualFold(strings.TrimSpace(id), "tool:"+tool) {
			return "switched off in What is on this machine"
		}
	}

	if s.brain.Agent != nil && s.brain.Agent.Registry != nil {
		for _, needed := range s.brain.Agent.Registry.Needing(tool) {
			for _, id := range s.brain.Cfg.TurnedOff {
				if strings.EqualFold(strings.TrimSpace(id), needed) {
					return needed + " is switched off in What is on this machine"
				}
			}
		}
	}

	return "out of reach at this privacy setting"
}
