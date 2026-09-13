package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Importing a job classification, from a button.
 *
 * Never on its own and never from the network — see occupations.Source. What
 * this adds is the part a person sees: it starts in the background, says which
 * pass it is on and how far into it, can be stopped, and says afterwards what
 * it did and what it could not settle. Pressing it again after stopping picks
 * up where it was, because every row already written is found and left.
 */

// importJobs is which background job is doing which import, so Stop reaches
// the one that is running. A run from before a restart has no job, and its
// row already says it was cut off.
var importJobs sync.Map

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
		From   string `json:"from"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	run := map[string]func(context.Context, *store.DB, *occupations.Source, func(occupations.Progress)) (occupations.Progress, error){
		occupations.ESCO: occupations.ImportESCO,
		occupations.ONET: occupations.ImportONET,
	}[strings.ToLower(strings.TrimSpace(body.Source))]

	if run == nil {
		fail(w, http.StatusBadRequest, "that is esco or onet")

		return
	}

	// Opened here as well as in the work, so a wrong path is said at once
	// rather than as a failed import a minute later.
	src, err := occupations.OpenSource(body.From)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	id, err := s.brain.DB.StartImport(body.Source, body.From)
	if err != nil {
		src.Close()
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	job, err := s.brain.Jobs.StartSilent("importing "+body.Source, func(ctx context.Context) (string, error) {
		defer src.Close()
		defer importJobs.Delete(id)

		p, err := run(ctx, s.brain.DB, src, func(p occupations.Progress) {
			s.brain.DB.ImportProgress(id, p.Stage, p.Done, p.Of, p.So, p.Notes)
		})

		s.brain.DB.ImportProgress(id, p.Stage, p.Done, p.Of, p.So, p.Notes)

		switch {
		case ctx.Err() != nil:
			s.brain.DB.FinishImport(id, store.ImportStopped, "you stopped it")
		case err != nil:
			s.brain.DB.FinishImport(id, store.ImportFailed, err.Error())
		default:
			s.brain.DB.FinishImport(id, store.ImportDone, "")
		}

		return "", err
	})
	if err != nil {
		src.Close()
		s.brain.DB.FinishImport(id, store.ImportFailed, err.Error())
		fail(w, http.StatusConflict, err.Error())

		return
	}

	importJobs.Store(id, job.ID)

	ok(w, map[string]any{"id": id})
}

func (s *Server) handleImports(w http.ResponseWriter, r *http.Request) {
	runs, err := s.brain.DB.Imports(5)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"imports": runs})
}

func (s *Server) handleStopImport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "that is not an import")

		return
	}

	job, running := importJobs.Load(id)
	if !running {
		fail(w, http.StatusNotFound, "that import is not running")

		return
	}

	if err := s.brain.Jobs.Stop(job.(int64)); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"stopping": id})
}
