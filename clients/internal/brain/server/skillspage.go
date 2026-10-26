package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/skills"
)

/*
 * The Skills page: every skill the assistant can follow — the built-in ones,
 * the ones read from other agents' skill folders, and what its owner wrote
 * or taught — with where each came from, a switch for each, the folders
 * they are read from, and a Reload that reads them all again.
 */

type skillFolder struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Skills int    `json:"skills"`
	Kind   string `json:"kind"` // default, mine, added
}

func countSkills(folder string) (bool, int) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return false, 0
	}

	n := 0

	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(folder, e.Name(), skills.SkillFile)); err == nil {
				n++
			}
		} else if strings.EqualFold(filepath.Ext(e.Name()), ".md") && !strings.EqualFold(e.Name(), "README.md") {
			n++
		}
	}

	return true, n
}

func (s *Server) handleSkillCatalogue(w http.ResponseWriter, _ *http.Request) {
	list, err := s.brain.SkillCatalogue()
	if err != nil && len(list) == 0 {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	defaults := map[string]bool{}
	for _, f := range skills.DefaultSkillFolders() {
		defaults[f] = true
	}

	mine := filepath.Join(s.brain.Root, brain.MySkillsFolder)

	var folders []skillFolder

	var added []string

	for _, f := range s.brain.SkillFolders() {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.brain.Root, path)
		}

		kind := "added"

		switch {
		case defaults[f]:
			kind = "default"
		case f == mine:
			kind = "mine"
		default:
			added = append(added, f)
		}

		exists, n := countSkills(path)
		folders = append(folders, skillFolder{Path: f, Exists: exists, Skills: n, Kind: kind})
	}

	ok(w, map[string]any{"skills": list, "folders": folders, "added": added, "mine": mine})
}

func (s *Server) handleSkillEnabled(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}

	if !decodeUpTo(w, r, &body, 1<<10) {
		return
	}

	if err := s.brain.SetSkillEnabled(r.PathValue("name"), body.On); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"name": r.PathValue("name"), "on": body.On})
}

func (s *Server) handleSkillFolders(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Folders []string `json:"folders"`
	}

	if !decodeUpTo(w, r, &body, 1<<14) {
		return
	}

	for _, f := range body.Folders {
		if f = strings.TrimSpace(f); f != "" && !filepath.IsAbs(f) {
			fail(w, http.StatusBadRequest, "Give each folder as a full path: "+f)

			return
		}
	}

	n, err := s.brain.SetSkillFolders(body.Folders)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"skills": n})
}
