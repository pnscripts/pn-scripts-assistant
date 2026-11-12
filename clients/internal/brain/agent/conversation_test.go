package agent

import (
	"context"
	"encoding/json"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/tools"
)

// whereAsked records the conversation a call says it came from.
type whereAsked struct{ got *int64 }

func (whereAsked) Name() string        { return "where_asked" }
func (whereAsked) Description() string { return "Says where it was asked." }
func (whereAsked) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (whereAsked) Risk() tools.Risk                 { return tools.Safe }
func (whereAsked) Summarize(json.RawMessage) string { return "Where was I asked" }

func (w whereAsked) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	*w.got = tools.ConversationOf(ctx)

	return "here", nil
}

// A tool run in a conversation is told which, so an answer that comes
// later (a restart being watched for) is said there.
func TestAToolIsToldWhichConversationAskedForIt(t *testing.T) {
	var got int64

	loop, db := newLoop(t, whereAsked{got: &got})
	everythingAllowed(loop)

	conv, _ := db.NewConversation("t")

	model := &scripted{replies: []llm.Response{
		{Content: "Looking.", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "where_asked", Arguments: json.RawMessage(`{}`)}}},
		{Content: "Done."},
	}}

	if _, err := loop.RunBrief(context.Background(), conv, model, nil, Brief{WithTools: true}); err != nil {
		t.Fatal(err)
	}

	if got != conv || conv == 0 {
		t.Fatalf("the tool was told conversation %d, it was %d", got, conv)
	}
}
