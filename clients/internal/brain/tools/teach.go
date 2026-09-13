package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Teaching is what the skill tools need from the brain.
//
// An interface rather than the brain itself, because a tool that imported the
// brain would be a tool the brain could not hold.
type Teaching interface {
	SaveSkill(name, when, how string) error
	ForgetSkill(name string) error
	SkillNames() []string
}

/*
 * RememberHowToDoThis writes down the way its owner wants a job done.
 *
 * Mutating, and it is worth being clear about why, because the thing it writes
 * is only text. A skill is read back on later turns and followed — so what is
 * written here shapes what the assistant does for weeks, and a model that
 * could quietly write its own standing instructions would be a model that had
 * talked its way past every gate exactly once and then never needed to again.
 *
 * The summary is the whole skill, word for word, for the same reason the email
 * tool shows the whole message: an approval you cannot check is not one.
 *
 * What it writes cannot widen anything. Following a skill still means calling
 * ordinary tools, each stopping for approval as it always did — so the worst a
 * bad skill can do is waste a turn, which is the property that makes this a
 * reasonable thing to have at all.
 */
type RememberHowToDoThis struct {
	Brain Teaching
}

func (RememberHowToDoThis) Name() string { return "remember_how_to_do_this" }

func (RememberHowToDoThis) Description() string {
	return "Write down how a particular job should be done, so it is followed the " +
		"same way next time. Use when told to remember a way of working, a checklist, " +
		"a procedure, or 'do it like this from now on'. Not for facts about the owner, " +
		"which are remembered on their own."
}

func (RememberHowToDoThis) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "A short name, lower case, words joined by underscores. For example invoicing or new_client."
			},
			"when": {
				"type": "string",
				"description": "One sentence saying when this should be used, in the owner's words."
			},
			"how": {
				"type": "string",
				"description": "The instructions themselves, in full. Write the steps, not a summary of them."
			}
		},
		"required": ["name", "when", "how"]
	}`)
}

func (RememberHowToDoThis) Risk() Risk { return Mutating }

type skillArgs struct {
	Name string `json:"name"`
	When string `json:"when"`
	How  string `json:"how"`
}

// Summarize renders the whole skill. Somebody approving this is agreeing to
// instructions that will be followed later, and the only honest way to show
// that is to show it.
func (RememberHowToDoThis) Summarize(raw json.RawMessage) string {
	var a skillArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Write down how something is done"
	}

	var b strings.Builder

	b.WriteString("Remember how to do this, as \"" + a.Name + "\"\n")

	if a.When != "" {
		b.WriteString("Used when: " + a.When + "\n")
	}

	b.WriteString("\n" + strings.TrimSpace(a.How))

	return b.String()
}

func (t RememberHowToDoThis) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a skillArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to remember: %w", err)
	}

	if err := t.Brain.SaveSkill(a.Name, a.When, a.How); err != nil {
		return "", err
	}

	return "Written down as \"" + a.Name + "\". It will be followed next time this " +
		"comes up, and it can be read or changed in Skills.", nil
}

// WhatIveBeenTaught lists the skills, so asking is answered by looking rather
// than by remembering — the same rule as everything else about this machine.
type WhatIveBeenTaught struct {
	Brain Teaching
}

func (WhatIveBeenTaught) Name() string { return "what_ive_been_taught" }

func (WhatIveBeenTaught) Description() string {
	return "List the ways of working that have been written down. Use when asked " +
		"what you have been taught, what skills you have, or whether you know how " +
		"something is done here."
}

func (WhatIveBeenTaught) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (WhatIveBeenTaught) Risk() Risk { return Safe }

func (WhatIveBeenTaught) Summarize(json.RawMessage) string {
	return "List what it has been taught"
}

func (t WhatIveBeenTaught) Execute(context.Context, json.RawMessage) (string, error) {
	names := t.Brain.SkillNames()

	if len(names) == 0 {
		return "Nothing has been written down yet. Ways of working can be taught by " +
			"saying how something should be done, or by putting a file in the " +
			"skills folder.", nil
	}

	return "Written down so far: " + strings.Join(names, ", ") +
		". Call one by name to read how it is done.", nil
}

/*
 * ForgetHowToDoThis removes one.
 *
 * Mutating, because it deletes a file its owner wrote — and because a model
 * that could quietly drop the instruction it keeps failing to follow would be
 * a model that had solved the wrong problem.
 */
type ForgetHowToDoThis struct {
	Brain Teaching
}

func (ForgetHowToDoThis) Name() string { return "forget_how_to_do_this" }

func (ForgetHowToDoThis) Description() string {
	return "Delete a written-down way of working, by name. Use when told to forget " +
		"how something is done, or that a procedure no longer applies."
}

func (ForgetHowToDoThis) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The skill's name."}
		},
		"required": ["name"]
	}`)
}

func (ForgetHowToDoThis) Risk() Risk { return Mutating }

func (ForgetHowToDoThis) Summarize(raw json.RawMessage) string {
	var a skillArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Forget a way of working"
	}

	return "Delete the written-down way of doing \"" + a.Name + "\""
}

func (t ForgetHowToDoThis) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a skillArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read which one: %w", err)
	}

	if err := t.Brain.ForgetSkill(a.Name); err != nil {
		return "", err
	}

	return "Forgotten how to do \"" + a.Name + "\".", nil
}
