package brain

import (
	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/cite"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The trust layer: what an answer came from, and what was done and why.
 *
 * Two records, both kept by this program rather than reported by the model.
 * Sources are the numbered places an answer was built from (see the cite
 * package); deeds are every call made, with the request that led to it, who
 * acted, which model and why, and whether anything left the machine. The
 * model can be wrong about either. These cannot, because nothing in them is
 * its account of itself.
 */

// recordDeed writes one call down. A record that cannot be written is
// logged and the work goes on: the answer matters more than its receipt.
func (b *Brain) recordDeed(d agent.Deed) {
	if b.DB == nil {
		return
	}

	deed := store.Deed{
		ConversationID: d.ConversationID,
		Tool:           d.Step.Tool,
		Summary:        d.Step.Summary,
		Asked:          d.Step.Asked,
		Request:        d.Request,
		Result:         d.Step.Result,
		Specialist:     d.As,
		Model:          d.Model,
		ModelWhy:       d.ModelWhy,
		Outcome:        d.Outcome,
		Level:          string(d.Step.Level),
		Millis:         d.Step.Millis,
	}

	if b.Agent != nil && b.Agent.Registry != nil {
		if t, ok := b.Agent.Registry.Get(d.Step.Tool); ok {
			deed.Changes = t.Risk() == tools.Mutating

			// Only what actually ran can have sent anything.
			if d.Outcome == agent.DeedDone || d.Outcome == agent.DeedFailed {
				deed.Leaves = tools.Reaches(t, d.Args)
			}
		}
	}

	if _, err := b.DB.AddDeed(deed); err != nil {
		b.Log.Warn("could not record what was done", "tool", d.Step.Tool, "error", err)
	}
}

// recordApproved is an action its owner said yes to, carried out after the
// turn that asked had ended.
func (b *Brain) recordApproved(inv store.Invocation, tool tools.Tool, done bool) {
	outcome := agent.DeedDone
	if !done {
		outcome = agent.DeedFailed
	}

	b.recordInvocation(inv, outcome, tools.Reaches(tool, []byte(inv.Arguments)), tool.Risk() == tools.Mutating)
}

// recordDenied is an action its owner said no to.
func (b *Brain) recordDenied(inv store.Invocation) {
	b.recordInvocation(inv, agent.DeedRefused, "", true)
}

func (b *Brain) recordInvocation(inv store.Invocation, outcome, leaves string, changes bool) {
	if b.DB == nil {
		return
	}

	_, err := b.DB.AddDeed(store.Deed{
		ConversationID: inv.ConversationID,
		Tool:           inv.Tool,
		Summary:        inv.Summary,
		Asked:          inv.Arguments,
		Request:        "you approved it",
		Result:         inv.Result,
		Outcome:        outcome,
		Changes:        changes,
		Leaves:         leaves,
	})
	if err != nil {
		b.Log.Warn("could not record a decided action", "tool", inv.Tool, "error", err)
	}
}

// citing reports whether this turn asks the model to cite: typed turns with
// the setting on. A spoken answer is heard, and "[2]" read out is noise.
func (b *Brain) citing(spoken bool) bool {
	return b.Cfg.CiteSources && !spoken
}

// sourcesFor holds a finished answer to its ledger: invented numbers out,
// real ones marked cited.
func sourcesFor(reply string, ledger *cite.Ledger) (string, []cite.Source) {
	return cite.Check(reply, ledger.Sources())
}
