package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

// notReady is a tool that can tell from the call that it cannot run, like
// ask_claude with no Claude of its user's own.
type notReady struct {
	counted
	why string
}

func (notReady) Risk() tools.Risk                 { return tools.Mutating }
func (n notReady) Name() string                   { return n.counted.name }
func (notReady) Description() string              { return "hand a question over" }
func (notReady) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (notReady) Summarize(json.RawMessage) string { return "Ask Claude" }
func (notReady) AlwaysAsks(json.RawMessage) string {
	return "giving files and a question to Claude"
}
func (n notReady) Unready(json.RawMessage) string { return n.why }
func (n notReady) Execute(ctx context.Context, a json.RawMessage) (string, error) {
	return n.counted.Execute(ctx, a)
}

func unreadyTurn(t *testing.T, why string, setup func(*Loop)) (int, Result, []string) {
	t.Helper()

	ran := 0
	loop, db := newLoop(t, notReady{counted{name: "ask_claude", ran: &ran}, why})

	if setup != nil {
		setup(loop)
	}

	conv, _ := db.NewConversation("t")

	res, err := loop.RunBrief(context.Background(), conv, askingFor("ask_claude"), nil, Brief{WithTools: true})
	if err != nil {
		t.Fatal(err)
	}

	msgs, _ := db.History(conv)

	var told []string

	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			told = append(told, m.Content)
		}
	}

	return ran, res, told
}

// A call its tool says cannot run is said at once, under every setting: no
// question that could only fail after the yes, and nothing run.
func TestACallThatCannotRunIsSaidInsteadOfAsked(t *testing.T) {
	everything := func(l *Loop) {
		l.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
		l.NothingAsks = func() bool { return true }
		l.Unbounded = func() bool { return true }
	}

	for name, setup := range map[string]func(*Loop){"asks": nil, "everything allowed": everything} {
		ran, res, told := unreadyTurn(t, "there is no file /home/you/notes.md", setup)

		if ran != 0 || len(res.Pending) != 0 {
			t.Errorf("%s: ran %d, asked %d", name, ran, len(res.Pending))
		}

		if !strings.Contains(strings.Join(told, "\n"), "there is no file /home/you/notes.md") {
			t.Errorf("%s: the model was not told why: %q", name, told)
		}
	}

	// Ready: asked, as the tool says, and not run before the yes.
	ran, res, _ := unreadyTurn(t, "", everything)
	if ran != 0 || len(res.Pending) != 1 {
		t.Errorf("a ready hand-off: ran %d, asked %d", ran, len(res.Pending))
	}
}
