package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/tools"
)

// a tool that records whether it was actually run, since the whole question
// here is the difference between not being offered and not being allowed.
type counted struct {
	name string
	ran  *int
}

func (c counted) Name() string                     { return c.name }
func (c counted) Description() string              { return "decide what is waiting" }
func (c counted) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (c counted) Risk() tools.Risk                 { return tools.Safe }
func (c counted) Summarize(json.RawMessage) string { return "decide" }

func (c counted) Execute(context.Context, json.RawMessage) (string, error) {
	*c.ran++

	return "decided", nil
}

func askingFor(name string) *scripted {
	return &scripted{replies: []llm.Response{{
		Content: "Tidying that up.",
		ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: name, Arguments: json.RawMessage(`{}`),
		}},
	}}}
}

/*
 * A turn told it may not use a tool does not use it, even when it asks.
 *
 * Refused rather than merely hidden, and the difference is the whole point:
 * a model that saw the tool in an earlier turn will name it again after the
 * rule changes, and a limit that only shortens the menu is a convention. This
 * is the test that makes it a rule.
 */
func TestATurnCannotUseWhatItWasToldItMayNot(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("decide_waiting"), nil,
		Brief{WithTools: true, Never: []string{"decide_waiting"}})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 {
		t.Fatalf("the tool ran %d times", ran)
	}

	if len(res.Steps) != 0 {
		t.Errorf("a refused call was recorded as a step: %+v", res.Steps)
	}
}

/*
 * And the refusal says whose decision it was, rather than "no".
 *
 * The model has to be able to tell its owner what is waiting instead of
 * silently dropping it, and a bare refusal gives it nothing to say.
 */
func TestTheRefusalSaysWhoseDecisionItIs(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})

	conv, _ := db.NewConversation("t")

	model := askingFor("decide_waiting")

	if _, err := loop.RunBrief(context.Background(), conv, model, nil,
		Brief{WithTools: true, Never: []string{"decide_waiting"}}); err != nil {
		t.Fatal(err)
	}

	said := ""

	for _, req := range model.seen {
		for _, m := range req.Messages {
			if m.Role == llm.RoleTool {
				said = m.Content
			}
		}
	}

	if !strings.Contains(said, "owner's own decision") {
		t.Errorf("the model was told %q", said)
	}
}

/*
 * The same tool with nothing said about it still runs.
 *
 * Which is what makes the test above mean anything: without this, a tool that
 * never ran for some unrelated reason would look like a rule being enforced.
 */
func TestTheSameToolRunsWhenNothingForbidsIt(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, counted{name: "decide_waiting", ran: &ran})

	conv, _ := db.NewConversation("t")

	if _, err := loop.RunBrief(context.Background(), conv, askingFor("decide_waiting"), nil,
		Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Fatalf("the tool ran %d times, want once", ran)
	}
}

// And it is not offered in the first place, which is what stops a small model
// reaching for it at all.
func TestWhatATurnMayNotUseIsNotOffered(t *testing.T) {
	ran := 0

	loop, db := newLoop(t,
		counted{name: "decide_waiting", ran: &ran},
		counted{name: "list_waiting", ran: &ran},
	)

	conv, _ := db.NewConversation("t")

	model := &scripted{replies: []llm.Response{{Content: "nothing to do"}}}

	_, err := loop.RunBrief(context.Background(), conv, model,
		[]llm.Message{{Role: llm.RoleUser, Content: "what is waiting for a decision"}},
		Brief{WithTools: true, Never: []string{"decide_waiting"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(model.seen) == 0 {
		t.Fatal("the model was never asked")
	}

	for _, spec := range model.seen[0].Tools {
		if spec.Name == "decide_waiting" {
			t.Error("a tool the turn may not use was offered to it")
		}
	}
}

/*
 * The gate is told who is asking, and one agent's permission is not another's.
 *
 * End to end, through the loop rather than in the book alone: the whole point
 * of a roster with different authority is that an agent cannot borrow what
 * another was allowed. Checked here because this is the only path that
 * actually runs a tool.
 */
func TestTheGateIsToldWhoIsAsking(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, mutating{counted{name: "send_email", ran: &ran}})

	asked := []string{}

	loop.MayI = func(who, tool string, changes bool) permits.Answer {
		asked = append(asked, who)

		if who == "writer" {
			return permits.Allow
		}

		return permits.Ask
	}

	conv, _ := db.NewConversation("t")

	// The one that may.
	if _, err := loop.RunBrief(context.Background(), conv, askingFor("send_email"), nil,
		Brief{WithTools: true, As: "writer"}); err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Fatalf("the writer's allowed action ran %d times", ran)
	}

	// The one that may not: it stops and waits, rather than borrowing.
	res, err := loop.RunBrief(context.Background(), conv, askingFor("send_email"), nil,
		Brief{WithTools: true, As: "researcher"})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 1 {
		t.Errorf("the researcher borrowed the writer's permission")
	}

	if !res.WaitingForApproval() {
		t.Error("the researcher's action did not stop for approval")
	}

	if len(asked) != 2 || asked[0] != "writer" || asked[1] != "researcher" {
		t.Errorf("the gate was asked about %v", asked)
	}
}

// mutating makes a fake tool one that changes something, so it has to pass the
// gate rather than being waved through as an observation.
type mutating struct{ counted }

func (mutating) Risk() tools.Risk { return tools.Mutating }
