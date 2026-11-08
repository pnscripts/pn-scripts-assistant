package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/rules"
)

// RuleKeeping is what the rule tools need from the brain.
type RuleKeeping interface {
	KeepRule(text, source string) (rules.Rule, error)
	RuleTexts() []string
}

/*
 * KeepARule keeps a standing instruction as one of its user's rules.
 *
 * Most rules are noticed without the model — a sentence starting "always" or
 * "never" is proposed beside the answer — and this is for the rest: "make
 * that a rule", or an instruction put in words the noticing does not know.
 *
 * Mutating for the same reason as remember_how_to_do_this, and more so: a
 * rule can stop actions, and a model able to write its own rules would be a
 * model able to decide what it is never asked about. The summary is the rule
 * word for word, and nothing is kept until its user says yes.
 */
type KeepARule struct{ Brain RuleKeeping }

func (KeepARule) Name() string { return "keep_a_rule" }

func (KeepARule) Description() string {
	return "Keep a standing instruction as one of the user's rules, followed in everything from now on: " +
		"something to always do, never do, or ask about first. Use when told to make something a rule, or " +
		"to always or never do something from now on. Write the rule in the user's own words. Not for " +
		"procedures (remember_how_to_do_this) or facts about the user."
}

func (KeepARule) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"rule": {"type": "string", "description": "The rule, one or two sentences, in the user's words."},
			"said": {"type": "string", "description": "What the user said that this rule comes from, word for word."}
		},
		"required": ["rule"]
	}`)
}

func (KeepARule) Risk() Risk { return Mutating }

/*
 * AlwaysAsks: a rule is kept only with its user's yes, whatever else they
 * have allowed. A page the model read could otherwise have it write a rule
 * that every later prompt and every coding agent's brief would carry.
 */
func (KeepARule) AlwaysAsks(json.RawMessage) string {
	return "keeping a rule, which is followed from now on"
}

func (KeepARule) Cues() []string {
	return []string{"rule", "rules", "правило", "правила"}
}

type ruleArgs struct {
	Rule string `json:"rule"`
	Said string `json:"said"`
}

// Summarize is the rule as it will be kept, and what it was made from.
func (KeepARule) Summarize(raw json.RawMessage) string {
	var a ruleArgs

	if json.Unmarshal(raw, &a) != nil || strings.TrimSpace(a.Rule) == "" {
		return "Keep a rule"
	}

	out := "Keep as your rule: " + rules.Clean(a.Rule)

	if said := strings.TrimSpace(a.Said); said != "" {
		out += "\nFrom what you said: " + said
	}

	return out
}

func (t KeepARule) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var a ruleArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	r, err := t.Brain.KeepRule(a.Rule, a.Said)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Kept as rule %d: %s It is followed from now on and can be changed or switched off "+
		"on the Rules page.", r.ID, r.Text), nil
}

// MyRules lists the rules being followed, so "what are my rules" is
// answered by looking.
type MyRules struct{ Brain RuleKeeping }

func (MyRules) Name() string { return "my_rules" }

func (MyRules) Description() string {
	return "List the user's standing rules being followed now. Use when asked what the rules are, " +
		"or whether there is a rule about something."
}

func (MyRules) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (MyRules) Risk() Risk { return Safe }

func (MyRules) Cues() []string { return []string{"rule", "rules", "правило", "правила"} }

func (MyRules) Summarize(json.RawMessage) string { return "List your rules" }

func (t MyRules) Execute(context.Context, json.RawMessage) (string, error) {
	texts := t.Brain.RuleTexts()

	if len(texts) == 0 {
		return "There are no rules yet. Say \"always…\" or \"never…\", or add one on the Rules page.", nil
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("%d rules are followed now:\n", len(texts)))

	for _, text := range texts {
		b.WriteString("- " + text + "\n")
	}

	return strings.TrimRight(b.String(), "\n"), nil
}
