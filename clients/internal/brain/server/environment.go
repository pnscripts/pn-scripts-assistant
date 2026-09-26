package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/environs"
)

/*
 * What is on this machine, for the page that shows it.
 *
 * The same reading the model is given, so that what the assistant says it can
 * do and what the interface shows cannot disagree — two accounts of one
 * machine is how a program starts contradicting itself.
 *
 * And the one setting in this program that takes a capability away. Everything
 * found is available; this is a list of exceptions, empty on every brain until
 * somebody writes in it. That is the whole of the permission model for
 * capabilities: not a switch per thing, a short list of the things somebody
 * would rather it left alone.
 */
func (s *Server) handleEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx, stop := context.WithTimeout(r.Context(), environs.HowLongToAsk*3)
	defer stop()

	/*
	 * The reading that is already taken, unless the page asks for a new one.
	 *
	 * Reading the machine is a second or more — thirty programs, each asked
	 * what it is — and the page opens this panel every time somebody visits
	 * the System tab. Doing it on every visit made the list arrive after the
	 * panel, which is the one way a list of what is here can be worse than
	 * no list: it looks like nothing is here.
	 *
	 * "Look again" is a button, and switching something off takes a new
	 * reading anyway.
	 */
	things := s.brain.World().All(ctx)

	if r.URL.Query().Get("again") != "" {
		things = s.brain.World().Again(ctx)
	}

	off := map[string]bool{}

	for _, id := range s.brain.Cfg.TurnedOff {
		off[strings.TrimSpace(id)] = true
	}

	type shown struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Version string `json:"version,omitempty"`
		Path    string `json:"path,omitempty"`
		Why     string `json:"why,omitempty"`
		Needs   string `json:"needs,omitempty"`
		Local   bool   `json:"local"`
		Off     bool   `json:"off"`
	}

	out := make([]shown, 0, len(things))

	for _, thing := range things {
		out = append(out, shown{
			ID: thing.ID, Kind: string(thing.Kind), Title: thing.Title,
			State: string(thing.State), Version: thing.Version, Path: thing.Path,
			Why: thing.Why, Needs: thing.Needs, Local: thing.Local,
			Off: off[thing.ID],
		})
	}

	/*
	 * Something switched off that is no longer on the machine is still shown
	 * as switched off, so it can be switched back on.
	 *
	 * Otherwise uninstalling something would strand the decision about it:
	 * invisible in the list, still in the settings, and still taking effect
	 * the day it was installed again.
	 */
	for id := range off {
		if !known(things, id) {
			out = append(out, shown{ID: id, Title: id, State: string(environs.Unknown), Off: true})
		}
	}

	ok(w, map[string]any{"things": out, "off": s.brain.Cfg.TurnedOff})
}

/*
 * handleEnvironmentOff switches one thing off, or back on.
 *
 * By id, one at a time, because that is how somebody decides it: they are
 * looking at a list and they mean this one. Saved to the settings file, so it
 * survives a restart — which the first version of this did not, and a setting
 * that forgets is worse than none.
 */
func (s *Server) handleEnvironmentOff(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID  string `json:"id"`
		Off bool   `json:"off"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "that could not be read: "+err.Error())

		return
	}

	id := strings.TrimSpace(body.ID)

	if id == "" {
		fail(w, http.StatusBadRequest, "say which one")

		return
	}

	kept := make([]string, 0, len(s.brain.Cfg.TurnedOff)+1)

	for _, each := range s.brain.Cfg.TurnedOff {
		if !strings.EqualFold(strings.TrimSpace(each), id) {
			kept = append(kept, each)
		}
	}

	if body.Off {
		kept = append(kept, id)
	}

	s.brain.Cfg.TurnedOff = kept

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, "that could not be saved: "+err.Error())

		return
	}

	// Read the machine again, so the answer already reflects the decision
	// rather than the page having to ask twice.
	ctx, stop := context.WithTimeout(r.Context(), environs.HowLongToAsk*3)
	defer stop()

	s.brain.World().Again(ctx)

	ok(w, map[string]any{"off": s.brain.Cfg.TurnedOff})
}

func known(things []environs.Thing, id string) bool {
	for _, thing := range things {
		if thing.ID == id {
			return true
		}
	}

	return false
}
