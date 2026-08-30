package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// Stripping a leaked tool call can leave nothing, and silence reads as a crash.
func TestEmptyReplyIsExplainedRatherThanShownAsNothing(t *testing.T) {
	loop, db := newLoop(t)

	// Exactly what qwen2.5-coder:7b returns for "What is 2+2?": a tool call
	// printed as prose rather than made as a call.
	model := &scripted{replies: []llm.Response{
		{Content: `{"name": "run_command", "arguments": {"argv": ["echo", "4"]}}`},
	}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if res.Reply == "" {
		t.Fatal("the user would have seen nothing at all")
	}

	if strings.Contains(res.Reply, "run_command") {
		t.Errorf("raw tool-call JSON reached the user: %q", res.Reply)
	}
}

// The real observed failure: qwen2.5-coder:7b decides correctly to list a
// directory and then prints the call instead of making it.
func TestToolCallWrittenAsTextIsRecovered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "found.txt"), []byte("x"), 0o644)

	loop, db := newLoop(t, tools.ListDirectory{})

	model := &scripted{replies: []llm.Response{
		{Content: `{"name": "list_directory", "arguments": {"path": ` + quote(dir) + `}}`},
		{Content: "There is one file, found.txt."},
	}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if res.Reply != "There is one file, found.txt." {
		t.Fatalf("reply was %q", res.Reply)
	}

	if len(res.ActionsTaken) != 1 {
		t.Fatalf("the recovered call did not run: %v", res.ActionsTaken)
	}

	last := model.seen[len(model.seen)-1]
	fed := false

	for _, m := range last.Messages {
		if m.Role == llm.RoleTool && strings.Contains(m.Content, "found.txt") {
			fed = true
		}
	}

	if !fed {
		t.Error("the recovered call's result was not fed back")
	}
}

// A recovered Mutating call is still a Mutating call. Recovery must not become
// a way around the approval gate.
func TestRecoveredMutatingCallStillStopsForApproval(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nope.txt")

	loop, db := newLoop(t, tools.WriteFile{})

	model := &scripted{replies: []llm.Response{
		{Content: `{"name":"write_file","arguments":{"path":` + quote(target) + `,"content":"x"}}`},
	}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a tool call recovered from text bypassed approval")
	}

	if !res.WaitingForApproval() {
		t.Fatal("the recovered call did not stop for approval")
	}
}

// Recovery must be strict, or prose about tools and JSON quoted from a file
// start being treated as instructions to act.
func TestRecoveryIsStrict(t *testing.T) {
	loop, _ := newLoop(t, tools.ReadFile{}, tools.ListDirectory{})

	reject := []string{
		"You could use read_file for that.",
		`Try {"name": "read_file"} to see it.`,
		`{"name": "make_coffee", "arguments": {}}`,
		`{"result": 4}`,
		`{"tool": "read_file"}`,
		"",
		"{not json}",
	}

	for _, in := range reject {
		if _, ok := loop.recoverToolCall(in); ok {
			t.Errorf("recovered a call from %q", in)
		}
	}

	// A fenced call is still a call; models fence it despite being asked not to.
	if _, ok := loop.recoverToolCall("```json\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"/tmp/x\"}}\n```"); !ok {
		t.Error("did not recover a fenced tool call")
	}

	// "parameters" appears in the wild alongside "arguments".
	call, ok := loop.recoverToolCall(`{"name":"read_file","parameters":{"path":"/tmp/x"}}`)
	if !ok || !strings.Contains(string(call.Arguments), "/tmp/x") {
		t.Errorf("did not recover a call using \"parameters\": %+v", call)
	}
}

// Schemas are written indented for people to read, and every tab and newline is
// a token charged on every call. Compacting changes nothing the model sees.
func TestToolSchemasAreSentCompact(t *testing.T) {
	loop, _ := newLoop(t, tools.ReadFile{}, tools.WriteFile{}, tools.RunCommand{}, tools.ListDirectory{})

	var raw, sent int

	for _, spec := range loop.specs() {
		tool, _ := loop.Registry.Get(spec.Name)
		raw += len(tool.Parameters())
		sent += len(spec.Parameters)

		// Compacting must not damage the schema.
		var schema map[string]any

		if err := json.Unmarshal(spec.Parameters, &schema); err != nil {
			t.Errorf("%s schema is no longer valid JSON: %v", spec.Name, err)
		}

		if schema["type"] != "object" {
			t.Errorf("%s lost its type", spec.Name)
		}

		if bytes.ContainsAny(spec.Parameters, "\n\t") {
			t.Errorf("%s schema still carries formatting whitespace", spec.Name)
		}
	}

	if sent >= raw {
		t.Errorf("compacting saved nothing: %d bytes sent against %d written", sent, raw)
	}

	t.Logf("schemas: %d bytes written, %d sent (%d%% smaller)", raw, sent, 100-sent*100/raw)
}

/*
 * A tool call introduced by a sentence.
 *
 * This is what small models actually do, and it cost a whole evening. Asked to
 * approve everything waiting, qwen2.5-coder:7b produced exactly the right call
 * with exactly the right arguments — and put "Sure, I'll approve everything for
 * you." in front of it. Recovery required the whole reply to be one JSON
 * object, so the call was dropped, and the brain told its owner it had done
 * something it had not done.
 */
func TestAToolCallAfterASentence(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	replies := []string{
		"Sure, I'll do that for you.\n\n```json\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"/tmp/a\"}}\n```",
		"```json\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"/tmp/a\"}}\n```",
		"Let me look.\n```\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"/tmp/a\"}}\n```\nThat should do it.",
		"{\"name\": \"read_file\", \"arguments\": {\"path\": \"/tmp/a\"}}",
	}

	for _, reply := range replies {
		call, ok := loop.recoverToolCall(reply)
		if !ok {
			t.Errorf("dropped the call in %q", reply)

			continue
		}

		if call.Name != "read_file" {
			t.Errorf("recovered %q from %q", call.Name, reply)
		}

		if !strings.Contains(string(call.Arguments), "/tmp/a") {
			t.Errorf("lost the arguments from %q: %s", reply, call.Arguments)
		}
	}
}

// But talking about a tool is still not calling one.
//
// The looser reading was tried — every pair of braces anywhere in the reply —
// and it turns an explanation into an action, which is how an assistant starts
// doing things nobody asked for.
func TestTalkingAboutAToolIsStillNotCallingOne(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	for _, reply := range []string{
		`Try {"name": "read_file"} to see it.`,
		`The format is {"name": "read_file", "arguments": {"path": "..."}} in prose.`,
		`I could use read_file for that. Shall I?`,
	} {
		if _, ok := loop.recoverToolCall(reply); ok {
			t.Errorf("acted on an explanation: %q", reply)
		}
	}
}

/*
 * The plumbing does not belong on screen, or in the voice.
 *
 * A model that announces the call and then prints it leaves its owner reading
 * "Understood." followed by a wall of braces — and the voice reads the braces
 * out. The sentence is the answer; the JSON escaped.
 */
func TestTheCallIsNotShownToThePerson(t *testing.T) {
	cases := []struct {
		reply string
		want  string
	}{
		{"Understood.\n\n{\n  \"name\": \"decide_waiting\",\n  \"arguments\": {}\n}", "Understood."},
		{"Sure, I'll do that.\n\n```json\n{\"name\": \"read_file\"}\n```", "Sure, I'll do that."},
		{"{\"name\": \"read_file\"}", ""},
		{"Here is what I found: the file has three lines.", "Here is what I found: the file has three lines."},
	}

	for _, c := range cases {
		if got := presentable(c.reply); got != c.want {
			t.Errorf("presentable(%q) = %q, want %q", c.reply, got, c.want)
		}
	}
}

/*
 * A streamed turn must not be spoken until it is clear it is an answer.
 *
 * A model about to call a tool opens with a brace or a fence. Half a tool call
 * read out loud is a string of punctuation, which is the one outcome worse than
 * saying nothing.
 */
func TestWaitingToSeeWhetherItIsAnAnswer(t *testing.T) {
	cases := []struct {
		opening string
		prose   bool
		settled bool
	}{
		{`{"name": "read_file"`, false, true},
		{"```json", false, true},
		{"[", false, true},
		{"Good morning, Petar", true, true},
		{"I have checked and", true, true},

		// Not yet enough to tell.
		{"Good", false, false},
		{"", false, false},
		{"   ", false, false},
	}

	for _, c := range cases {
		prose, settled := isProse(c.opening)

		if settled != c.settled {
			t.Errorf("%q: settled=%v, want %v", c.opening, settled, c.settled)

			continue
		}

		if settled && prose != c.prose {
			t.Errorf("%q: prose=%v, want %v", c.opening, prose, c.prose)
		}
	}
}

/*
 * The tools go with a streamed turn.
 *
 * The first version of streaming left them out, which would have been a quiet
 * disaster: every turn that meant doing something would have streamed
 * beautifully and been unable to do any of it.
 */
func TestAStreamedTurnStillCarriesItsTools(t *testing.T) {
	var sent struct {
		Tools  []map[string]any `json:"tools"`
		Stream bool             `json:"stream"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = w.Write([]byte(`{"message":{"content":"Hello."},"done":true}` + "\n"))
	}))

	defer server.Close()

	client := llm.NewOllama(server.URL, "qwen2.5-coder:7b", "nomic-embed-text")

	_, err := client.ChatStream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		Tools:    []llm.ToolSpec{{Name: "read_file", Description: "read a file"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !sent.Stream {
		t.Error("the request did not ask for a stream")
	}

	if len(sent.Tools) != 1 {
		t.Fatalf("the tools were dropped from the streamed request: %+v", sent.Tools)
	}
}
