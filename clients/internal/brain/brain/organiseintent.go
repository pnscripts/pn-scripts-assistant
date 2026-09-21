package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Hiring and projects, asked for in so many words, answered without a model.
 *
 * "Hire a game developer" and "make me a Tetris game" are instructions, and
 * both have exactly one right first move: work out a proposal and show it, or
 * ask the one question that has to be answered first. That is deterministic,
 * it takes no time, and a small model reliably gets it wrong in one of two
 * ways — it describes hiring somebody instead of proposing it, or it guesses
 * a folder. So the obvious phrasings are handled here, and the model is still
 * there, with the same tools, for everything these do not recognise.
 *
 * An answer to a question asked here is also read here: "permanent", a folder,
 * "three.js", "yes". Anything else falls through to the model, and the
 * proposal stays open for a while in case the answer comes later.
 *
 * Nothing material happens on this path. What would change the organisation
 * or the disk is put to its owner as an approval, the same one the approvals
 * panel shows, carried out only when they press it.
 */

// HowLongAQuestionStaysOpen is how long an answer is taken as the answer to
// a proposal's question.
const HowLongAQuestionStaysOpen = 30 * time.Minute

var hireAsk = regexp.MustCompile(`(?i)^\s*(?:(?:hey|hi|ok|okay)[\s,]+(?:\p{L}+)?[\s,!:]*)?(?:please[\s,]+)?` +
	`(?:(?:can|could|would)\s+you\s+)?(?:please\s+)?(?:hire|наеми)\s+(.+)$`)

var projectAsk = regexp.MustCompile(`(?i)^\s*(?:(?:hey|hi|ok|okay)[\s,]+(?:\p{L}+)?[\s,!:]*)?(?:please[\s,]+)?` +
	`(?:(?:can|could|would)\s+you\s+)?(?:please\s+)?` +
	`(?:make|build|create|write|develop|start|help\s+me\s+(?:plan|build|make|write))\s+(?:me\s+|us\s+)?(.+)$`)

// affirmative is a short yes, in either language — or a few of them run
// together, "yes, start it", "yes please".
var affirmative = regexp.MustCompile(`(?i)^\s*(?:(?:yes|yeah|yep|ok|okay|sure|go ahead|go|do it|start it|start|approve|` +
	`approved|please|sounds good|да|добре|давай|започни)[\s,.!]*)+$`)

// handleOrganising answers what it recognises, and says whether it did.
func (b *Brain) handleOrganising(ctx context.Context, conversationID int64, message string) (string, []agent.Pending, bool) {
	if b.Tasks == nil || b.Engines == nil {
		return "", nil, false
	}

	if reply, pending, ok := b.answeringAProject(ctx, conversationID, message); ok {
		return reply, pending, true
	}

	if reply, pending, ok := b.answeringAHire(ctx, conversationID, message); ok {
		return reply, pending, true
	}

	if m := hireAsk.FindStringSubmatch(message); m != nil {
		return b.proposeAHire(ctx, conversationID, strings.TrimSpace(m[1]))
	}

	if m := projectAsk.FindStringSubmatch(message); m != nil {
		if kind, _ := kindWords(message); kind {
			folder, _ := tools.LooksLikeAFolder(message)

			offer, err := b.proposeProjectIn(conversationID, bootstrap.Request{Sentence: strings.TrimSpace(message), Dir: folder})
			if err != nil {
				return "", nil, false
			}

			return b.aboutAProject(ctx, conversationID, offer)
		}
	}

	return "", nil, false
}

// kindWords is whether a request names a kind of project at all — a game, a
// book, an API — so "make me a coffee" is left to the model.
func kindWords(message string) (bool, string) {
	lower := " " + strings.ToLower(message) + " "

	for _, w := range []string{" game", "tetris", " book", "novel", " api", "website", " web page", "landing page",
		" backend", "electrical", "wiring", "construction", "renovation", " игра", " книга", " роман"} {
		if strings.Contains(lower, w) {
			return true, strings.TrimSpace(w)
		}
	}

	return false, ""
}

func (b *Brain) proposeAHire(ctx context.Context, conversationID int64, request string) (string, []agent.Pending, bool) {
	offer, err := b.proposeHireIn(conversationID, request, "", "")
	if err != nil {
		return "I could not work out who that would be: " + err.Error(), nil, true
	}

	return b.aboutAHire(ctx, conversationID, offer)
}

// aboutAHire is what to say about a proposal: it, and the question it needs,
// or the approval it is waiting for.
func (b *Brain) aboutAHire(ctx context.Context, conversationID int64, offer tools.HireOffer) (string, []agent.Pending, bool) {
	switch {
	case offer.Existing:
		return offer.Text, nil, true
	case offer.Permanence == "":
		return offer.Text + "\n\n" + b.inOwnWords(ctx, "the one question under a proposed hire",
			map[string]any{"what you need to know": "whether the hire is permanent or for one task only"},
			"Should this hire be permanent, or only for one task?", 160), nil, true

	case offer.Permanence == team.TaskOnly && offer.Goal == "":
		return offer.Text + "\n\n" + b.inOwnWords(ctx, "the one question under a proposed hire for a single task",
			map[string]any{"what you need to know": "which task the hire is for"},
			"What is the one task?", 160), nil, true
	}

	pending, err := b.askToConfirmHire(conversationID, offer.ID, offer.Permanence, offer.Goal)
	if err != nil {
		return offer.Text + "\n\n" + err.Error(), nil, true
	}

	return offer.Text + "\n\n" + b.inOwnWords(ctx,
		"one closing line under a proposed hire that is waiting to be approved",
		map[string]any{"it will be done exactly as written above": true},
		"Approve it below and it will be done exactly as written here.", 160), pending, true
}

// askToConfirmHire puts the hire to its owner, as an approval.
func (b *Brain) askToConfirmHire(conversationID, id int64, permanence, goal string) ([]agent.Pending, error) {
	args, _ := json.Marshal(map[string]any{"proposal": id, "permanence": permanence, "goal": goal})

	return b.askToApprove(conversationID, "confirm_hire", args)
}

// askToApprove records a call as waiting for its owner, exactly as the loop
// would have, so it appears where every other approval does.
func (b *Brain) askToApprove(conversationID int64, name string, args json.RawMessage) ([]agent.Pending, error) {
	tool, ok := b.Agent.Registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("%s is not available", name)
	}

	summary := tool.Summarize(args)

	if why := tools.ConsentFor(tool, args); why != "" {
		summary = "Always asked — " + why + ": " + summary
	}

	id, err := b.DB.RecordInvocation(conversationID, name, string(args), summary, string(tools.Mutating))
	if err != nil {
		return nil, err
	}

	return []agent.Pending{{ID: id, Tool: name, Summary: summary}}, nil
}

/*
 * answeringAHire reads a reply to a hire proposal's question: permanent or
 * one task, and for one task, which.
 */
func (b *Brain) answeringAHire(ctx context.Context, conversationID int64, message string) (string, []agent.Pending, bool) {
	row, err := b.DB.OpenProposal(conversationID, store.ProposeHire, HowLongAQuestionStaysOpen)
	if err != nil || row == nil {
		return "", nil, false
	}

	var p team.Proposal

	if json.Unmarshal([]byte(row.Body), &p) != nil || p.Existing != nil {
		return "", nil, false
	}

	switch {
	case p.Permanence == "":
		answer := team.Answer(message)
		if answer == "" {
			return "", nil, false
		}

		p.Permanence = answer

		if answer == team.TaskOnly && p.Goal == "" {
			if body, err := json.Marshal(p); err == nil {
				b.DB.UpdateProposal(row.ID, string(body))
			}

			return "For one task, then. What is the task?", nil, true
		}

	case p.Permanence == team.TaskOnly && p.Goal == "":
		if strings.TrimSpace(message) == "" || strings.HasSuffix(strings.TrimSpace(message), "?") {
			return "", nil, false
		}

		p.Goal = strings.TrimSpace(message)

	default:
		return "", nil, false
	}

	if body, err := json.Marshal(p); err == nil {
		b.DB.UpdateProposal(row.ID, string(body))
	}

	pending, err := b.askToConfirmHire(conversationID, row.ID, p.Permanence, p.Goal)
	if err != nil {
		return err.Error(), nil, true
	}

	how := "permanently"
	if p.Permanence == team.TaskOnly {
		how = "for this task only: " + p.Goal
	}

	return b.inOwnWords(ctx, "one line saying who is being hired and on what terms, waiting to be approved",
		map[string]any{"who": p.Agent.Title, "their name here": p.Agent.Name, "terms": how,
			"it will be done exactly as proposed": true},
		fmt.Sprintf("%s (%s), %s. Approve it below and it will be done exactly as proposed.",
			p.Agent.Title, p.Agent.Name, how), 200), pending, true
}

/*
 * answeringAProject reads a reply to a project proposal: the folder it asked
 * for, the engine it offered, or a yes to a proposal that is ready.
 */
func (b *Brain) answeringAProject(ctx context.Context, conversationID int64, message string) (string, []agent.Pending, bool) {
	row, err := b.DB.OpenProposal(conversationID, store.ProposeProject, HowLongAQuestionStaysOpen)
	if err != nil || row == nil {
		return "", nil, false
	}

	var p bootstrap.Proposal

	if json.Unmarshal([]byte(row.Body), &p) != nil {
		return "", nil, false
	}

	request := p.Request

	switch {
	case p.Question != "" && len(p.Options) > 0:
		chosen := chosenOption(message, p.Options)
		if chosen == "" {
			return "", nil, false
		}

		request.Package = chosen

	case p.Question != "":
		folder, ok := tools.LooksLikeAFolder(message)
		if !ok {
			return "", nil, false
		}

		request.Dir = folder

	case p.Question == "" && !p.Ready() && affirmative.MatchString(message):
		/*
		 * Answered from the facts rather than left to the model to work out:
		 * asked to make sense of a blocked "yes" on this machine, a 7B model
		 * called the wrong tool twice and said nothing useful. What it may do
		 * is put these words; what it may not do is decide them.
		 */
		return b.inOwnWords(ctx, "an answer to somebody who said yes to a proposal that cannot be started",
			map[string]any{
				"what stops it":            p.Blockers,
				"nothing has been started": true,
				"what they can do":         "deal with that, then ask again and it will be proposed afresh",
			},
			"It cannot start until:\n  - "+strings.Join(p.Blockers, "\n  - ")+
				"\n\nNothing has been started. Once that is dealt with, ask again and it will be proposed afresh.",
			320), nil, true

	case p.Ready() && affirmative.MatchString(message):
		args, _ := json.Marshal(map[string]any{"proposal": row.ID})

		pending, err := b.askToApprove(conversationID, "start_project", args)
		if err != nil {
			return err.Error(), nil, true
		}

		return b.inOwnWords(ctx, "one line telling somebody their yes is now waiting for them to approve it",
			map[string]any{"what it will start": p.Name, "where": p.Dir,
				"it will be done exactly as proposed": true},
			"Approve it below and I will start "+p.Name+" exactly as proposed.", 200), pending, true

	default:
		return "", nil, false
	}

	// The old proposal is superseded by the one with the answer in it.
	b.DB.DecideProposal(row.ID, store.ProposalSuperseded, "answered")

	offer, err := b.proposeProjectIn(conversationID, request)
	if err != nil {
		return err.Error(), nil, true
	}

	return b.aboutAProject(ctx, conversationID, offer)
}

// aboutAProject is a project proposal said: its question, or it and the
// question of whether to start.
func (b *Brain) aboutAProject(ctx context.Context, conversationID int64, offer tools.ProjectOffer) (string, []agent.Pending, bool) {
	switch {
	case offer.Question != "":
		// Nothing but talking: no proposal to show yet, only the one thing it
		// needs answered.
		return b.inOwnWords(ctx, "the one question you need answered before you can propose a project",
			map[string]any{"what you need to know": offer.Question}, offer.Text, 200), nil, true

	case !offer.Ready:
		_, p, _ := b.projectProposal(offer.ID)

		return offer.Text + "\n\n" + b.inOwnWords(ctx,
			"one closing line under a proposal that cannot be started yet",
			map[string]any{"what stops it": p.Blockers, "nothing has been started": true},
			"It cannot start until those are dealt with.", 200), nil, true
	}

	_, p, _ := b.projectProposal(offer.ID)

	return offer.Text + "\n\n" + b.inOwnWords(ctx,
		"one closing line under a proposal, asking whether to start it",
		map[string]any{"what it would make": p.Name, "where": p.Dir,
			"saying yes puts it to you as an approval first": true},
		"Shall I start it? Say yes and it will be put to you to approve.", 200), nil, true
}

// chosenOption is which offered package a reply names, by its title or the
// engine's name.
func chosenOption(message string, options []bootstrap.Option) string {
	lower := strings.ToLower(message)

	names := map[string][]string{
		"software.game.godot":   {"godot"},
		"software.game.threejs": {"three", "threejs", "three.js", "web", "browser"},
		"software.game.unity":   {"unity"},
		"software.game.unreal":  {"unreal", "ue5"},
	}

	for _, o := range options {
		if strings.Contains(lower, strings.ToLower(o.Title)) {
			return o.Package
		}

		for _, name := range names[o.Package] {
			if strings.Contains(lower, name) {
				return o.Package
			}
		}
	}

	return ""
}
