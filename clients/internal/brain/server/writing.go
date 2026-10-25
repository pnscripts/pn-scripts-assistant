package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/proof"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The Writing page: proofread, compare two versions, rewrite in a tone.
 *
 * Proofreading and comparing are rules and answer at once, with no model and
 * nothing leaving the machine. Rewriting asks the model through the brain,
 * which is where privacy is enforced, and comes back proofread and compared
 * with the original so nothing changes without being shown.
 */

// mostText is the longest text the page sends: a long chapter.
const mostText = 120_000

type writingRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Tone     string `json:"tone"`
	MaxWords int    `json:"max_words"`
	Chosen   []int  `json:"chosen"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

func (s *Server) readWriting(w http.ResponseWriter, r *http.Request) (writingRequest, bool) {
	var body writingRequest

	if !decodeUpTo(w, r, &body, 4*mostText) {
		return body, false
	}

	for _, t := range []string{body.Text, body.Before, body.After} {
		if utf8.RuneCountInString(t) > mostText {
			fail(w, http.StatusRequestEntityTooLarge, "That text is too long to check in one go; split it into parts.")

			return body, false
		}
	}

	return body, true
}

// handleWritingTones lists the tone presets in both languages.
func (s *Server) handleWritingTones(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"tones": proof.Tones})
}

// handleWritingCheck proofreads a text, against a tone and a length if given.
func (s *Server) handleWritingCheck(w http.ResponseWriter, r *http.Request) {
	body, good := s.readWriting(w, r)
	if !good {
		return
	}

	if strings.TrimSpace(body.Text) == "" {
		fail(w, http.StatusBadRequest, "There is no text to check.")

		return
	}

	ok(w, tools.ProofReport(body.Text, body.Language, body.Tone, body.MaxWords))
}

// handleWritingApply makes the sure corrections and the ones chosen.
func (s *Server) handleWritingApply(w http.ResponseWriter, r *http.Request) {
	body, good := s.readWriting(w, r)
	if !good {
		return
	}

	lang := body.Language
	if lang != "en" && lang != "bg" {
		lang = proof.Language(body.Text)
	}

	found := proof.Check(body.Text, lang)

	ok(w, map[string]any{"text": proof.Apply(body.Text, found, body.Chosen...)})
}

// handleWritingCompare compares two versions word by word.
func (s *Server) handleWritingCompare(w http.ResponseWriter, r *http.Request) {
	body, good := s.readWriting(w, r)
	if !good {
		return
	}

	ok(w, proof.Compare(body.Before, body.After))
}

// handleWritingRewrite rewrites a text in a tone with the model.
func (s *Server) handleWritingRewrite(w http.ResponseWriter, r *http.Request) {
	body, good := s.readWriting(w, r)
	if !good {
		return
	}

	done, err := s.brain.Rewrite(r.Context(), body.Text, body.Tone, body.Language, body.MaxWords)
	if errors.Is(err, brain.ErrNoModel) {
		fail(w, http.StatusServiceUnavailable, "No language model is available to rewrite with. Proofreading still works.")

		return
	}

	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, done)
}

func decodeUpTo(w http.ResponseWriter, r *http.Request, into any, most int64) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, most)).Decode(into); err != nil {
		fail(w, http.StatusBadRequest, "That request could not be read.")

		return false
	}

	return true
}
