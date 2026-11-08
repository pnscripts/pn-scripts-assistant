package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/tools"
)

// sender is a tool that sends, as far as the rules can tell from its name,
// and records whether it really ran and what it said it did.
type sender struct {
	ran  *int
	said string
}

func (sender) Name() string                         { return "send_message_now" }
func (sender) Description() string                  { return "send a message" }
func (sender) Parameters() json.RawMessage          { return json.RawMessage(`{"type":"object"}`) }
func (sender) Risk() tools.Risk                     { return tools.Mutating }
func (sender) Summarize(raw json.RawMessage) string { return "Send a message: " + string(raw) }

func (s sender) Execute(context.Context, json.RawMessage) (string, error) {
	*s.ran++

	return s.said, nil
}

func keptRule(t *testing.T, said string) rules.Rule {
	t.Helper()

	p, ok := rules.Notice(said)
	if !ok {
		t.Fatalf("%q was not noticed", said)
	}

	r := p.Rule(1, time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	r.ID, r.Status, r.On = 1, rules.Kept, true

	return r
}

func sending(args string) *scripted {
	return &scripted{replies: []llm.Response{{
		Content:   "Sending it.",
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: "send_message_now", Arguments: json.RawMessage(args)}},
	}}}
}

// Everything allowed and nothing asked: the loosest the gate can be set.
func everythingAllowed(l *Loop) {
	l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	l.Unbounded = func() bool { return true }
	l.NothingAsks = func() bool { return true }
	l.GoalMode = func() bool { return true }
}

/*
 * A "never" stops the call and names the rule, however much else has been
 * allowed: the user's own refusal is the one thing "everything allowed"
 * never lifted.
 */
func TestARuleStopsWhatItForbidsEvenWhenEverythingIsAllowed(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, sender{ran: &ran, said: "sent"})
	everythingAllowed(loop)

	rule := keptRule(t, "Never send messages to anna@example.org.")
	loop.Rules = func() []rules.Rule { return []rules.Rule{rule} }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, sending(`{"to":"anna@example.org"}`), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 {
		t.Fatal("the rule did not stop it")
	}

	if len(res.Steps) != 1 || !res.Steps[0].Failed || !strings.Contains(res.Steps[0].Result, rule.Text) {
		t.Fatalf("the rule was not named: %+v", res.Steps)
	}

	// Somebody else is not the rule's business.
	ran = 0

	if _, err := loop.RunBrief(context.Background(), conv, sending(`{"to":"boris@example.org"}`), nil, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Errorf("a message the rule is not about did not go: ran %d", ran)
	}
}

// "Ask me first" is asked, with the rule beside the question, even on
// "never stop" and in goal mode.
func TestAnAskFirstRuleIsAskedWhateverElseWasAllowed(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, sender{ran: &ran, said: "sent"})
	everythingAllowed(loop)

	ask := keptRule(t, "Always ask me before you send a message.")
	remind := keptRule(t, "Always proofread a message before you send it.")
	remind.ID = 2
	loop.Rules = func() []rules.Rule { return []rules.Rule{ask, remind} }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, sending(`{"to":"anyone"}`), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 || len(res.Pending) != 1 {
		t.Fatalf("not asked: ran %d, pending %v", ran, res.Pending)
	}

	for _, want := range []string{ask.Text, remind.Text} {
		if !strings.Contains(res.Pending[0].Summary, want) {
			t.Errorf("the question does not show %q:\n%s", want, res.Pending[0].Summary)
		}
	}
}

// A proposal nobody has kept, and a rule switched off, do nothing.
func TestOnlyKeptRulesThatAreOnAct(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, sender{ran: &ran, said: "sent"})
	everythingAllowed(loop)

	proposed := keptRule(t, "Never send messages.")
	proposed.Status = rules.Proposed

	off := keptRule(t, "Never send messages.")
	off.On = false

	loop.Rules = func() []rules.Rule { return []rules.Rule{proposed, off} }

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, sending(`{}`), nil, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Errorf("a rule that is not live acted: ran %d", ran)
	}
}

// What a call turned out to do is judged too, and said.
func TestARuleBrokenByWhatACallDidIsSaid(t *testing.T) {
	ran := 0
	loop, db := newLoop(t, sender{ran: &ran, said: "Sent to anna@example.org."})
	everythingAllowed(loop)

	rule := keptRule(t, "Never send messages to anna@example.org.")
	loop.Rules = func() []rules.Rule { return []rules.Rule{rule} }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, sending(`{"to":"the usual list"}`), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 || len(res.Steps) != 1 {
		t.Fatalf("ran %d, steps %+v", ran, res.Steps)
	}

	if !strings.Contains(res.Steps[0].Result, "went against your rule") {
		t.Errorf("not said: %q", res.Steps[0].Result)
	}
}

/*
 * A message the model writes is polished before anybody is asked about it:
 * the question shows the words as they will go, with what was taken out
 * listed above them, and the stored call is the polished one.
 */
func TestAnOutgoingMessageIsPolishedBeforeTheQuestion(t *testing.T) {
	loop, db := newLoop(t, tools.SendEmail{})

	conv, _ := db.NewConversation("t")

	model := &scripted{replies: []llm.Response{{
		Content: "Sending it.",
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: "send_email", Arguments: json.RawMessage(
			`{"to":"boris@example.org","subject":"Invoice","body":"Hi Boris,\n\nI hope this email finds you well. The invoice is attached."}`)}},
	}}}

	res, err := loop.RunBrief(context.Background(), conv, model, nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Pending) != 1 {
		t.Fatalf("not asked: %+v", res)
	}

	q := res.Pending[0].Summary
	if !strings.Contains(q, "Polished before you see it") || !strings.Contains(q, "Hi Boris,\n\nThe invoice is attached.") {
		t.Errorf("the question:\n%s", q)
	}

	inv, _ := db.Invocation(res.Pending[0].ID)
	if inv == nil || strings.Contains(inv.Arguments, "finds you well") {
		t.Errorf("what runs after the yes is not what was shown: %+v", inv)
	}
}
