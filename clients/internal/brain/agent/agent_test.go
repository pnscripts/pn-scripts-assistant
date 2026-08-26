package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
)

func newLoop(t *testing.T, ts ...tools.Tool) (*Loop, *store.DB) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return &Loop{
		DB:       db,
		Registry: tools.NewRegistry(ts...),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, db
}

// scripted returns queued responses in order, so a whole loop can be driven
// without a model.
type scripted struct {
	replies []llm.Response
	seen    []llm.Request
}

func (s *scripted) Name() string                   { return "scripted" }
func (s *scripted) Available(context.Context) bool { return true }

func (s *scripted) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	s.seen = append(s.seen, req)

	if len(s.replies) == 0 {
		return llm.Response{Content: "done"}, nil
	}

	r := s.replies[0]
	s.replies = s.replies[1:]

	return r, nil
}

// The property the whole permission system rests on: a Mutating tool is never
// executed by the loop, whatever the model asks for.
func TestMutatingToolIsNeverRunWithoutApproval(t *testing.T) {
	target := filepath.Join(t.TempDir(), "should-not-exist.txt")

	loop, db := newLoop(t, tools.WriteFile{})

	model := &scripted{replies: []llm.Response{{
		Content: "I'll write that now.",
		ToolCalls: []llm.ToolCall{{
			ID:        "c1",
			Name:      "write_file",
			Arguments: json.RawMessage(`{"path":` + quote(target) + `,"content":"hello"}`),
		}},
	}}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the loop wrote a file without approval")
	}

	if !res.WaitingForApproval() {
		t.Fatal("the turn did not stop for approval")
	}

	if len(res.Pending) != 1 || res.Pending[0].Tool != "write_file" {
		t.Fatalf("pending is %+v", res.Pending)
	}

	// The action must be recorded, or approving it later is impossible.
	waiting, err := db.PendingInvocations()
	if err != nil {
		t.Fatal(err)
	}

	if len(waiting) != 1 {
		t.Fatalf("%d invocations recorded, want 1", len(waiting))
	}

	if !strings.Contains(waiting[0].Summary, target) {
		t.Errorf("summary does not name the file: %q", waiting[0].Summary)
	}
}

// Safe tools run without asking; stopping for every directory listing would
// train the user to click yes.
func TestSafeToolRunsWithoutApproval(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)

	loop, db := newLoop(t, tools.ListDirectory{})

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "list_directory",
			Arguments: json.RawMessage(`{"path":` + quote(dir) + `}`),
		}}},
		{Content: "There is one file, a.txt."},
	}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if res.WaitingForApproval() {
		t.Fatal("a safe tool asked for approval")
	}

	if len(res.ActionsTaken) != 1 {
		t.Fatalf("actions taken: %v", res.ActionsTaken)
	}

	// The result must be fed back, or the model is answering from nothing.
	last := model.seen[len(model.seen)-1]
	found := false

	for _, m := range last.Messages {
		if m.Role == llm.RoleTool && strings.Contains(m.Content, "a.txt") {
			found = true
		}
	}

	if !found {
		t.Error("the tool result was not fed back to the model")
	}
}

// A model that keeps calling tools must not loop forever; small local models do
// this readily.
func TestStepLimitStopsRunawayLoops(t *testing.T) {
	dir := t.TempDir()

	loop, db := newLoop(t, tools.ListDirectory{})

	replies := make([]llm.Response, 40)
	for i := range replies {
		replies[i] = llm.Response{ToolCalls: []llm.ToolCall{{
			ID: "c", Name: "list_directory",
			Arguments: json.RawMessage(`{"path":` + quote(dir) + `}`),
		}}}
	}

	model := &scripted{replies: replies}
	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !res.HitStepLimit {
		t.Fatal("the loop did not stop at the step limit")
	}

	if len(model.seen) != MaxSteps {
		t.Errorf("model was called %d times, want %d", len(model.seen), MaxSteps)
	}
}

// Hallucinated tool names are common. Telling the model lets it correct itself;
// failing the turn would lose the conversation.
func TestUnknownToolIsReportedToTheModel(t *testing.T) {
	loop, db := newLoop(t, tools.ReadFile{})

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "make_coffee", Arguments: json.RawMessage(`{}`)}}},
		{Content: "Sorry, I cannot do that."},
	}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if res.Reply != "Sorry, I cannot do that." {
		t.Errorf("reply was %q", res.Reply)
	}

	last := model.seen[len(model.seen)-1]
	told := false

	for _, m := range last.Messages {
		if strings.Contains(m.Content, "No such tool: make_coffee") {
			told = true
		}
	}

	if !told {
		t.Error("the model was not told the tool does not exist")
	}
}

// A model that has already made a structured call often prints the call as
// prose too. Showing that to a person is showing them the machinery.
func TestRawToolCallJSONIsNotShownAsAReply(t *testing.T) {
	cases := map[string]string{
		`{"name": "read_file", "arguments": {"path": "/tmp/x"}}`: "",
		`  {"name":"add"}  `:  "",
		"Here is the answer.": "Here is the answer.",
		`{"result": 4}`:       `{"result": 4}`,
		"":                    "",
	}

	for in, want := range cases {
		if got := presentable(in); got != want {
			t.Errorf("presentable(%q) = %q, want %q", in, got, want)
		}
	}
}

// An action already decided must not be decidable again, or a replayed request
// could turn a denial into an approval.
func TestDecisionsAreFinal(t *testing.T) {
	_, db := newLoop(t)

	id, err := db.RecordInvocation(0, "write_file", `{"path":"/tmp/a"}`, "Create /tmp/a", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.DecideInvocation(id, store.InvocationDenied)
	if err != nil || !ok {
		t.Fatalf("first decision failed: ok=%v err=%v", ok, err)
	}

	ok, err = db.DecideInvocation(id, store.InvocationApproved)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Fatal("a denied action was re-decided as approved")
	}

	inv, _ := db.Invocation(id)
	if inv.Status != store.InvocationDenied {
		t.Errorf("status is %q, want denied", inv.Status)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}
