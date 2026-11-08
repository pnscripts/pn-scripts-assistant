package brain

import (
	"errors"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/rules"
)

/*
 * Its user's rules, as the brain holds them (see package rules).
 *
 * Noticed in what its user says and proposed with their words, never kept
 * until they say so; read into every turn and every step of a goal; checked
 * by the agent loop before and after every outward or lasting call, and
 * again here before an approved action is carried out, because a rule kept
 * while the action waited still counts.
 */

// LiveRules is the rules being followed now. A brain whose rules cannot be
// read follows none rather than refusing everything; the failure is logged.
func (b *Brain) LiveRules() []rules.Rule {
	if b.DB == nil {
		return nil
	}

	live, err := b.DB.LiveRules()
	if err != nil {
		b.Log.Warn("could not read the rules", "error", err)

		return nil
	}

	return live
}

// rulesBlock is the live rules as the model reads them, within most runes.
func (b *Brain) rulesBlock(most int) string {
	return rules.Block(b.LiveRules(), most)
}

// withRules adds the live rules to a persona.
func (b *Brain) withRules(persona string, most int) string {
	if block := b.rulesBlock(most); block != "" {
		return persona + "\n\n" + block
	}

	return persona
}

// RulesInPrompt is how much of a prompt the rules may take, in characters:
// enough for a few dozen sentences, never the whole of a small model's
// window. Rules that do not fit are counted and named on the Rules page.
const RulesInPrompt = 4000

// RulesInSpokenPrompt is the same for a spoken turn, where the prompt is
// the wait.
const RulesInSpokenPrompt = 1200

// RuleProposal is a proposed rule as a reply carries it: with what it would
// do to an action, so the card can say so before it is kept.
type RuleProposal struct {
	rules.Rule
	Mode  string      `json:"mode"`
	Scope rules.Scope `json:"scope"`
}

/*
 * noticeRule proposes a rule from what was just said, when it holds one that
 * has not been proposed before. The proposal waits for its user's answer;
 * nil when there is nothing to propose.
 */
func (b *Brain) noticeRule(message string, conversationID int64) *RuleProposal {
	p, ok := rules.Notice(message)
	if !ok {
		return nil
	}

	if known, err := b.DB.KnownRule(p.Text); err != nil || known {
		return nil
	}

	r, err := b.DB.AddRule(p.Rule(conversationID, time.Now().UTC()))
	if err != nil {
		b.Log.Warn("could not keep a proposed rule", "error", err)

		return nil
	}

	return &RuleProposal{Rule: r, Mode: rules.Mode(r), Scope: rules.ScopeOf(r)}
}

/*
 * KeepRule is a rule the model was asked to keep, in words its user has
 * seen in the approval. The same words already proposed are kept rather
 * than added twice; a rule already kept is said to be.
 */
func (b *Brain) KeepRule(text, source string) (rules.Rule, error) {
	text = rules.Clean(text)
	if text == "" {
		return rules.Rule{}, errors.New("a rule needs words")
	}

	all, err := b.DB.Rules(true)
	if err != nil {
		return rules.Rule{}, err
	}

	for _, r := range all {
		if !strings.EqualFold(r.Text, text) {
			continue
		}

		switch r.Status {
		case rules.Kept:
			if !r.On {
				return b.DB.SwitchRule(r.ID, true)
			}

			return r, nil
		case rules.Proposed:
			return b.DB.DecideRule(r.ID, true)
		}

		// Declined earlier and asked for now in so many words: asked for
		// wins, as a new rule.
	}

	p, ok := rules.Notice(text)
	if !ok {
		p = rules.Proposal{Text: text, Actions: rules.ActionsIn(text), Words: rules.SpecificWords(text), Days: rules.DaysIn(text)}
	}

	r := p.Rule(b.CurrentConversation(), time.Now().UTC())
	r.Text, r.Source, r.Status, r.On = text, strings.TrimSpace(source), rules.Kept, true

	return b.DB.AddRule(r)
}

// RuleTexts is the live rules' words, for the model to read back.
func (b *Brain) RuleTexts() []string {
	var out []string

	for _, r := range b.LiveRules() {
		out = append(out, r.Text)
	}

	return out
}
