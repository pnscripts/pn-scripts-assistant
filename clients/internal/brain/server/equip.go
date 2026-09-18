package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * Equipping the organisation, from the interface: proposals, packages, what
 * the machine has, projects and integrations.
 *
 * Everything that decides — making a hire, starting a project, installing,
 * switching an integration on — is at the desk only (see guard.go) and is its
 * owner pressing the button that says so, which is the approval. Reading is
 * allowed from anywhere a device may read.
 */

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(into); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return false
	}

	return true
}

func idOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "which one?")

		return 0, false
	}

	return id, true
}

/*
 * handleHireFor answers "I need someone who specialises in Laravel security"
 * with a proposal now, never a hire: who it would be, what they could touch,
 * what the machine would need. Making it is its own button.
 */
func (s *Server) handleHireFor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sentence   string `json:"sentence"`
		Permanence string `json:"permanence"`
		Goal       string `json:"goal"`
	}

	if !decode(w, r, &body) {
		return
	}

	offer, err := s.brain.ProposeHire(r.Context(), body.Sentence, body.Permanence, body.Goal)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	s.proposalView(w, offer.ID)
}

// proposalView is one proposal with its body read, for the interface.
func (s *Server) proposalView(w http.ResponseWriter, id int64) {
	row, err := s.brain.DB.ProposalByID(id)
	if err != nil || row == nil {
		fail(w, http.StatusNotFound, "there is no such proposal")

		return
	}

	ok(w, proposalOut(*row))
}

func proposalOut(row store.Proposal) map[string]any {
	out := map[string]any{
		"id": row.ID, "kind": row.Kind, "request": row.Request, "state": row.State,
		"outcome": row.Outcome, "created_at": row.CreatedAt,
	}

	switch row.Kind {
	case store.ProposeHire:
		var p team.Proposal

		if json.Unmarshal([]byte(row.Body), &p) == nil {
			out["proposal"], out["text"] = p, p.Text()
		}
	case store.ProposeProject:
		var p bootstrap.Proposal

		if json.Unmarshal([]byte(row.Body), &p) == nil {
			out["proposal"], out["text"] = p, p.Text()
			out["ready"], out["question"], out["options"] = p.Ready(), p.Question, p.Options
		}
	}

	return out
}

// handleProposals is the latest proposals of a kind.
func (s *Server) handleProposals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.brain.DB.Proposals(r.URL.Query().Get("kind"), 20)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	out := make([]map[string]any, 0, len(rows))

	for _, row := range rows {
		out = append(out, proposalOut(row))
	}

	ok(w, map[string]any{"proposals": out})
}

// handleConfirmHire is its owner pressing "hire".
func (s *Server) handleConfirmHire(w http.ResponseWriter, r *http.Request) {
	id, found := idOf(w, r)
	if !found {
		return
	}

	var body struct {
		Permanence string `json:"permanence"`
		Goal       string `json:"goal"`
	}

	if !decode(w, r, &body) {
		return
	}

	said, err := s.brain.ConfirmHire(r.Context(), id, body.Permanence, body.Goal)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"said": said})
}

// handleDecline is its owner saying no to a proposal.
func (s *Server) handleDecline(w http.ResponseWriter, r *http.Request) {
	id, found := idOf(w, r)
	if !found {
		return
	}

	moved, err := s.brain.DB.DecideProposal(id, store.ProposalDeclined, "declined in the interface")
	if err != nil || !moved {
		fail(w, http.StatusBadRequest, "that proposal was already decided")

		return
	}

	ok(w, map[string]any{"declined": id})
}

/*
 * handlePackages is every capability package, what each needs on this
 * machine as it stands now, and any that could not be used and why.
 */
func (s *Server) handlePackages(w http.ResponseWriter, r *http.Request) {
	set := s.brain.Packages()
	recipes := s.brain.Recipes()

	out := make([]map[string]any, 0)

	for _, p := range set.All() {
		needs := make([]map[string]any, 0, len(p.Requires))

		for _, req := range p.Requires {
			needs = append(needs, map[string]any{"why": req.Why, "optional": req.Optional,
				"status": recipes.Check(req.ID, req.Versions)})
		}

		out = append(out, map[string]any{
			"id": p.ID, "version": p.Version, "title": p.Title, "summary": p.Summary, "work": p.Work,
			"jobs": p.Jobs, "engine": p.Engine, "role": p.Role, "requires": needs,
			"integrations": p.Integrations, "evidence": p.Evidence, "risk": p.Risk,
			"limits": p.Limits, "escalation": p.Escalation, "relies_on": p.Needs(),
			"built_in": p.BuiltIn, "file": p.File,
		})
	}

	ok(w, map[string]any{"packages": out, "refused": set.Refused,
		"folder": s.brain.Root + "/" + capability.FolderName})
}

// handleProvision is every recipe and where it stands on this machine.
func (s *Server) handleProvision(w http.ResponseWriter, r *http.Request) {
	out := make([]provision.Status, 0)

	for _, recipe := range s.brain.Recipes().All() {
		if recipe.Kind == provision.Part {
			continue
		}

		out = append(out, recipe.Check(""))
	}

	ok(w, map[string]any{"recipes": out})
}

// handleInstall is its owner pressing "install" beside one recipe.
func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Recipe   string `json:"recipe"`
		Versions string `json:"versions"`
	}

	if !decode(w, r, &body) {
		return
	}

	said, err := s.brain.StartInstall(body.Recipe, body.Versions)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"said": said})
}

// handleInspect looks at a folder. Reading only.
func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))

	if path == "" {
		fail(w, http.StatusBadRequest, "which folder?")

		return
	}

	ok(w, map[string]any{"text": s.brain.InspectProject(path)})
}

// handleProposeProject works a project out, and asks what it must.
func (s *Server) handleProposeProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Request string `json:"request"`
		Folder  string `json:"folder"`
		Package string `json:"package"`
	}

	if !decode(w, r, &body) {
		return
	}

	if strings.TrimSpace(body.Request) == "" {
		fail(w, http.StatusBadRequest, "what should the project be?")

		return
	}

	offer, err := s.brain.ProposeProject(r.Context(), body.Request, body.Folder, body.Package)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	s.proposalView(w, offer.ID)
}

// handleStartProject is its owner pressing "start".
func (s *Server) handleStartProject(w http.ResponseWriter, r *http.Request) {
	id, found := idOf(w, r)
	if !found {
		return
	}

	said, err := s.brain.StartProject(r.Context(), id)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"said": said})
}

// handleEvidence is what a task can prove it did.
func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	id, found := idOf(w, r)
	if !found {
		return
	}

	rows, err := s.brain.DB.EvidenceFor(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"evidence": rows})
}

// handleIntegrations is every integration, never with its secrets.
func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	g := s.brain.Integrations

	if g == nil {
		ok(w, map[string]any{"integrations": []any{}})

		return
	}

	ok(w, map[string]any{"integrations": g.List(), "events": g.Events("", 30)})
}

/*
 * handleIntegration is one action on one integration: add, approve, activate,
 * deactivate, revoke, remove, grant, read-only, secret. Its owner's, at the
 * desk.
 */
func (s *Server) handleIntegration(w http.ResponseWriter, r *http.Request) {
	g := s.brain.Integrations

	if g == nil {
		fail(w, http.StatusNotFound, "this build has no integrations")

		return
	}

	id := strings.TrimSpace(r.PathValue("id"))

	var body struct {
		Agents []string `json:"agents"`
		Tools  []string `json:"tools"`
		Name   string   `json:"name"`
		Value  string   `json:"value"`
	}

	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}

	var err error

	switch r.PathValue("action") {
	case "add":
		_, err = g.Add(id)
	case "approve":
		_, err = g.Approve(id, body.Agents)
	case "activate":
		_, err = g.Activate(r.Context(), id)
	case "deactivate":
		g.Deactivate(id)
	case "revoke":
		err = g.Revoke(id)
	case "remove":
		g.Deactivate(id)
		err = g.Book.Remove(id)
	case "grant":
		_, err = g.Grant(id, body.Agents)
	case "readonly":
		_, err = g.MarkReadOnly(id, body.Tools)
	case "secret":
		err = g.Book.SetSecret(id, body.Name, body.Value)
	case "health":
		err = g.Health(r.Context(), id)
	default:
		fail(w, http.StatusNotFound, "there is no such action")

		return
	}

	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"integrations": g.List()})
}

// handleOwnIntegration adds one its owner wrote by hand, not approved.
func (s *Server) handleOwnIntegration(w http.ResponseWriter, r *http.Request) {
	var server mcp.Server

	if !decode(w, r, &server) {
		return
	}

	if s.brain.Integrations == nil {
		fail(w, http.StatusNotFound, "this build has no integrations")

		return
	}

	if err := s.brain.Integrations.AddOwn(server); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"integrations": s.brain.Integrations.List()})
}
