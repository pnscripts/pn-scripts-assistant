package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * The organisation: what parts there are, what seats are in them, and who
 * holds each one.
 *
 * Three things are deliberately kept apart here and it is the whole point of
 * the panel. A job is a definition — there are hundreds, and they exist
 * whether or not anybody here does them. A seat is a responsibility inside
 * this organisation. An agent is who actually turns up. A seat with nobody in
 * it is not a gap in the data; it is the program able to say "there is a place
 * for somebody who does this here, and it is empty", which is the question
 * worth being able to answer.
 */
func (s *Server) handleOrganisation(w http.ResponseWriter, r *http.Request) {
	chart := org.Chart(s.brain.Root)
	roster := team.Roster(s.brain.Root)
	depth := org.DepthOf(chart)

	doing, waiting := s.workload()
	record := s.record()
	remembers, _ := s.brain.DB.HowMuchRemembered()

	held := map[string][]map[string]any{}
	seated := map[string]bool{}

	for _, agent := range roster {
		fit := team.Settle(agent, chart, s.brain.DB, s.brain.Agent.Registry)

		shown := map[string]any{
			"name":      agent.Name,
			"title":     agent.Title,
			"for":       agent.For,
			"state":     orElse(agent.State, team.Active),
			"seniority": orElse(agent.Seniority, fit.Seat.Seniority),
			"built_in":  agent.BuiltIn,
			"can":       fit.Can,
			"tools":     shownTools(fit),
			"never":     fit.Never,
			"model":     agent.ModelOn(agent.Through(""), s.brain.ModelRoles()),
			"provider":  agent.Provider,
			"persona":   agent.Persona,
			"manner":    agent.Manner,
			"also":      agent.Also,
			"hired_for": agent.HiredFor,

			// What it is doing now and what is queued for it — the registry's
			// "who is available" — and what it has actually got done.
			"doing":     doing[agent.Name],
			"waiting":   waiting[agent.Name],
			"record":    record[agent.Name],
			"remembers": remembers[agent.Name],
		}

		if fit.Job != nil {
			shown["job"] = map[string]any{
				"id": fit.Job.ID, "title": fit.Job.Title,
				"risk": fit.Job.Risk, "oversight": fit.Job.Oversight,
			}
		}

		if agent.Position != "" {
			seated[agent.Position] = true
		}

		held[agent.Position] = append(held[agent.Position], shown)
	}

	units := make([]map[string]any, 0, len(chart))

	for _, unit := range org.Shape(chart) {
		seats := make([]map[string]any, 0, len(unit.Seats))

		for _, seat := range unit.Seats {
			seats = append(seats, map[string]any{
				"name":       seat.Name,
				"title":      seat.Title,
				"job":        seat.Job,
				"job_title":  jobTitle(s.brain.DB, seat.Job),
				"seniority":  seat.Seniority,
				"reports_to": seat.ReportsTo,
				"held_by":    held[seat.Name],
				"vacant":     !seated[seat.Name],
			})
		}

		units = append(units, map[string]any{
			"name": unit.Name, "title": unit.Title, "kind": unit.Kind,
			"parent": unit.Parent, "purpose": unit.Purpose,
			"depth": depth[unit.Name], "built_in": unit.BuiltIn,
			"seats": seats,
		})
	}

	jobs, _ := s.brain.DB.HowManyOccupations()
	capabilities, _ := s.brain.DB.HowManyCapabilities()

	ok(w, map[string]any{
		"units": units,

		// Anybody who holds no seat. Not an error — an agent written before
		// there was an organisation has no seat, and still works.
		"unseated": held[""],

		"folder":       org.Folder(s.brain.Root),
		"agents":       team.Folder(s.brain.Root),
		"jobs":         jobs,
		"capabilities": capabilities,
		"categories":   occupations.Categories(),
		"templates":    templateViews(team.Templates(s.brain.Root)),
	})
}

/*
 * workload is what each agent is doing this minute and how much is queued
 * behind it, from the rows of the work that is live.
 *
 * Read, not tracked. The steps already record who is doing them, and a
 * second count kept in memory would be a second answer to the same question
 * that could be wrong in its own way.
 */
func (s *Server) workload() (map[string][]string, map[string]int) {
	doing := map[string][]string{}
	waiting := map[string]int{}

	live, err := s.brain.DB.LiveTasks()
	if err != nil {
		return doing, waiting
	}

	for _, task := range live {
		steps, err := s.brain.DB.Steps(task.ID)
		if err != nil {
			continue
		}

		for _, step := range steps {
			if step.Assignee == "" {
				continue
			}

			switch step.State {
			case store.StepRunning, store.StepHandedOn:
				doing[step.Assignee] = append(doing[step.Assignee], step.Instruction)
			case store.StepWaiting, store.StepNeedsYou:
				waiting[step.Assignee]++
			}
		}
	}

	return doing, waiting
}

// record is what each agent has got done in the last month, verified and
// claimed kept apart as everywhere else.
func (s *Server) record() map[string]store.AgentWork {
	out := map[string]store.AgentWork{}

	review, err := s.brain.DB.HowItHasBeenGoing(time.Now().AddDate(0, -1, 0))
	if err != nil {
		return out
	}

	for _, person := range review.People {
		out[person.Name] = person
	}

	return out
}

func templateViews(templates []team.Template) []map[string]any {
	out := make([]map[string]any, 0, len(templates))

	for _, t := range templates {
		out = append(out, map[string]any{
			"name": t.Name, "title": t.Title, "job": t.Job, "for": t.For,
			"tools": t.Tools, "built_in": t.BuiltIn,
		})
	}

	return out
}

// shownTools says "everything" rather than an empty list, because an empty
// list means two opposite things in this program and a panel must not be the
// place somebody finds that out.
func shownTools(fit team.Fit) any {
	if !fit.Narrowed {
		return []string{"everything"}
	}

	if len(fit.Tools) == 0 {
		return []string{"none — its work is judgement"}
	}

	return fit.Tools
}

func jobTitle(db *store.DB, id string) string {
	if id == "" {
		return ""
	}

	job, err := db.Occupation(id)
	if err != nil || job == nil {
		return ""
	}

	return job.Title
}

/*
 * handleJobs searches what work there is, as against who does it.
 *
 * The taxonomy is hundreds of rows and will be thousands after an import, so
 * this is a search rather than a list. Matching is done in Go rather than in
 * SQL because SQLite's LIKE only knows the English alphabet, and this
 * assistant is spoken to in two languages.
 */
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	looking := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))

	var (
		found []store.Occupation
		err   error
	)

	if looking != "" {
		found, err = s.brain.DB.FindOccupations(looking, 40)
	} else {
		found, err = s.brain.DB.Occupations(category, 40)
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	shown := make([]map[string]any, 0, len(found))

	for _, job := range found {
		needs, _ := s.brain.DB.CapabilitiesOf(job.ID)

		shown = append(shown, map[string]any{
			"id": job.ID, "title": job.Title, "category": job.Category,
			"what": job.Description, "status": job.Status, "risk": job.Risk,
			"oversight": job.Oversight, "aliases": job.Aliases,
			"ladder": job.Steps(), "needs": needs, "does": job.Does,
			"makes": job.Makes, "came_from": job.CameFrom,
		})
	}

	ok(w, map[string]any{"jobs": shown})
}

/*
 * handleWhoKnows answers "who here is good at this" from the data.
 *
 * Structured, not asked. Putting the question to every agent would cost
 * minutes on this machine and come back with opinions rather than an answer.
 */
func (s *Server) handleWhoKnows(w http.ResponseWriter, r *http.Request) {
	capability := strings.TrimSpace(r.URL.Query().Get("can"))

	if capability == "" {
		fail(w, http.StatusBadRequest, "good at what?")

		return
	}

	found := team.Knowing(team.Roster(s.brain.Root), org.Chart(s.brain.Root),
		s.brain.DB, s.brain.Agent.Registry, capability)

	who := make([]map[string]any, 0, len(found))

	for _, agent := range found {
		who = append(who, map[string]any{
			"name": agent.Name, "title": agent.Title, "position": agent.Position,
		})
	}

	// And which jobs call for it, which is the answer when nobody here does:
	// it names what would have to be hired rather than saying no.
	jobs, _ := s.brain.DB.OccupationsNeeding(capability, 10)

	could := make([]map[string]any, 0, len(jobs))

	for _, job := range jobs {
		could = append(could, map[string]any{"id": job.ID, "title": job.Title})
	}

	ok(w, map[string]any{"who": who, "jobs": could})
}

/*
 * handleHire creates an agent, or changes one.
 *
 * A hire is a file in the agents folder, so everything here is also doable in
 * an editor — and what the program writes and what somebody writes by hand are
 * the same thing, which is the rule the roster has always followed.
 */
func (s *Server) handleHire(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string       `json:"name"`
		Title     string       `json:"title"`
		For       string       `json:"for"`
		Position  string       `json:"position"`
		Job       string       `json:"job"`
		Seniority string       `json:"seniority"`
		Uses      string       `json:"uses"`
		Provider  string       `json:"provider"`
		Can       []string     `json:"can"`
		Brief     string       `json:"brief"`
		State     string       `json:"state"`
		Tools     []string     `json:"tools"`
		Never     []string     `json:"never"`
		Also      []string     `json:"also"`
		Persona   string       `json:"persona"`
		Manner    *team.Manner `json:"manner"`

		// Template starts a new agent from one, and CloneOf from somebody
		// already here. Either way it is a copy, and changing the original
		// later changes nobody made from it.
		Template string `json:"template"`
		CloneOf  string `json:"clone_of"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	name := strings.ToLower(strings.TrimSpace(body.Name))

	roster := team.Roster(s.brain.Root)

	agent, existing := team.Find(roster, name)

	// A copy is always somebody new. Copying onto a name already taken would
	// quietly rewrite whoever has it.
	if body.CloneOf != "" && existing {
		fail(w, http.StatusBadRequest, "there is already somebody called "+name)

		return
	}

	if !existing {
		agent = team.Agent{Name: name, Uses: team.UsesWork, State: team.Active}

		if body.Template != "" {
			for _, t := range team.Templates(s.brain.Root) {
				if t.Name == body.Template {
					agent = team.FromTemplate(t, name)
				}
			}
		}

		if body.CloneOf != "" {
			original, ok := team.Find(roster, body.CloneOf)
			if !ok {
				fail(w, http.StatusBadRequest, "there is nobody called "+body.CloneOf+" to copy")

				return
			}

			agent = team.FromTemplate(original, name)
		}
	}

	for _, field := range []struct {
		into  *string
		given string
	}{
		{&agent.Title, body.Title},
		{&agent.For, body.For},
		{&agent.Position, strings.ToLower(body.Position)},
		{&agent.Job, strings.ToLower(body.Job)},
		{&agent.Seniority, strings.ToLower(body.Seniority)},
		{&agent.Uses, strings.ToLower(body.Uses)},
		{&agent.Brief, body.Brief},
		{&agent.State, strings.ToLower(body.State)},
	} {
		if given := strings.TrimSpace(field.given); given != "" {
			*field.into = given
		}
	}

	// Cleared rather than ignored when emptied on purpose: a service somebody
	// unpins should actually be unpinned.
	agent.Provider = strings.TrimSpace(body.Provider)

	// A list sent is the whole list — adding a tool or a capability is
	// sending the list with it in, taking one away is sending it without.
	for _, list := range []struct {
		into  *[]string
		given []string
	}{
		{&agent.Can, body.Can},
		{&agent.Tools, body.Tools},
		{&agent.Never, body.Never},
		{&agent.Also, body.Also},
	} {
		if list.given != nil {
			*list.into = list.given
		}
	}

	if body.Persona != "" {
		agent.Persona = body.Persona
	}

	if body.Manner != nil {
		agent.Manner = *body.Manner
	}

	/*
	 * A seat fills in what it can, so hiring is one field rather than six.
	 *
	 * The job and the seniority come from the position when they were not
	 * given, because that is what a position is for — and a hire that had to
	 * restate them would be a second copy of the seat, out of date by the
	 * second week.
	 */
	if agent.Position != "" {
		if _, seat, found := org.Seat(org.Chart(s.brain.Root), agent.Position); found {
			agent.Job = orElse(agent.Job, seat.Job)
			agent.Seniority = orElse(agent.Seniority, seat.Seniority)
			agent.Title = orElse(agent.Title, seat.Title)
			agent.For = orElse(agent.For, describeJob(s.brain.DB, seat.Job))
		}
	}

	/*
	 * Somebody new is a proposal, whichever button asked for them — the same
	 * one the chat makes, confirmed through the same door. Only a change to
	 * somebody already here is written straight away, and even then never a
	 * tool list emptied to mean "everything".
	 */
	if !existing {
		offer, err := s.brain.ProposeNewAgent(name, agent)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())

			return
		}

		s.proposalView(w, offer.ID)

		return
	}

	if body.Tools != nil && len(body.Tools) == 0 {
		fail(w, http.StatusBadRequest, "an empty tool list would mean every tool — name the tools, or say none")

		return
	}

	if err := team.Save(s.brain.Root, agent); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	team.Forget()

	ok(w, map[string]any{"hired": agent.Name, "was_already_here": existing})
}

// describeJob is a job's own description, used as an agent's when nobody wrote
// one — so a hire from a job definition arrives knowing what it is for.
func describeJob(db *store.DB, id string) string {
	if id == "" {
		return ""
	}

	job, err := db.Occupation(id)
	if err != nil || job == nil {
		return ""
	}

	return job.Description
}

/*
 * handleAgentState activates, suspends or retires somebody.
 *
 * Kept rather than deleted, so what they did is still recorded against a name
 * somebody can find. Removing the file is still what "remove" does.
 */
func (s *Server) handleAgentState(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	var body struct {
		State string `json:"state"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	state := strings.ToLower(strings.TrimSpace(body.State))

	switch state {
	case team.Active, team.Suspended, team.Retired:
	default:
		fail(w, http.StatusBadRequest, "that is active, suspended or retired")

		return
	}

	agent, found := team.Find(team.Roster(s.brain.Root), name)
	if !found {
		fail(w, http.StatusNotFound, "there is nobody called "+name)

		return
	}

	agent.State = state

	// A hire for one task made permanent by being activated is no longer
	// that task's to let go.
	if state == team.Active {
		agent.HiredFor = 0
	}

	if err := team.Save(s.brain.Root, agent); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	team.Forget()

	ok(w, map[string]any{"name": name, "state": state})
}

// handleMemories is what one agent remembers from its own work, newest first.
// Reading, so not a decision: it is not on the desk-only list.
func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	memories, err := s.brain.DB.Memories(name, 20)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"memories": memories})
}

// handleRetire removes an agent. Built-in ones go back to what they shipped
// as rather than disappearing, which is the same rule the roster has always
// had: the answer to "what is the developer now" is one of two things.
func (s *Server) handleRetire(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	if err := team.Reset(s.brain.Root, name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	team.Forget()

	ok(w, map[string]any{"gone": name})
}

// handleSaveUnit writes one part of the organisation.
func (s *Server) handleSaveUnit(w http.ResponseWriter, r *http.Request) {
	var unit org.Unit

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&unit); err != nil {
		fail(w, http.StatusBadRequest, "unreadable request")

		return
	}

	unit.Name = strings.ToLower(strings.TrimSpace(unit.Name))

	if err := org.Save(s.brain.Root, unit); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"saved": unit.Name})
}

// handleResetUnit puts a built-in part of the organisation back.
func (s *Server) handleResetUnit(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))

	if err := org.Reset(s.brain.Root, name); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"reset": name})
}

func orElse(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}
