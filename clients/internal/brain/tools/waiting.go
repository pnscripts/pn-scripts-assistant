package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * The queue of things waiting for the owner to say yes.
 *
 * Two kinds of thing collect there: actions the brain proposed that change
 * something, and lessons it drew from a conversation that it wants to keep. The
 * interface has buttons for both, and until now that was the only way to answer
 * them — so being told out loud to "approve everything and remember everything"
 * produced a brain that said it would, and then did nothing at all, because it
 * had no way to.
 *
 * A promise it cannot keep is worse than a refusal. These are the tools that
 * make the sentence true.
 */

// Queue is what these tools need from the brain.
//
// An interface rather than the brain itself, because the brain imports the tool
// package to build its registry and the tools cannot import it back.
type Queue interface {
	PendingActions() ([]QueueItem, error)
	PendingLessons() ([]QueueItem, error)
	DecideAction(ctx context.Context, id int64, approve bool) (string, error)
	DecideLesson(ctx context.Context, id int64, keep bool) (string, error)
}

// QueueItem is one thing waiting, in the terms a model can reason about.
type QueueItem struct {
	ID      int64
	Summary string
}

/* ---------- listing ---------- */

// ListWaiting reports what is waiting for the owner.
type ListWaiting struct{ Queue Queue }

func (ListWaiting) Name() string { return "list_waiting" }

func (ListWaiting) Description() string {
	return "List everything waiting for the owner to decide: actions the brain " +
		"proposed that would change something, and things it wants to remember. " +
		"Use this before deciding, and whenever asked what is waiting or pending."
}

func (ListWaiting) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListWaiting) Risk() Risk { return Safe }

func (ListWaiting) Summarize(json.RawMessage) string { return "List what is waiting for you" }

func (t ListWaiting) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Queue == nil {
		return "", fmt.Errorf("there is no queue to read")
	}

	actions, err := t.Queue.PendingActions()
	if err != nil {
		return "", err
	}

	lessons, err := t.Queue.PendingLessons()
	if err != nil {
		return "", err
	}

	if len(actions) == 0 && len(lessons) == 0 {
		return "Nothing is waiting.", nil
	}

	var b strings.Builder

	if len(actions) > 0 {
		fmt.Fprintf(&b, "Actions waiting for approval (%d):\n", len(actions))

		for _, it := range actions {
			fmt.Fprintf(&b, "  %d. %s\n", it.ID, it.Summary)
		}
	}

	if len(lessons) > 0 {
		fmt.Fprintf(&b, "Things to remember, waiting (%d):\n", len(lessons))

		for _, it := range lessons {
			fmt.Fprintf(&b, "  %d. %s\n", it.ID, it.Summary)
		}
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

/* ---------- deciding ---------- */

/*
 * DecideWaiting answers what is waiting.
 *
 * Marked Safe, and that deserves saying out loud, because it is the tool that
 * carries out actions the approval gate is holding. The gate exists so that a
 * person decides rather than the model — and here a person has: this tool only
 * runs because its owner asked for it in the sentence they just spoke. Making
 * it Mutating would mean an approval needing an approval, which is not a
 * stricter rule, it is a broken one.
 *
 * What it does instead is account for itself. Every decision comes back naming
 * each item, so the answer read aloud is "approved 2: write greetings.txt,
 * remember that you prefer …" rather than "done".
 */
type DecideWaiting struct{ Queue Queue }

func (DecideWaiting) Name() string { return "decide_waiting" }

func (DecideWaiting) Description() string {
	return "Approve or refuse what is waiting for the owner. Use this whenever " +
		"the owner says to approve, accept, remember, keep, reject, deny, " +
		"discard or forget the things that are waiting. With no id, it decides " +
		"every waiting item the same way."
}

func (DecideWaiting) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"decision": {
				"type": "string",
				"enum": ["approve", "reject"],
				"description": "approve carries out actions and keeps things to remember; reject discards them"
			},
			"kind": {
				"type": "string",
				"enum": ["actions", "memories", "everything"],
				"description": "which queue to decide; defaults to everything"
			},
			"id": {
				"type": "integer",
				"description": "decide only this one item; omit to decide all of them"
			}
		},
		"required": ["decision"],
		"additionalProperties": false
	}`)
}

func (DecideWaiting) Risk() Risk { return Safe }

func (DecideWaiting) Summarize(raw json.RawMessage) string {
	var a decideArgs

	_ = json.Unmarshal(raw, &a)

	which := a.Kind
	if which == "" {
		which = "everything"
	}

	if a.ID > 0 {
		return fmt.Sprintf("%s item %d", a.Decision, a.ID)
	}

	return fmt.Sprintf("%s %s waiting", a.Decision, which)
}

type decideArgs struct {
	Decision string `json:"decision"`
	Kind     string `json:"kind"`
	ID       int64  `json:"id"`
}

func (t DecideWaiting) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if t.Queue == nil {
		return "", fmt.Errorf("there is no queue to decide")
	}

	var a decideArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	yes, err := readDecision(a.Decision)
	if err != nil {
		return "", err
	}

	kind := strings.ToLower(strings.TrimSpace(a.Kind))
	if kind == "" {
		kind = "everything"
	}

	var done []string

	if kind == "actions" || kind == "everything" {
		items, err := t.Queue.PendingActions()
		if err != nil {
			return "", err
		}

		for _, it := range items {
			if a.ID > 0 && it.ID != a.ID {
				continue
			}

			outcome, err := t.Queue.DecideAction(ctx, it.ID, yes)
			if err != nil {
				done = append(done, fmt.Sprintf("could not decide %d: %v", it.ID, err))

				continue
			}

			done = append(done, outcome)
		}
	}

	if kind == "memories" || kind == "everything" {
		items, err := t.Queue.PendingLessons()
		if err != nil {
			return "", err
		}

		for _, it := range items {
			if a.ID > 0 && it.ID != a.ID {
				continue
			}

			outcome, err := t.Queue.DecideLesson(ctx, it.ID, yes)
			if err != nil {
				done = append(done, fmt.Sprintf("could not decide %d: %v", it.ID, err))

				continue
			}

			done = append(done, outcome)
		}
	}

	if len(done) == 0 {
		if a.ID > 0 {
			return fmt.Sprintf("Nothing with id %d was waiting.", a.ID), nil
		}

		return "Nothing was waiting.", nil
	}

	// Named rather than counted, because this is the record of what somebody
	// agreed to without reading it themselves.
	return fmt.Sprintf("%d decided:\n  %s", len(done), strings.Join(done, "\n  ")), nil
}

// readDecision accepts the words people actually use.
//
// A small model asked to approve something writes "accept", "yes", "keep" and
// "remember" about as often as it writes the word in the schema, and refusing
// those is a tool that fails for no reason its owner can see.
func readDecision(word string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "approve", "approved", "accept", "yes", "keep", "remember", "allow", "ok":
		return true, nil
	case "reject", "rejected", "deny", "no", "discard", "forget", "refuse":
		return false, nil
	}

	return false, fmt.Errorf("decision must be approve or reject, not %q", word)
}
