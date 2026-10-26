package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/research"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Research page: a question looked into with sources, the reports kept,
 * each one read with its claims check and its sources, downloaded as
 * Markdown, or removed; and somebody's own text checked against the sources
 * it cites.
 *
 * A source goes to the page without the text that was read from it — that
 * is kept for checking again, and is the web page's, not the report's.
 */

// researchFailed says why research could not be done.
func researchFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tools.ErrNoWeb):
		fail(w, http.StatusForbidden, "Research reads the web, and the privacy setting keeps this machine to itself. "+
			"Allow the web in Control to use it.")
	case errors.Is(err, brain.ErrNoModel), errors.Is(err, tools.ErrNoResearchModel):
		fail(w, http.StatusServiceUnavailable, "No language model is available to write the report.")
	case errors.Is(err, tools.ErrNothingRead):
		fail(w, http.StatusNotFound, "No source could be found or read for that question. Try other words.")
	default:
		fail(w, http.StatusBadGateway, err.Error())
	}
}

// withoutText is the sources as the page may see them.
func withoutText(sources []research.Source) []research.Source {
	out := make([]research.Source, len(sources))

	for i, s := range sources {
		s.Text = ""
		out[i] = s
	}

	return out
}

func (s *Server) handleResearchList(w http.ResponseWriter, _ *http.Request) {
	list, err := s.brain.DB.ResearchReports()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"reports": list, "web": s.brain.Mode.AllowsWeb(), "depths": research.Depths})
}

func (s *Server) researchOf(w http.ResponseWriter, r *http.Request) (store.ResearchReport, bool) {
	id, good := idOf(w, r)
	if !good {
		return store.ResearchReport{}, false
	}

	rep, err := s.brain.DB.Research(id)
	if errors.Is(err, store.ErrNoResearch) {
		fail(w, http.StatusNotFound, "That report is not there.")

		return rep, false
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return rep, false
	}

	return rep, true
}

func (s *Server) handleResearchOne(w http.ResponseWriter, r *http.Request) {
	rep, good := s.researchOf(w, r)
	if !good {
		return
	}

	var (
		sources []research.Source
		claims  []research.Claim
	)

	_ = json.Unmarshal(rep.Sources, &sources)
	_ = json.Unmarshal(rep.Claims, &claims)

	rep.Sources, rep.Claims = nil, nil

	ok(w, map[string]any{"report": rep, "sources": withoutText(sources), "claims": claims})
}

func (s *Server) handleResearchDownload(w http.ResponseWriter, r *http.Request) {
	rep, good := s.researchOf(w, r)
	if !good {
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="research-%d.md"`, rep.ID))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(rep.Report))
}

func (s *Server) handleResearchRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question string `json:"question"`
		Depth    string `json:"depth"`
		Language string `json:"language"`
	}

	if !decodeUpTo(w, r, &body, 1<<12) {
		return
	}

	if n := len([]rune(body.Question)); n < 3 || n > 500 {
		fail(w, http.StatusBadRequest, "Ask the question in 3 to 500 characters.")

		return
	}

	if _, good := research.DepthNamed(body.Depth); !good {
		fail(w, http.StatusBadRequest, "The depth must be quick or thorough.")

		return
	}

	done, err := s.brain.Researching().Research(r.Context(), body.Question, body.Depth, body.Language)
	if err != nil {
		researchFailed(w, err)

		return
	}

	rep := done.Report
	rep.Sources, rep.Claims = nil, nil

	ok(w, map[string]any{
		"report": rep, "sources": withoutText(done.Sources), "claims": done.Claims,
		"summary": done.Summary, "queries": done.Queries,
	})
}

func (s *Server) handleResearchCheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text     string   `json:"text"`
		Sources  []string `json:"sources"`
		Language string   `json:"language"`
	}

	if !decodeUpTo(w, r, &body, 1<<17) {
		return
	}

	done, err := s.brain.Researching().CheckClaims(r.Context(), body.Text, body.Sources, body.Language)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, done)
}

func (s *Server) handleResearchRemove(w http.ResponseWriter, r *http.Request) {
	id, good := idOf(w, r)
	if !good {
		return
	}

	if err := s.brain.DB.RemoveResearch(id); errors.Is(err, store.ErrNoResearch) {
		fail(w, http.StatusNotFound, "That report is not there.")

		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"removed": id})
}
