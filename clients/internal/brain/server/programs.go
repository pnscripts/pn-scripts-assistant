package server

import (
	"net/http"

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

	ok(w, map[string]any{"programs": out, "sources": sources, "first_looks": looks, "kinds": programs.Shown(records)})
}
