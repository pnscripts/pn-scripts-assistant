package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The door other programs ask through, and the switch at the desk for
 * what they may ask (package otherprograms).
 *
 * "pn-scripts-assistant mcp" puts every question of another program
 * through POST /api/outside/ask, with the name that program gave. One
 * door, so that three things happen in one place and cannot be forgotten
 * on a route: the switch is checked, the question is written on the
 * record with who asked, and only then is it answered, with exactly what
 * that question is for and nothing more.
 *
 * A POST, because asking writes the record. Like every route that changes
 * something it is refused to a web page on another site (Guard), so a page
 * open in a browser cannot ask as "another program"; and it is under
 * /api/outside, which a paired device with limited rights may not post to.
 */

// mostAsked is far more than a question to these tools needs.
const mostAsked = 16 << 10

// strictly reads one JSON object into into and nothing after it, refusing
// a field it does not know.
func strictly(w http.ResponseWriter, r *http.Request, into any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mostAsked))
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(into); err != nil {
		return err
	}

	if dec.More() {
		return errors.New("more than one request")
	}

	return nil
}

/*
 * fromAPage reports whether a browser sent the request from a page that
 * is not the assistant's own. Guard already refuses another site by its
 * Origin; a browser also says where a request began in Sec-Fetch-Site,
 * and here both are read, because the two things behind this file are
 * the ones a page must never do: speak as "another program", and decide
 * what other programs may ask.
 */
func fromAPage(r *http.Request, ownPageToo bool) bool {
	site := r.Header.Get("Sec-Fetch-Site")

	if ownPageToo {
		return site != "" || r.Header.Get("Origin") != ""
	}

	return site != "" && site != "same-origin" && site != "none"
}

// toldProgram is a program as another program is told of it: its name,
// version, source and whether it is here. Never where it is or how it is
// started.
type toldProgram struct {
	Kind      programs.Kind `json:"kind"`
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Version   string        `json:"version,omitempty"`
	Present   bool          `json:"present"`
	RemovedAt *time.Time    `json:"removed_at,omitempty"`
}

func (s *Server) handleOutsideAsk(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Client    struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"client"`
	}

	// A program says nothing about pages; anything that does is a browser,
	// and no page asks as another program, not even the assistant's own.
	if fromAPage(r, true) {
		fail(w, http.StatusForbidden, "Only a program on this computer asks here, not a page.")

		return
	}

	if err := strictly(w, r, &asked); err != nil {
		fail(w, http.StatusBadRequest, "That request could not be read.")

		return
	}

	subject, known := otherprograms.SubjectOf(asked.Tool)
	if !known {
		fail(w, http.StatusBadRequest, "That is not a question other programs may ask.")

		return
	}

	by := otherprograms.Caller(asked.Client.Name, asked.Client.Version)

	// What the question was given, read before anything is decided: a
	// question that cannot be read is not a question, and is not recorded.
	var kind programs.Kind

	if len(asked.Arguments) > 0 && string(asked.Arguments) != "null" {
		var arguments struct {
			Kind *string `json:"kind"`
		}

		dec := json.NewDecoder(bytes.NewReader(asked.Arguments))
		dec.DisallowUnknownFields()

		if err := dec.Decode(&arguments); err != nil || (arguments.Kind != nil && asked.Tool != "list_programs") {
			fail(w, http.StatusBadRequest, "That question does not take those arguments.")

			return
		}

		if arguments.Kind != nil {
			kind = programs.Kind(*arguments.Kind)
		}
	}

	if kind != "" && !programs.Known(kind) {
		fail(w, http.StatusBadRequest, "kind is app, deb, snap, flatpak, win or msix")

		return
	}

	given := ""
	if kind != "" {
		given = "kind: " + string(kind)
	}

	if subject != "" && !s.brain.OutsideMay(subject) {
		en, _ := otherprograms.NotAllowed(subject)

		// A refusal that cannot be written down is still a refusal.
		if err := s.brain.AskedFromOutside(asked.Tool, by, given, agent.DeedRefused, en); err != nil {
			s.log.Warn("could not record a question another program asked", "tool", asked.Tool, "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Its user has not allowed other programs to ask that.",
			"outside": "not allowed",
			"subject": string(subject),
		})

		return
	}

	// No record, no answer: what another program was told is always
	// written down first.
	noted := func(result string) bool {
		if err := s.brain.AskedFromOutside(asked.Tool, by, given, agent.DeedDone, result); err != nil {
			s.log.Warn("could not record a question another program asked; not answering it", "tool", asked.Tool, "error", err)
			fail(w, http.StatusInternalServerError, "The question could not be written on the record, so it was not answered.")

			return false
		}

		return true
	}

	switch asked.Tool {
	case "assistant_status":
		if noted("") {
			ok(w, map[string]any{"noted": true})
		}

	case "list_programs":
		records, _, err := s.brain.ProgramInventory()
		if err != nil {
			fail(w, http.StatusInternalServerError, "The list of programs could not be read.")

			return
		}

		told := make([]toldProgram, 0, len(records))

		for _, rec := range records {
			if kind == "" || rec.Kind == kind {
				told = append(told, toldProgram{Kind: rec.Kind, ID: rec.ID, Name: rec.Name, Version: toldVersion(rec.Version),
					Present: rec.Present, RemovedAt: rec.RemovedAt})
			}
		}

		if noted(fmt.Sprintf("Programs I told it about: %d.", len(told))) {
			ok(w, map[string]any{"programs": told})
		}
	}
}

// toldVersion is a version as another program is told it: a menu entry's
// fingerprint is not one.
func toldVersion(v string) string {
	if programs.IsPin(v) {
		return ""
	}

	return v
}

// outsideState is the switches and the last questions, as the page reads
// them.
func (s *Server) outsideState(w http.ResponseWriter) {
	switches, recent, err := s.brain.OutsideSwitches()
	if err != nil {
		fail(w, http.StatusInternalServerError, "What other programs may ask could not be read.")

		return
	}

	ok(w, map[string]any{"subjects": switches, "recent": recent})
}

func (s *Server) handleOutside(w http.ResponseWriter, _ *http.Request) { s.outsideState(w) }

// handleOutsideAllow sets one switch, at the desk only.
func (s *Server) handleOutsideAllow(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Subject string `json:"subject"`
		Allowed *bool  `json:"allowed"`
	}

	if fromAPage(r, false) {
		fail(w, http.StatusForbidden, "that request came from somewhere else")

		return
	}

	if err := strictly(w, r, &asked); err != nil || asked.Allowed == nil || !otherprograms.Known(otherprograms.Subject(asked.Subject)) {
		fail(w, http.StatusBadRequest, "Say which (programs) and whether it is allowed (true or false).")

		return
	}

	if err := s.brain.AllowOutside(otherprograms.Subject(asked.Subject), *asked.Allowed); err != nil {
		fail(w, http.StatusInternalServerError, "That could not be kept.")

		return
	}

	s.outsideState(w)
}
