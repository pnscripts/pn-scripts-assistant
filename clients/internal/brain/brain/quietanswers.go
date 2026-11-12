package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/quiethours"
	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Questions held for the quiet hours alone answer themselves when they open.
 *
 * A call to a coding agent whose allowance is ahead of its pace is put to its
 * user outside the quiet hours, even when everything else had said yes (see
 * tools.QuietHolding). Its user may never come back to it, and then the work
 * they had already allowed waits for ever. So a question held for that and
 * nothing else is approved by itself when the window opens, unless its user
 * answered first; its answer goes to the conversation it was asked in.
 *
 * Judged again at the opening, on what is true then: a permission set to ask
 * or never, a rule that asks about it now, a protected file, consent, or full
 * access leave it to its user, and it stops promising to start by itself.
 * Only while a window is set and open: with no window, nothing answers itself.
 */

// howOftenToLookAtHeldQuestions is the longest a held question can wait
// after its window opened when the setting changed while the loop slept.
const howOftenToLookAtHeldQuestions = 10 * time.Minute

func (b *Brain) watchHeldQuestions(ctx context.Context) {
	for {
		b.AnswerHeldQuestions(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(quiethours.Sleep(b.QuietHours(), time.Now(), howOftenToLookAtHeldQuestions)):
		}
	}
}

// AnswerHeldQuestions approves every held question that answers itself, when
// the quiet hours are open, one at a time and oldest first, and says how
// many it started.
func (b *Brain) AnswerHeldQuestions(ctx context.Context) int {
	if b.DB == nil || !quiethours.OpenNow(b.QuietHours(), time.Now()) {
		return 0
	}

	waiting, err := b.DB.WaitingForQuietHours()
	if err != nil {
		b.Log.Warn("could not read the questions waiting for the quiet hours", "error", err)

		return 0
	}

	started := 0

	for _, inv := range waiting {
		if ctx.Err() != nil {
			break
		}

		// Read again for every one: the window can close while the one
		// before it runs, and then the rest wait for the next opening.
		if !quiethours.OpenNow(b.QuietHours(), time.Now()) {
			break
		}

		if why := b.stillAnswersItself(inv); why != "" {
			b.Log.Info("a held question is left to its user", "invocation", inv.ID, "why", why)

			if err := b.DB.NoLongerAnswersItself(inv.ID); err != nil {
				b.Log.Warn("could not leave a held question to its user", "invocation", inv.ID, "error", err)
			}

			continue
		}

		what := b.whatItDoes(inv)

		done, execErr := b.decide(ctx, inv.ID, true, true)
		if !done.AnsweredItself {
			// Its user answered first: theirs stands.
			continue
		}

		started++

		b.Log.Info("a held question answered itself in the quiet hours", "invocation", inv.ID, "tool", inv.Tool)

		b.sayWhatItDid(done, what, execErr)
	}

	return started
}

/*
 * stillAnswersItself is why a held question no longer answers itself, or
 * empty when it still does: the gate the loop ran when it was asked, run
 * again on what is true now. Every answer here only ever leaves the
 * question to its user; none of them runs anything.
 */
func (b *Brain) stillAnswersItself(inv store.Invocation) string {
	tool, ok := b.Agent.Registry.Get(inv.Tool)
	if !ok {
		return "the tool no longer exists"
	}

	args := json.RawMessage(inv.Arguments)

	q, ok := tool.(tools.QuietHolding)
	if !ok {
		return "its tool is never held for the quiet hours"
	}

	if _, only := q.QuietHold(args); !only {
		return "its tool always asks about this call"
	}

	if why := tools.ConsentFor(tool, args); why != "" {
		return "always its owner's: " + why
	}

	if _, _, held := protect.InArguments(args); held {
		return "it names a protected file"
	}

	changes := tool.Risk() == tools.Mutating
	level := tools.LevelOf(tool, args)

	answer := permits.Ask
	if b.Agent.MayI != nil {
		answer = b.Agent.MayI("", tool.Name(), changes, level)
	} else if !changes {
		answer = permits.Allow
	}

	switch {
	case answer == permits.Refuse:
		return "its user told it never to do this"
	case answer == permits.Allow:
	case b.Cfg.GoalMode && tools.GoalModeAsks(tool, args, func(string) bool { return false }) == "":
		// Goal mode said yes to it, as the loop would have.
	default:
		return "its user's permissions ask about it now"
	}

	act := agent.ActOf(tool, args, tool.Summarize(args), time.Now())
	if len(act.Actions) > 0 {
		if v := rules.Check(b.LiveRules(), act); v.Stop != nil || len(v.Ask) > 0 {
			return "a rule asks about it now"
		}
	}

	return ""
}

// sayWhatItDid puts the answer in the conversation the question was asked
// in, so its user finds it where they left it.
func (b *Brain) sayWhatItDid(inv store.Invocation, what string, execErr error) {
	if inv.ConversationID == 0 {
		return
	}

	text := heldAnswerSaid(b.Cfg.Language, what, inv.Result, execErr)

	if _, err := b.DB.AddMessage(inv.ConversationID, llm.RoleAssistant, "", "", text); err != nil {
		b.Log.Warn("could not say what a held question did", "invocation", inv.ID, "error", err)
	}
}

// whatItDoes is the first line of what its tool says of the call, such as
// "Ask Claude Code (only reads ~/notes and answers)".
func (b *Brain) whatItDoes(inv store.Invocation) string {
	tool, ok := b.Agent.Registry.Get(inv.Tool)
	if !ok {
		return inv.Tool
	}

	line, _, _ := strings.Cut(tool.Summarize(json.RawMessage(inv.Arguments)), "\n")

	return strings.TrimSuffix(strings.TrimSpace(line), ":")
}

// heldAnswerSaid is what the conversation is told, in its user's language:
// what started, and what it said or why it failed.
func heldAnswerSaid(language, what, result string, execErr error) string {
	bg := strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "bg")

	switch {
	case execErr != nil && bg:
		return "Тихите ви часове настъпиха, затова пуснах това, което чакаше, но не стана: " +
			strings.TrimSpace(execErr.Error())
	case execErr != nil:
		return fmt.Sprintf("Your quiet hours opened, so I started what was waiting (%s), and it failed: %s",
			what, strings.TrimSpace(execErr.Error()))
	case bg:
		return "Тихите ви часове настъпиха, затова пуснах това, което чакаше. Отговорът:\n\n" +
			strings.TrimSpace(result)
	}

	return fmt.Sprintf("Your quiet hours opened, so I started what was waiting (%s). It said:\n\n%s",
		what, strings.TrimSpace(result))
}
