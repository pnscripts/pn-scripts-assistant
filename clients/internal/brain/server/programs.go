package server

import (
	"errors"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * handleProgramInventory is the programs on this computer, present and
 * removed, how each source is kept current and when each was first read
 * (what that look found was already there), and the sources worth showing
 * (kinds: this system's, then any a remembered program came from): ?kind=
 * narrows the programs to one source. Read only.
 */
func (s *Server) handleProgramInventory(w http.ResponseWriter, r *http.Request) {
	kind := programs.Kind(r.URL.Query().Get("kind"))

	if kind != "" && !programs.Known(kind) {
		fail(w, http.StatusBadRequest, "kind is app, deb, snap, flatpak, win or msix")

		return
	}

	records, sources, err := s.brain.ProgramInventory()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	looks, err := s.brain.DB.ProgramFirstLooks()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	out := make([]programs.Record, 0, len(records))

	for _, rec := range records {
		if kind == "" || rec.Kind == kind {
			out = append(out, rec)
		}
	}

	if sources == nil {
		sources = []programs.SourceState{}
	}

	decided, err := s.brain.ProgramPermits()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"programs": out, "sources": sources, "first_looks": looks, "kinds": programs.Shown(records),
		"permits": cardPermits(records, decided), "verbs": programs.InUse,
		// Whether a standing "always" is acted on: not on "ask every time".
		"honoured": s.brain.Freedom() != permits.AskEveryTime,
	})
}

// cardPermit is one standing decision as a card shows it: asleep is an
// "always" given to another version than the one here.
type cardPermit struct {
	programs.Permit
	Asleep bool `json:"asleep,omitempty"`
}

// cardPermits is every program's standing decisions by its key (kind:id).
func cardPermits(records []programs.Record, decided map[int64][]programs.Permit) map[string][]cardPermit {
	out := map[string][]cardPermit{}

	for _, r := range records {
		for _, p := range decided[r.RowID] {
			out[r.Key()] = append(out[r.Key()], cardPermit{Permit: p, Asleep: p.Asleep(r.Version)})
		}
	}

	return out
}

/*
 * handleProgramPermit is a decision from a program's card: {kind, id,
 * verb, answer} with answer ask, allow or never. At the desk only (guard):
 * what another program may be made to do is its owner's decision.
 */
func (s *Server) handleProgramPermit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   programs.Kind   `json:"kind"`
		ID     string          `json:"id"`
		Verb   programs.Verb   `json:"verb"`
		Answer programs.Answer `json:"answer"`
	}

	if !decode(w, r, &body) {
		return
	}

	switch {
	case !programs.Known(body.Kind) || strings.TrimSpace(body.ID) == "":
		fail(w, http.StatusBadRequest, "say which program: kind and id")

		return
	case !programs.KnownVerb(body.Verb):
		fail(w, http.StatusBadRequest, "verb is one of: start, control-input, change, read, export, send, uninstall")

		return
	case !programs.KnownAnswer(body.Answer):
		fail(w, http.StatusBadRequest, "answer is ask, allow or never")

		return
	}

	rec, err := s.brain.SetProgramPermit(body.Kind, body.ID, body.Verb, body.Answer)
	if errors.Is(err, brain.ErrNoSuchProgram) {
		fail(w, http.StatusNotFound, "there is no such program here")

		return
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	decided, err := s.brain.ProgramPermits()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"program": rec, "permits": cardPermits([]programs.Record{rec}, decided)[rec.Key()]})
}
