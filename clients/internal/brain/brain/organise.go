package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tasks"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Equipping the organisation: capability packages, what they need, and hiring
 * against them.
 *
 * Everything here is reached two ways — a tool in conversation, a button in
 * the Organisation view — and both go through these methods, so the proposal
 * somebody reads in the panel and the one read out in conversation are the
 * same proposal, kept the same way, approved through the same gate.
 */

// tasksTaking is a task led by somebody hired for exactly it.
func tasksTaking(lead string, packages []string) tasks.Taking {
	return tasks.Taking{Lead: lead, Packages: packages}
}

// packageCache is the packages, re-read at most once a second: they are
// asked for on every proposal and every step's brief, and edited by hand.
type packageCache struct {
	sync.Mutex

	set *capability.Set
	at  time.Time
}

// Recipes is everything this program knows how to find and install.
func (b *Brain) Recipes() *provision.Book {
	b.recipesOnce.Do(func() { b.recipes = provision.Standard() })

	return b.recipes
}

// Packages is the capability packages in use: what ships, and its owner's.
func (b *Brain) Packages() *capability.Set {
	b.packages.Lock()
	defer b.packages.Unlock()

	if b.packages.set != nil && time.Since(b.packages.at) < time.Second {
		return b.packages.set
	}

	b.packages.set = capability.Load(b.Root, b.knownNames())
	b.packages.at = time.Now()

	return b.packages.set
}

// ForgetPackages makes the next read a fresh one, after somebody changed one.
func (b *Brain) ForgetPackages() {
	b.packages.Lock()
	b.packages.set = nil
	b.packages.Unlock()
}

/*
 * knownNames is what a package's names are checked against.
 *
 * Tools against the registry as it is — so a package naming a tool that does
 * not exist is refused rather than hiring somebody whose list quietly lacks
 * it — recipes against the book, jobs against the catalogue, integrations
 * against the approved list and engines against the adapters.
 */
func (b *Brain) knownNames() capability.Known {
	known := capability.Known{
		Recipe: b.Recipes().Known,
		Job: func(id string) bool {
			job, err := b.DB.Occupation(id)

			return err == nil && job != nil
		},
		Oversight: func(id string) bool {
			job, err := b.DB.Occupation(id)

			return err == nil && job != nil && job.Oversight
		},
	}

	if b.Agent != nil && b.Agent.Registry != nil {
		known.Tool = func(name string) bool {
			if _, ok := b.Agent.Registry.Get(name); ok {
				return true
			}

			// Registered only once they are set up, and still this
			// program's: a package may name them before they are.
			switch name {
			case "send_email", "read_email", "list_devices", "set_device":
				return true
			}

			return false
		}
	}

	if b.Integrations != nil {
		known.Integration = b.Integrations.Known
	}

	if b.Engines != nil {
		known.Engine = b.Engines.Known
	}

	return known
}

// hiringInputs is everything a proposal is worked out from, as it is now.
func (b *Brain) hiringInputs() team.Inputs {
	in := team.Inputs{
		Root:      b.Root,
		Roster:    team.Roster(b.Root),
		Chart:     org.Chart(b.Root),
		Catalogue: b.DB,
		Templates: team.Templates(b.Root),
		Packages:  b.Packages(),
		Recipes:   b.Recipes(),
		Sizes:     b.modelRoles(),
		Provider:  b.Cfg.DefaultProvider,
	}

	if b.Agent != nil {
		in.Toolbox = b.Agent.Registry
	}

	if b.Integrations != nil {
		in.Integration = b.Integrations.StateOf
	}

	return in
}

/*
 * ProposeHire works out a hire and keeps the proposal, whole.
 *
 * Kept even when somebody already here does the work, so "who would you hire
 * for this" leaves a record of the answer. Nothing is written to the
 * organisation.
 */
func (b *Brain) ProposeHire(_ context.Context, request, permanence, goal string) (tools.HireOffer, error) {
	return b.proposeHireIn(b.CurrentConversation(), request, permanence, goal)
}

func (b *Brain) proposeHireIn(conversationID int64, request, permanence, goal string) (tools.HireOffer, error) {
	p, err := team.Propose(b.hiringInputs(), team.Wish{Sentence: request})
	if err != nil {
		return tools.HireOffer{}, err
	}

	if permanence == team.Permanent || permanence == team.TaskOnly {
		p.Permanence = permanence
	}

	if strings.TrimSpace(goal) != "" {
		p.Goal = strings.TrimSpace(goal)
	}

	return b.keepHireProposal(conversationID, request, p)
}

/*
 * ProposeNewAgent is somebody new taken on from the Organisation view — from
 * a job in the catalogue, a template or a copy of somebody here — as the same
 * proposal a sentence in the chat makes. That view used to write the agent
 * there and then: permanent, in one click, and with nothing to derive tools
 * from, able to use every one of them. Now it is a proposal like any other,
 * made permanent only when its owner confirms it.
 */
func (b *Brain) ProposeNewAgent(name string, base team.Agent) (tools.HireOffer, error) {
	if strings.TrimSpace(base.Job) == "" {
		return tools.HireOffer{}, fmt.Errorf("choose a job for them: what somebody new may use is worked out from their job")
	}

	p, err := team.Propose(b.hiringInputs(), team.Wish{Job: base.Job, New: true, Sentence: base.Title})
	if err != nil {
		return tools.HireOffer{}, err
	}

	p.Permanence = team.Permanent

	// What its owner chose, over what was worked out.
	if strings.TrimSpace(name) != "" {
		p.Agent.Name = team.UnusedName(team.Roster(b.Root), strings.ToLower(strings.TrimSpace(name)))
	}

	if base.Uses != "" {
		p.Agent.Uses = base.Uses
	}

	if base.Title != "" {
		p.Agent.Title = base.Title
	}

	if base.Brief != "" {
		p.Agent.Brief = base.Brief
	}

	if base.Persona != "" {
		p.Agent.Persona = base.Persona
	}

	if base.Position != "" {
		p.Seat, p.SeatIn, p.SeatUnit = base.Position, "", ""
	}

	/*
	 * A copy keeps what its original may use, when that was ever narrowed —
	 * its owner approved exactly that list once already. An original with
	 * no list at all is the generalist, and a copy of it is not a second
	 * generalist: it gets what its job gives it.
	 */
	if len(base.Tools) > 0 {
		team.WithTools(&p, base.Tools)
	}

	return b.keepHireProposal(b.CurrentConversation(), "take on "+p.Agent.Name+" as "+p.Job.Title, p)
}

// keepHireProposal stores a hire proposal and says it in words.
func (b *Brain) keepHireProposal(conversationID int64, request string, p team.Proposal) (tools.HireOffer, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return tools.HireOffer{}, err
	}

	id, err := b.DB.Propose(store.Proposal{
		Kind: store.ProposeHire, ConversationID: conversationID, Request: request, Body: string(body),
	})
	if err != nil {
		return tools.HireOffer{}, err
	}

	if p.Existing != nil {
		b.DB.DecideProposal(id, store.ProposalSuperseded, "already here: "+p.Existing.Name)
	}

	p.ID = id

	b.Log.Info("proposed a hire", "proposal", id, "job", p.Job.ID, "existing", p.Existing != nil)

	return tools.HireOffer{
		ID: id, Text: fmt.Sprintf("Proposal %d.\n%s", id, p.Text()), Existing: p.Existing != nil,
		Permanence: p.Permanence, Goal: p.Goal,
	}, nil
}

// hireProposal reads a kept proposal back.
func (b *Brain) hireProposal(id int64) (*store.Proposal, team.Proposal, error) {
	row, err := b.DB.ProposalByID(id)
	if err != nil {
		return nil, team.Proposal{}, err
	}

	if row == nil || row.Kind != store.ProposeHire {
		return nil, team.Proposal{}, fmt.Errorf("there is no hire proposal %d", id)
	}

	var p team.Proposal

	if err := json.Unmarshal([]byte(row.Body), &p); err != nil {
		return nil, team.Proposal{}, fmt.Errorf("proposal %d could not be read: %w", id, err)
	}

	p.ID = id

	return row, p, nil
}

// ShowHire is a proposal as it would be carried out, for the approval.
func (b *Brain) ShowHire(id int64, permanence, goal string) (string, bool) {
	_, p, err := b.hireProposal(id)
	if err != nil {
		return "", false
	}

	p.Permanence = permanence

	if goal != "" {
		p.Goal = goal
	}

	how := "permanently"
	if permanence == team.TaskOnly {
		how = "for one task"
	}

	return fmt.Sprintf("Hire %s (%s) %s — proposal %d\n%s", p.Agent.Title, p.Agent.Name, how, id, p.Text()), true
}

/*
 * ConfirmHire carries out a proposal, exactly as it was kept.
 *
 * Decided first, then made: the proposal moves from open once, so a second
 * approval of the same one — a double click, a replayed request — does
 * nothing. A task-only hire then starts its task, led by the hire, and is
 * written on the task so the task's end lets them go.
 */
func (b *Brain) ConfirmHire(ctx context.Context, id int64, permanence, goal string) (string, error) {
	row, p, err := b.hireProposal(id)
	if err != nil {
		return "", err
	}

	if row.State != store.ProposalOpen {
		return "", fmt.Errorf("proposal %d was already %s", id, row.State)
	}

	p.Permanence = permanence

	if strings.TrimSpace(goal) != "" {
		p.Goal = strings.TrimSpace(goal)
	}

	if p.Permanence == team.TaskOnly && p.Goal == "" {
		return "", fmt.Errorf("a hire for one task needs the task")
	}

	moved, err := b.DB.DecideProposal(id, store.ProposalAccepted, "")
	if err != nil {
		return "", err
	}

	if !moved {
		return "", fmt.Errorf("proposal %d was decided while this was waiting", id)
	}

	hired, err := team.Accept(b.Root, p, 0)
	if err != nil {
		b.DB.NoteOutcome(id, "failed: "+err.Error())

		return "", err
	}

	b.Log.Info("hired from a proposal", "proposal", id, "agent", hired.Agent.Name, "permanence", p.Permanence)

	if p.Permanence == team.Permanent {
		b.DB.NoteOutcome(id, "hired "+hired.Agent.Name)

		where := ""
		if hired.Seat != "" {
			where = ", in the seat " + hired.Seat
		}

		return fmt.Sprintf("Hired %s as %s%s. They are in the Organisation view, and their file is %s.md in the agents folder.",
			hired.Agent.Name, hired.Agent.Title, where, hired.Agent.Name), nil
	}

	if b.Tasks == nil {
		team.DropHire(b.Root, hired.Agent.Name)

		return "", fmt.Errorf("tasks cannot run in this build, so a hire for one task has nothing to do")
	}

	task, started, err := b.Tasks.TakeWith(ctx, row.ConversationID, p.Goal, b.Cfg.DefaultProvider, true,
		tasksTaking(hired.Agent.Name, hired.Agent.Packages))
	if err != nil || !started || task == nil {
		team.DropHire(b.Root, hired.Agent.Name)

		why := "the task could not be started"
		if err != nil {
			why += ": " + err.Error()
		}

		b.DB.NoteOutcome(id, "let go: "+why)

		return "", fmt.Errorf("%s, so %s was let go again", why, hired.Agent.Name)
	}

	// Written on the agent and on the task, so the task's end lets them go
	// and its account names them.
	agent := hired.Agent
	agent.HiredFor = task.ID

	if err := team.Save(b.Root, agent); err != nil {
		b.Log.Warn("could not tie a hire to its task", "agent", agent.Name, "task", task.ID, "error", err)
	}

	team.Forget()

	line := fmt.Sprintf("%s — hired for this task from proposal %d, as its owner approved", agent.Title, id)

	if err := b.DB.RecordHire(task.ID, line); err != nil {
		b.Log.Warn("could not record a hire on its task", "task", task.ID, "error", err)
	}

	b.DB.NoteOutcome(id, fmt.Sprintf("hired %s for task %d", agent.Name, task.ID))

	return fmt.Sprintf("Hired %s as %s for this task only, and started it: %s. They will be let go when it ends.",
		agent.Name, agent.Title, task.Name), nil
}

// StartInstall installs one recipe behind the conversation, and proves it.
func (b *Brain) StartInstall(id, versions string) (string, error) {
	r, ok := b.Recipes().Get(id)
	if !ok {
		return "", fmt.Errorf("there is no recipe called %q", id)
	}

	if b.Jobs == nil {
		return "", fmt.Errorf("there is nowhere to run that")
	}

	if _, err := b.Jobs.Start("Installing "+r.Title, func(ctx context.Context) (string, error) {
		var said strings.Builder

		status, err := provision.Carry(ctx, r, versions, &said)
		if err != nil {
			return "", fmt.Errorf("%w\n%s", err, lastFewLines(said.String(), 12))
		}

		return fmt.Sprintf("%s is installed (%s) at %s, and it runs.", r.Title,
			orDash(status.Version), status.Path), nil
	}); err != nil {
		return "", err
	}

	b.Log.Info("installing a requirement", "recipe", id, "versions", versions)

	size := ""
	if r.Size != "" {
		size = " It is " + r.Size + "."
	}

	return fmt.Sprintf("Installing %s in the background.%s I will say when it is done, and whether it runs.",
		r.Title, size), nil
}

func lastFewLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}

func orDash(value string) string {
	if value == "" {
		return "version not said"
	}

	return value
}

// provisioningOf is the brain as the tools see it.
type provisioningOf struct{ b *Brain }

func (p provisioningOf) Recipes() *provision.Book  { return p.b.Recipes() }
func (p provisioningOf) Packages() *capability.Set { return p.b.Packages() }
func (p provisioningOf) StartInstall(id, versions string) (string, error) {
	return p.b.StartInstall(id, versions)
}
