package brain

import (
	"context"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/tools"
)

// A standing instruction is proposed once, with the user's words and where
// it came from, and is not followed until it is kept.
func TestARuleIsProposedOnceAndFollowedOnlyWhenKept(t *testing.T) {
	b := quietBrain(t)

	conv, _ := b.DB.NewConversation("rules")

	said := "Thanks. From now on, never push to main without asking me."

	r := b.noticeRule(said, conv)
	if r == nil || r.Status != rules.Proposed || r.Source != said || r.ConversationID != conv ||
		r.Text != "From now on, never push to main without asking me." || r.Kind != rules.AskFirst {
		t.Fatalf("%+v", r)
	}

	if again := b.noticeRule(said, conv); again != nil {
		t.Errorf("proposed twice: %+v", again)
	}

	if b.noticeRule("Send the report to Anna.", conv) != nil {
		t.Error("a one-off was proposed")
	}

	if strings.Contains(b.personaFor("ollama", false), "push to main") {
		t.Fatal("a proposal went into the prompt")
	}

	if _, err := b.DB.DecideRule(r.ID, true); err != nil {
		t.Fatal(err)
	}

	for _, spoken := range []bool{false, true} {
		if !strings.Contains(b.personaFor("ollama", spoken), r.Text) {
			t.Errorf("the kept rule is not in the prompt (spoken %v)", spoken)
		}
	}

	if !strings.Contains(b.taskPersonaFor("ollama"), r.Text) {
		t.Error("the kept rule is not in a goal step's prompt")
	}
}

// Asked to keep a rule in so many words, the proposal already waiting is the
// one kept, not a second copy.
func TestKeepingARuleByToolKeepsTheProposal(t *testing.T) {
	b := quietBrain(t)

	r := b.noticeRule("Always ask me before you delete anything.", 0)
	if r == nil {
		t.Fatal("not proposed")
	}

	kept, err := b.KeepRule("always ask me before you delete anything", "make that a rule")
	if err != nil || kept.ID != r.ID || kept.Status != rules.Kept || !kept.On {
		t.Fatalf("%+v %v", kept, err)
	}

	again, _ := b.KeepRule("Always ask me before you delete anything.", "")
	if all, _ := b.DB.Rules(true); len(all) != 1 || again.ID != r.ID {
		t.Errorf("kept twice: %v", all)
	}

	fresh, err := b.KeepRule("Never send emails on Sundays.", "the user's words")
	if err != nil || fresh.Kind != rules.Never || len(fresh.Actions) != 1 || fresh.Source != "the user's words" {
		t.Errorf("%+v %v", fresh, err)
	}

	if len(b.RuleTexts()) != 2 {
		t.Errorf("%v", b.RuleTexts())
	}
}

/*
 * A rule kept while an action waited for its yes stops it when the yes
 * comes: approving a question asked before the rule existed is not
 * approving breaking it.
 */
func TestARuleKeptWhileAnActionWaitedStillStopsIt(t *testing.T) {
	b := quietBrain(t)

	conv, _ := b.DB.NewConversation("waiting")

	dir := t.TempDir()
	args := `{"argv":["git","commit","-m","x"],"dir":"` + dir + `"}`

	id, err := b.DB.RecordInvocation(conv, "run_command", args, "git commit -m x", string(tools.Mutating))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := b.KeepRule("Never commit anything.", ""); err != nil {
		t.Fatal(err)
	}

	done, err := b.Decide(context.Background(), id, true)
	if err == nil || !strings.Contains(err.Error(), "Stopped by your rule") || done.Status != "failed" {
		t.Fatalf("%+v %v", done, err)
	}
}
