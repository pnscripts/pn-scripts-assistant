package agent

import (
	"encoding/json"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The user's rules at the moment of acting (see package rules).
 *
 * Checked on every call that does something outward or lasting, after the
 * owner's other refusals and before the approval gate: a rule that plainly
 * forbids the call stops it and is named; one that wants its user asked puts
 * the call in front of them with the rule beside it, whatever else has been
 * allowed — "never stop", goal mode and everything allowed included, because
 * a rule is its user's own refusal, and those are the things ADR 0007 never
 * lifted.
 */

// ActOf is a call as the rules see it: what kinds of action it is, and
// everything it says, for a rule's words to be looked for in.
func ActOf(tool tools.Tool, args json.RawMessage, summary string, at time.Time) rules.Act {
	return rules.Act{
		Actions: tools.ActionsOf(tool, args),
		Text:    summary + "\n" + string(args),
		At:      at,
	}
}

func (l *Loop) liveRules() []rules.Rule {
	if l.Rules == nil {
		return nil
	}

	return l.Rules()
}

// StoppedBy is what the model and the record are told when a rule stopped a
// call: which rule, and not to try a way round it.
func StoppedBy(r rules.Rule) string {
	return "Stopped by your rule \"" + r.Text + "\" (kept " + r.CreatedAt.Local().Format("2 Jan 2006") + "). " +
		"Do not try it another way; say which rule stopped it and ask what to do instead. " +
		"The rule can be changed on the Rules page."
}

// RuleLines is the rules an approval is shown with, one line each.
func RuleLines(asking, reminding []rules.Rule) string {
	var b strings.Builder

	for _, r := range asking {
		b.WriteString("Your rule: \"" + r.Text + "\"\n")
	}

	for _, r := range reminding {
		b.WriteString("Remember your rule: \"" + r.Text + "\"\n")
	}

	return b.String()
}

// RuleLinesIn is RuleLines in a language, for the approval card: the
// rule itself as its user wrote it, the words around it translated.
func RuleLinesIn(asking, reminding []rules.Rule, lang string) string {
	if lang != "bg" {
		return RuleLines(asking, reminding)
	}

	var b strings.Builder

	for _, r := range asking {
		b.WriteString("Ваше правило: „" + r.Text + "“\n")
	}

	for _, r := range reminding {
		b.WriteString("Припомняне на ваше правило: „" + r.Text + "“\n")
	}

	return b.String()
}

// orElse is s, or else when s is empty.
func orElse(s, otherwise string) string {
	if s == "" {
		return otherwise
	}

	return s
}

// BrokeAfterwards is the note added to what a call did when it turned out
// to go against a rule.
func BrokeAfterwards(broke []rules.Rule) string {
	if len(broke) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("\n\nThis went against ")

	for i, r := range broke {
		if i > 0 {
			b.WriteString(" and ")
		}

		b.WriteString("your rule \"" + r.Text + "\"")
	}

	b.WriteString(". Tell your user plainly what was done and where.")

	return b.String()
}
