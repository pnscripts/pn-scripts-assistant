package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/browse"
	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/paths"
	"pn-scripts-assistant/internal/brain/places"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tasks"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * Projects: from "make me a Tetris game" to work under way, with its owner's
 * yes in between.
 *
 * The proposal is worked out in the bootstrap package and kept here whole.
 * Starting it carries out exactly what was kept — the installs it listed, the
 * files it listed, the hire it proposed, the steps it showed — and refuses if
 * any of that has changed underneath it since it was read.
 */

// organisationTools are the tools this side of the program adds, with the
// engines and the integrations they stand on.
func (b *Brain) organisationTools() []tools.Tool {
	b.Engines = engines.Standard(func(ctx context.Context, url string, seconds int, png string) (engines.PageShot, error) {
		shot, err := browse.Snapshot(ctx, url, seconds, png)

		return engines.PageShot{Title: shot.Title, Errors: shot.Errors, Canvas: shot.Canvas,
			Picture: shot.Picture, Trouble: shot.Trouble}, err
	})

	b.Integrations = &mcp.Gateway{
		Book:   mcp.Open(b.Root),
		DB:     b.DB,
		Log:    b.Log,
		Ready:  func(recipe string) bool { return b.Recipes().Check(recipe, "").Ready() },
		Online: func() bool { return b.Mode.AllowsWeb() },
	}

	games := tools.Games{Engines: b.Engines, Online: func() bool { return b.Cfg.LookOnline }, Places: b.placePaths}

	return []tools.Tool{
		tools.HireAgent{Hiring: b},
		tools.ConfirmHire{Hiring: b},
		tools.CheckRequirements{Provisioning: provisioningOf{b}},
		tools.InstallRequirement{Provisioning: provisioningOf{b}},
		tools.InspectProject{Projects: b},
		tools.PlanProject{Projects: b},
		tools.StartProject{Projects: b},
		tools.GameEngines{Games: games},
		tools.GameDocs{Games: games},
		tools.GameCheck{Games: games},
		tools.GameBuild{Games: games},
		mcp.ListIntegrations{Gateway: b.Integrations},
		mcp.ActivateIntegration{Gateway: b.Integrations},
		mcp.ReadIntegration{Gateway: b.Integrations},
	}
}

// placePaths is the folders the brain has been given to look in.
func (b *Brain) placePaths() []string {
	list, err := places.List(b.Root)
	if err != nil {
		return nil
	}

	out := make([]string, 0, len(list))

	for _, p := range list {
		out = append(out, p.Path)
	}

	return out
}

// ownFolders are this program's own, which no project may go near.
func (b *Brain) ownFolders() []string {
	return []string{b.Root, paths.MachineFolder()}
}

func (b *Brain) bootstrapInputs() bootstrap.Inputs {
	return bootstrap.Inputs{
		Engines:  b.Engines,
		Packages: b.Packages(),
		Recipes:  b.Recipes(),
		Hiring:   b.hiringInputs(),
		Own:      b.ownFolders(),
		Choose:   b.chooseEngine,
		Stack:    b.stack,
		Time:     b.projectTime,
	}
}

// InspectProject looks at a folder and says what is there.
func (b *Brain) InspectProject(dir string) string {
	return workspace.Inspect(expandHome(dir), b.Engines, b.ownFolders()...).Text()
}

func expandHome(dir string) string {
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, dir[2:])
		}
	}

	return dir
}

// ProposeProject works a project out and keeps the proposal.
func (b *Brain) ProposeProject(_ context.Context, request, dir, pkg string) (tools.ProjectOffer, error) {
	return b.proposeProjectIn(b.CurrentConversation(), bootstrap.Request{Sentence: request, Dir: dir, Package: pkg})
}

func (b *Brain) proposeProjectIn(conversationID int64, r bootstrap.Request) (tools.ProjectOffer, error) {
	p := bootstrap.Propose(b.bootstrapInputs(), r)

	body, err := json.Marshal(p)
	if err != nil {
		return tools.ProjectOffer{}, err
	}

	id, err := b.DB.Propose(store.Proposal{Kind: store.ProposeProject, ConversationID: conversationID,
		Request: r.Sentence, Body: string(body)})
	if err != nil {
		return tools.ProjectOffer{}, err
	}

	b.Log.Info("proposed a project", "proposal", id, "dir", p.Dir, "question", p.Question != "",
		"ready", p.Ready())

	text := p.Text()
	if p.Question == "" {
		text = fmt.Sprintf("Proposal %d.\n%s", id, text)
	}

	return tools.ProjectOffer{ID: id, Text: text, Question: p.Question, Ready: p.Ready()}, nil
}

func (b *Brain) projectProposal(id int64) (*store.Proposal, bootstrap.Proposal, error) {
	row, err := b.DB.ProposalByID(id)
	if err != nil {
		return nil, bootstrap.Proposal{}, err
	}

	if row == nil || row.Kind != store.ProposeProject {
		return nil, bootstrap.Proposal{}, fmt.Errorf("there is no project proposal %d", id)
	}

	var p bootstrap.Proposal

	if err := json.Unmarshal([]byte(row.Body), &p); err != nil {
		return nil, bootstrap.Proposal{}, fmt.Errorf("proposal %d could not be read: %w", id, err)
	}

	p.ID = id

	return row, p, nil
}

// ShowProject is the proposal as it would be carried out, for the approval.
func (b *Brain) ShowProject(id int64) (string, bool) {
	_, p, err := b.projectProposal(id)
	if err != nil {
		return "", false
	}

	return fmt.Sprintf("Start project proposal %d\n%s", id, p.Text()), true
}

/*
 * StartProject carries out an approved proposal, in the background.
 *
 * Decided first, so a second approval does nothing. Then, in order and
 * stopping at the first thing that fails: install what it listed, lay the
 * files it listed — refusing if what would be written is no longer what was
 * shown — take on who it proposed, and start the work with the plan it
 * showed, confined to its folder.
 */
func (b *Brain) StartProject(_ context.Context, id int64) (string, error) {
	row, p, err := b.projectProposal(id)
	if err != nil {
		return "", err
	}

	if row.State != store.ProposalOpen {
		return "", fmt.Errorf("proposal %d was already %s", id, row.State)
	}

	if !p.Ready() {
		return "", fmt.Errorf("proposal %d cannot start yet: %s", id, strings.Join(append([]string{p.Question}, p.Blockers...), "; "))
	}

	moved, err := b.DB.DecideProposal(id, store.ProposalAccepted, "starting")
	if err != nil {
		return "", err
	}

	if !moved {
		return "", fmt.Errorf("proposal %d was decided while this was waiting", id)
	}

	if b.Jobs == nil || b.Tasks == nil {
		return "", fmt.Errorf("this build cannot run work in the background")
	}

	if _, err := b.Jobs.StartSilent("Setting up "+p.Name, func(ctx context.Context) (string, error) {
		said, err := b.setUp(ctx, row.ConversationID, p)

		if err != nil {
			b.DB.NoteOutcome(id, "failed: "+err.Error())
			b.SayInto(row.ConversationID, "Setting up "+p.Name+" stopped: "+err.Error())

			return "", err
		}

		b.DB.NoteOutcome(id, said)
		b.SayInto(row.ConversationID, said)

		return said, nil
	}); err != nil {
		return "", err
	}

	installing := ""
	if len(p.Installs) > 0 {
		installing = " First installing " + strings.Join(p.Installs, ", ") + "."
	}

	return fmt.Sprintf("Setting up %s in %s.%s I will say when the work has started.", p.Name, p.Dir, installing), nil
}

// setUp is the approved proposal, carried out.
func (b *Brain) setUp(ctx context.Context, conversationID int64, p bootstrap.Proposal) (string, error) {
	var evidence []store.Evidence

	for _, id := range p.Installs {
		recipe, ok := b.Recipes().Get(id)
		if !ok {
			return "", fmt.Errorf("there is no recipe called %s", id)
		}

		var said strings.Builder

		status, err := provision.Carry(ctx, recipe, "", &said)
		if err != nil {
			return "", fmt.Errorf("installing %s: %w", recipe.Title, err)
		}

		evidence = append(evidence, store.Evidence{Kind: store.EvidenceVersion, Subject: recipe.Title,
			Detail: "installed " + status.Version + " at " + status.Path, OK: true})
	}

	files, err := b.scaffoldAgain(p)
	if err != nil {
		return "", err
	}

	written, err := workspace.Lay(p.Dir, files, p.Config, b.Root)
	if err != nil {
		return "", err
	}

	for _, path := range written {
		evidence = append(evidence, store.Evidence{Kind: store.EvidenceFile, Subject: path,
			Detail: "created for the new project", OK: true})
	}

	lead := p.Agent
	hiredName := ""

	if p.Hire != nil {
		hired, err := team.Accept(b.Root, *p.Hire, 0)
		if err != nil {
			return "", fmt.Errorf("taking on %s: %w", p.Hire.Agent.Title, err)
		}

		lead, hiredName = hired.Agent.Name, hired.Agent.Name
	}

	steps := append([]store.TaskStep{}, p.Steps...)

	for i := range steps {
		steps[i].Assignee = lead
	}

	var packages []string
	if p.Package != nil {
		packages = []string{p.Package.ID}
	}

	evidence = append(evidence,
		store.Evidence{Kind: store.EvidenceApproval, Subject: fmt.Sprintf("project proposal %d", p.ID),
			Detail: "approved by its owner: " + p.Name + " in " + p.Dir, OK: true},
		store.Evidence{Kind: store.EvidenceAgent, Subject: lead, Detail: "under " + strings.Join(packages, ", "), OK: true})

	hired := ""

	if hiredName != "" {
		if agent, ok := team.Find(team.Roster(b.Root), hiredName); ok {
			hired = fmt.Sprintf("%s — hired for this project from proposal %d, as its owner approved", agent.Title, p.ID)
		}
	}

	// Kept before the first step runs: see tasks.Taking.Evidence.
	task, started, err := b.Tasks.TakeWith(ctx, conversationID, p.Request.Sentence, b.Cfg.DefaultProvider, true,
		tasks.Taking{Lead: lead, Project: p.Dir, Packages: packages, Steps: steps, Name: p.Name,
			DoneWhen: strings.Join(p.Done, "; "), Evidence: evidence, Hired: hired,
			HowLong: time.Duration(p.Minutes) * time.Minute})
	if err != nil || !started || task == nil {
		if hiredName != "" {
			team.DropHire(b.Root, hiredName)
		}

		if err == nil {
			err = fmt.Errorf("the work could not be started")
		}

		return "", err
	}

	if hiredName != "" {
		if agent, ok := team.Find(team.Roster(b.Root), hiredName); ok {
			agent.HiredFor = task.ID
			team.Save(b.Root, agent)
			team.Forget()
		}
	}

	return fmt.Sprintf("Started %s in %s: %d files laid out, %s doing it. It is in Tasks.",
		p.Name, p.Dir, len(written), lead), nil
}

/*
 * scaffoldAgain makes the files again, and refuses when they are not the ones
 * that were shown. A package or an engine changed in between would otherwise
 * write something nobody approved.
 */
func (b *Brain) scaffoldAgain(p bootstrap.Proposal) ([]engines.File, error) {
	if len(p.Files) == 0 {
		return nil, nil
	}

	var files []engines.File

	switch {
	case p.Engine != "":
		adapter, ok := b.Engines.Get(p.Engine)
		if !ok {
			return nil, fmt.Errorf("the engine %s is no longer known", p.Engine)
		}

		made, err := adapter.Scaffold(engines.Scaffold{Name: p.Name, Engine: p.EngineVersion})
		if err != nil {
			return nil, err
		}

		files = made
	case p.Kind == "book":
		files, _ = workspace.Template("book", p.Name)
	default:
		files, _ = workspace.Template("plan", p.Name)
	}

	if len(files) != len(p.Files) {
		return nil, fmt.Errorf("the files to be written are no longer the ones that were approved")
	}

	for i := range files {
		if files[i].Path != p.Files[i] {
			return nil, fmt.Errorf("the files to be written are no longer the ones that were approved")
		}
	}

	return files, nil
}
