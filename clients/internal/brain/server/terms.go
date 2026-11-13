package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/terms"
)

/*
 * The terms of use, before anything else.
 *
 * Until its owner has accepted them, the program serves the agreement page
 * and what that page needs, and refuses everything else with a reason: no
 * conversation, no greeting, no listening, no reading of what it knows, no
 * settings. The brain's own background work waits too (brain.Start), and
 * starts when they are accepted (brain.Agreed).
 *
 * Inside the guard, so a stranger off the network is still refused for who
 * they are before anything is said about terms.
 */

// TermsOnline is where the documentation keeps the same terms.
var TermsOnline = map[string]string{
	"en": "https" + "://github.com/pnscripts/pn-scripts-assistant/blob/main/docs/guide/terms.md",
	"bg": "https" + "://github.com/pnscripts/pn-scripts-assistant/blob/main/docs/guide/terms.bg.md",
}

// beforeTheTerms is what answers while the terms are not accepted: the
// agreement page's own needs, and the window's.
var beforeTheTerms = map[string]bool{
	"GET /api/terms":        true,
	"POST /api/terms":       true,
	"GET /api/ui-language":  true,
	"GET /api/v1/version":   true,
	"POST /api/present":     true, // a second copy bringing this window forward
	"POST /api/open-source": true, // the terms in the documentation, in the browser
}

// termsAccepted is whether its owner has accepted the terms in force, read
// from the brain's folder until they have and remembered after.
func (s *Server) termsAccepted() bool {
	if s.agreed.Load() {
		return true
	}

	if terms.Accepted(s.brain.Root) {
		s.agreed.Store(true)

		return true
	}

	return false
}

func (s *Server) untilAgreed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.termsAccepted() || len(r.URL.Path) < 5 || r.URL.Path[:5] != "/api/" ||
			beforeTheTerms[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "The terms of use have not been accepted yet: open the assistant's window and accept them first.",
			"terms": "not accepted",
		})
	})
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"version":  terms.Version,
		"dated":    terms.Dated,
		"accepted": s.termsAccepted(),
		"en":       terms.In("en"),
		"bg":       terms.In("bg"),
		"online":   TermsOnline,
	}

	if rec, err := terms.Load(s.brain.Root); err == nil && rec.Version > 0 {
		out["record"] = rec
	}

	ok(w, out)
}

/*
 * handleAcceptTerms records that the terms were accepted, and starts the
 * work the brain held back for them.
 *
 * The version the page showed comes with the acceptance, and only the terms
 * in force are accepted: a page left open across an update showed other
 * words, and is told to show the new ones.
 */
func (s *Server) handleAcceptTerms(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version  int    `json:"version"`
		Language string `json:"language"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "That is not an acceptance of the terms.")

		return
	}

	rec, err := terms.Accept(s.brain.Root, body.Version, body.Language, time.Now())
	if errors.Is(err, terms.ErrOtherVersion) {
		fail(w, http.StatusConflict, "The terms have changed since this page was opened. Reload it to read the ones in force.")

		return
	}

	if err != nil {
		s.log.Error("the acceptance of the terms could not be kept", "error", err)
		fail(w, http.StatusInternalServerError, "The acceptance could not be saved: "+err.Error())

		return
	}

	s.agreed.Store(true)
	s.log.Info("the terms of use were accepted", "version", rec.Version, "language", rec.Language)
	s.brain.Agreed()

	ok(w, map[string]any{"accepted": true, "record": rec})
}
