package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/profile"
	"pn-scripts-assistant/internal/brain/skills"
)

/*
 * What it knows about the person it works for, and what it has been taught.
 *
 * Two things somebody writes rather than two things the program works out, so
 * both are files in the brain's folder and both are editable from here or in
 * an editor. Neither goes through the learning queue: what its owner writes
 * about themselves does not need approving by them, and a skill they typed
 * into this box is not a claim to be quarantined.
 */
func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	text, err := profile.Read(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"profile": text,
		"path":    profile.Path(s.brain.Root),
		"most":    profile.MostRunes,

		// So the panel can say plainly where this goes, rather than leaving
		// somebody to guess whether writing it here sends it somewhere.
		"to_hosted": s.brain.Cfg.ProfileToHosted,
	})
}

func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Profile string `json:"profile"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	if err := profile.Write(s.brain.Root, body.Profile); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Read back rather than echoed, so the panel shows what was actually kept
	// when it was longer than the ceiling.
	text, _ := profile.Read(s.brain.Root)

	ok(w, map[string]any{"profile": text})
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	found, err := s.brain.Skills()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if found == nil {
		found = []skills.Skill{}
	}

	ok(w, map[string]any{
		"skills": found,
		"folder": skills.Folder(s.brain.Root),
	})
}

func (s *Server) handleSaveSkill(w http.ResponseWriter, r *http.Request) {
	var skill skills.Skill

	if err := json.NewDecoder(r.Body).Decode(&skill); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	skill.Name = strings.TrimSpace(strings.ToLower(skill.Name))

	if err := s.brain.SaveSkill(skill); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"saved": skill.Name})
}

func (s *Server) handleForgetSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := s.brain.ForgetSkill(name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"forgotten": name})
}

/*
 * handleReloadSkills reads the folder again.
 *
 * For somebody who edits a skill in an editor rather than here. It is offered
 * because the alternative is restarting the program to see a corrected file
 * take effect, and a correction that costs a restart is a correction nobody
 * makes twice.
 */
func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	count, err := s.brain.ReloadSkills()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"skills": count})
}
