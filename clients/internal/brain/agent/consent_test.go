package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// installing is a tool that always wants its owner's yes.
type installing struct{ counted }

func (installing) Risk() tools.Risk                 { return tools.Mutating }
func (installing) Consent(json.RawMessage) string   { return "installing software" }
func (installing) Summarize(json.RawMessage) string { return "Install Godot 4.7.2" }
func (i installing) Name() string                   { return i.counted.name }
func (i installing) Description() string            { return "install" }
func (installing) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (i installing) Execute(ctx context.Context, a json.RawMessage) (string, error) {
	return i.counted.Execute(ctx, a)
}

/*
 * "Never stop" and a standing yes both allow it, and it still asks.
 *
 * The one place a standing decision is not enough: installing, switching an
 * integration on, spending, hiring for good. The question names why.
 */
func TestConsentAsksWhateverTheSetting(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installing{counted{name: "install_requirement", ran: &ran}})

	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	loop.NothingAsks = func() bool { return true }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"), nil,
		Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 {
		t.Fatal("it installed without asking")
	}

	if len(res.Pending) != 1 || !strings.Contains(res.Pending[0].Summary, "installing software") {
		t.Fatalf("it did not ask, or did not say why: %+v", res.Pending)
	}
}

// And a refusal still wins: asking is not a way round "never".
func TestConsentDoesNotOverruleARefusal(t *testing.T) {
	ran := 0

	loop, db := newLoop(t, installing{counted{name: "install_requirement", ran: &ran}})

	loop.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Refuse }

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("install_requirement"), nil,
		Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	if ran != 0 || len(res.Pending) != 0 {
		t.Errorf("a refused install ran (%d) or was put to the owner (%d)", ran, len(res.Pending))
	}
}
