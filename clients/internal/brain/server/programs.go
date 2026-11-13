package server

import (
	"net/http"

	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * handleProgramInventory is the programs on this computer, present and
 * removed, and how each source is kept current: ?kind= narrows it to app,
 * deb, snap or flatpak. Read only.
 */
func (s *Server) handleProgramInventory(w http.ResponseWriter, r *http.Request) {
	kind := programs.Kind(r.URL.Query().Get("kind"))

	switch kind {
	case "", programs.App, programs.Deb, programs.Snap, programs.Flatpak:
	default:
		fail(w, http.StatusBadRequest, "kind is app, deb, snap or flatpak")

		return
	}

	records, sources, err := s.brain.ProgramInventory()
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

	ok(w, map[string]any{"programs": out, "sources": sources})
}
