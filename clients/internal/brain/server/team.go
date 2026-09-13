package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * Who does which part of a job.
 *
 * The two things somebody actually wants to change are here — which size of
 * model an agent uses, and whether it is pinned to a particular service. Tool
 * lists and standing instructions stay in the files, because they are the part
 * that is written once and read often, and a form with eight fields in it is a
 * form nobody finishes.
 */
func (s *Server) handleTeam(w http.ResponseWriter, r *http.Request) {
	roster := team.Roster(s.brain.Root)

	sizes := s.brain.ModelRoles()

	shown := make([]map[string]any, 0, len(roster))

	for _, agent := range roster {
		tools := agent.Tools

		if len(tools) == 0 {
			tools = []string{"everything"}
		}

		shown = append(shown, map[string]any{
			"name":     agent.Name,
			"title":    agent.Title,
			"for":      agent.For,
			"uses":     agent.Uses,
			"provider": agent.Provider,
			"tools":    tools,
			"brief":    agent.Brief,
			"built_in": agent.BuiltIn,

			// What that size of model actually is on this machine, so the
			// choice is between real models rather than between words.
			"model": agent.Model(sizes),
		})
	}

	ok(w, map[string]any{
		"team":   shown,
		"folder": team.Folder(s.brain.Root),
		/*
		 * Only the ones privacy currently allows.
		 *
		 * Offering a choice the router would refuse is offering somebody a
		 * setting that silently does nothing, and they would have no way of
		 * telling that from a bug.
		 */
		"providers": permittedProviders(s.brain.Router.Availabilities(r.Context())),
	})
}

func (s *Server) handleSaveAgent(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	var body struct {
		Uses     string `json:"uses"`
		Provider string `json:"provider"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	agent, found := team.Find(team.Roster(s.brain.Root), name)
	if !found {
		fail(w, http.StatusNotFound, "there is nobody on the team called that")

		return
	}

	agent.Uses = body.Uses
	agent.Provider = strings.TrimSpace(body.Provider)

	if err := team.Save(s.brain.Root, agent); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"saved": name})
}

// handleResetAgent puts one back to what it shipped as.
func (s *Server) handleResetAgent(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	if err := team.Reset(s.brain.Root, name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"reset": name})
}

func permittedProviders(list []llm.Availability) []string {
	out := []string{}

	for _, a := range list {
		if a.Permitted {
			out = append(out, a.Name)
		}
	}

	return out
}
