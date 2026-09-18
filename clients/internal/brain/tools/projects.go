package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/risk"
)

/*
 * Projects: look, propose, then — with its owner's yes — start.
 *
 * The same split as hiring and for the same reason. Looking at a folder and
 * working out a plan change nothing and are safe. Starting writes files into
 * somebody's folder, may install an engine and may hire somebody for the job,
 * so it is always put to its owner, with the whole plan as the question.
 */

// Projects is what the tools need from the brain.
type Projects interface {
	InspectProject(dir string) string
	ProposeProject(ctx context.Context, request, dir, pkg string) (ProjectOffer, error)
	ShowProject(id int64) (string, bool)
	StartProject(ctx context.Context, id int64) (string, error)
}

// ProjectOffer is what a proposal came to.
type ProjectOffer struct {
	ID       int64
	Text     string
	Question string
	Ready    bool
}

// InspectProject looks at a folder before anything is done in it.
type InspectProject struct {
	Projects Projects
}

func (InspectProject) Name() string { return "inspect_project" }

func (InspectProject) Description() string {
	return "Look at a folder before working in it: whether it exists, whether it is already a " +
		"project and of what (engine, language), its settings, uncommitted changes, conventions, " +
		"its own build and test commands, and anything risky. Changes nothing."
}

func (InspectProject) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"path": {"type": "string", "description": "The whole path to the folder."}},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

func (InspectProject) Risk() Risk { return Safe }

func (InspectProject) Summarize(raw json.RawMessage) string {
	return "Look at " + pathOf(raw)[0]
}

func (t InspectProject) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	path := pathOf(raw)[0]

	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("which folder? Give its whole path")
	}

	return t.Projects.InspectProject(path), nil
}

// PlanProject works out a project from a request, and asks what it must.
type PlanProject struct {
	Projects Projects
}

func (PlanProject) Name() string { return "plan_project" }

func (PlanProject) Description() string {
	return "Work out a project the owner asked for — \"make me a Tetris game\", \"build an API " +
		"in this folder\", \"plan the electrical work for this room\": where it goes, what it is " +
		"built with, who does it, what the machine needs, the steps and what finished means. " +
		"Writes nothing. Never guess the folder: leave it out and the owner is asked."
}

func (PlanProject) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"request": {"type": "string", "description": "What the owner asked for, in their words."},
			"folder": {"type": "string", "description": "Only a folder the owner named. Leave it out otherwise."},
			"package": {"type": "string", "description": "Only when the owner chose: a capability package id such as software.game.godot or software.game.threejs."}
		},
		"required": ["request"],
		"additionalProperties": false
	}`)
}

func (PlanProject) Risk() Risk { return Safe }

func (PlanProject) Summarize(raw json.RawMessage) string {
	var a struct{ Request string }

	json.Unmarshal(raw, &a)

	return "Work out the project: " + a.Request
}

func (t PlanProject) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Request string `json:"request"`
		Folder  string `json:"folder"`
		Package string `json:"package"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	offer, err := t.Projects.ProposeProject(ctx, a.Request, a.Folder, a.Package)
	if err != nil {
		return "", err
	}

	return offer.Text + "\n\n" + NextAboutProject(offer), nil
}

// NextAboutProject is what the model is told to do with a proposal.
func NextAboutProject(offer ProjectOffer) string {
	switch {
	case offer.Question != "":
		return "Ask the owner exactly this, with ask_first, and do not answer it yourself: " + offer.Question
	case !offer.Ready:
		return "Tell the owner what stops it starting. Nothing can be done until they act."
	default:
		return fmt.Sprintf("Show the owner this proposal. To start it, call start_project with "+
			"proposal %d — they will be asked to approve it before anything is written.", offer.ID)
	}
}

// StartProject carries out an approved project proposal.
type StartProject struct {
	Projects Projects
}

func (StartProject) Name() string { return "start_project" }

func (StartProject) Description() string {
	return "Start a project that plan_project proposed, exactly as proposed: create its files " +
		"(never overwriting anything), install what it needs, take on whoever does it, and begin " +
		"the work. Requires the owner's approval."
}

func (StartProject) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"proposal": {"type": "integer", "description": "The proposal's number."}},
		"required": ["proposal"],
		"additionalProperties": false
	}`)
}

func (StartProject) Risk() Risk { return Mutating }

func proposalOf(raw json.RawMessage) int64 {
	var a struct {
		Proposal int64 `json:"proposal"`
	}

	json.Unmarshal(raw, &a)

	return a.Proposal
}

// Consent: it writes into somebody's folder and may install and hire.
func (StartProject) Consent(json.RawMessage) string {
	return "a new project in your folders"
}

func (StartProject) Weight(json.RawMessage) risk.Level { return risk.High }

// Summarize is the whole plan: what is approved is what was read.
func (t StartProject) Summarize(raw json.RawMessage) string {
	id := proposalOf(raw)

	if t.Projects != nil {
		if shown, ok := t.Projects.ShowProject(id); ok {
			return shown
		}
	}

	return fmt.Sprintf("Start project proposal %d", id)
}

func (t StartProject) Touches(raw json.RawMessage) ([]string, []string) {
	return nil, nil
}

func (t StartProject) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if t.Projects == nil {
		return "", fmt.Errorf("projects are not available here")
	}

	return t.Projects.StartProject(ctx, proposalOf(raw))
}

// looksLikeAFolder is whether a reply names a folder: a whole path, or one
// from the home folder.
func LooksLikeAFolder(text string) (string, bool) {
	for _, word := range strings.Fields(text) {
		word = strings.Trim(word, "\"'`,.;:!?()")

		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~/") {
			return filepath.Clean(word), true
		}
	}

	return "", false
}
