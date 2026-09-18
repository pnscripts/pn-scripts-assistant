package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/risk"
)

/*
 * Hiring, in conversation: propose first, then ask, then hire.
 *
 * Two tools rather than one, and the split is the whole design. hire_agent
 * works out who would be hired and writes nothing, so it is safe and can be
 * used freely — "who would you hire for this?" is a fine question. What it
 * produces is a proposal with a number, kept as it was shown. confirm_hire
 * carries out exactly that proposal, and it changes the organisation, so its
 * owner is asked — and for a permanent hire, asked whatever the settings say,
 * because a permanent member of the organisation will act in its name from
 * then on.
 */

// Hiring is what the tools need from the brain.
type Hiring interface {
	ProposeHire(ctx context.Context, request, permanence, goal string) (HireOffer, error)

	// ShowHire is the proposal as it would be carried out, for the approval.
	ShowHire(id int64, permanence, goal string) (string, bool)

	ConfirmHire(ctx context.Context, id int64, permanence, goal string) (string, error)
}

// HireOffer is what a proposal came to.
type HireOffer struct {
	ID         int64
	Text       string
	Existing   bool
	Permanence string
	Goal       string
}

// HireAgent proposes a hire.
type HireAgent struct {
	Hiring Hiring
}

func (HireAgent) Name() string { return "hire_agent" }

func (HireAgent) Description() string {
	return "Propose hiring an expert into the organisation — a game developer, an electrician, " +
		"a book writer, a backend engineer. Works out the job, whether somebody here already " +
		"does it, what they could use, what the machine would need, and what needs a " +
		"professional. Writes nothing: it returns a numbered proposal to show the owner."
}

func (HireAgent) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"request": {"type": "string", "description": "Who is wanted, in the owner's words: \"a game-development expert\", \"an electrician to help plan this room\"."},
			"permanence": {"type": "string", "enum": ["permanent", "task"], "description": "Only if the owner said: permanent, or for one task only."},
			"goal": {"type": "string", "description": "For a hire for one task: the task, in the owner's words."}
		},
		"required": ["request"],
		"additionalProperties": false
	}`)
}

func (HireAgent) Risk() Risk { return Safe }

func (HireAgent) Summarize(args json.RawMessage) string {
	var a struct {
		Request string `json:"request"`
	}

	json.Unmarshal(args, &a)

	return "Work out who to hire: " + a.Request
}

func (t HireAgent) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Request    string `json:"request"`
		Permanence string `json:"permanence"`
		Goal       string `json:"goal"`
	}

	if err := argsOf(args, &a); err != nil {
		return "", err
	}

	if t.Hiring == nil {
		return "", fmt.Errorf("hiring is not available here")
	}

	offer, err := t.Hiring.ProposeHire(ctx, a.Request, a.Permanence, a.Goal)
	if err != nil {
		return "", err
	}

	return offer.Text + "\n\n" + NextAboutHire(offer), nil
}

/*
 * NextAboutHire is what the model is told to do with a proposal.
 *
 * Said to it outright, because the failure worth preventing is a model
 * deciding "permanent" for somebody because it seemed likely.
 */
func NextAboutHire(offer HireOffer) string {
	switch {
	case offer.Existing:
		return "Nobody new is needed. Tell the owner who already does this."
	case offer.Permanence == "":
		return fmt.Sprintf("Show the owner this proposal (number %d) and ask, with ask_first: "+
			"should this hire be permanent, or only for one task? Do not choose for them.", offer.ID)
	case offer.Permanence == "task" && offer.Goal == "":
		return fmt.Sprintf("Show the owner this proposal (number %d) and ask, with ask_first, "+
			"what the one task is.", offer.ID)
	default:
		return fmt.Sprintf("Show the owner this proposal. To make the hire, call confirm_hire with "+
			"proposal %d, permanence %q%s — they will be asked to approve it before anything is written.",
			offer.ID, offer.Permanence, goalArg(offer.Goal))
	}
}

func goalArg(goal string) string {
	if goal == "" {
		return ""
	}

	return fmt.Sprintf(" and goal %q", goal)
}

// ConfirmHire makes a proposed hire, once its owner has said yes.
type ConfirmHire struct {
	Hiring Hiring
}

func (ConfirmHire) Name() string { return "confirm_hire" }

func (ConfirmHire) Description() string {
	return "Make a hire that hire_agent proposed, exactly as proposed, once the owner has said " +
		"whether it is permanent or for one task. Requires the owner's approval."
}

func (ConfirmHire) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"proposal": {"type": "integer", "description": "The proposal's number."},
			"permanence": {"type": "string", "enum": ["permanent", "task"]},
			"goal": {"type": "string", "description": "For a hire for one task: the task."}
		},
		"required": ["proposal", "permanence"],
		"additionalProperties": false
	}`)
}

func (ConfirmHire) Risk() Risk { return Mutating }

type confirmArgs struct {
	Proposal   int64  `json:"proposal"`
	Permanence string `json:"permanence"`
	Goal       string `json:"goal"`
}

// Weight: somebody permanent is a lasting change to who acts in its owner's
// name; somebody for one task is gone when it ends.
func (ConfirmHire) Weight(args json.RawMessage) risk.Level {
	var a confirmArgs

	argsOf(args, &a)

	if a.Permanence == "permanent" {
		return risk.High
	}

	return risk.Medium
}

// Consent: a permanent hire is asked about whatever the settings say.
func (ConfirmHire) Consent(args json.RawMessage) string {
	var a confirmArgs

	argsOf(args, &a)

	if a.Permanence == "permanent" {
		return "a permanent member of the organisation"
	}

	return ""
}

// Summarize is the whole proposal, as it will be carried out — not a line
// about it. What is approved is what was read.
func (t ConfirmHire) Summarize(args json.RawMessage) string {
	var a confirmArgs

	argsOf(args, &a)

	if t.Hiring != nil {
		if shown, ok := t.Hiring.ShowHire(a.Proposal, a.Permanence, a.Goal); ok {
			return shown
		}
	}

	return fmt.Sprintf("Make hire %d (%s)", a.Proposal, a.Permanence)
}

func (t ConfirmHire) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a confirmArgs

	if err := argsOf(args, &a); err != nil {
		return "", err
	}

	if t.Hiring == nil {
		return "", fmt.Errorf("hiring is not available here")
	}

	switch a.Permanence {
	case "permanent", "task":
	default:
		return "", fmt.Errorf("permanent or task? The owner has to say which")
	}

	if a.Permanence == "task" && strings.TrimSpace(a.Goal) == "" {
		return "", fmt.Errorf("a hire for one task needs the task — ask the owner what it is")
	}

	return t.Hiring.ConfirmHire(ctx, a.Proposal, a.Permanence, a.Goal)
}
