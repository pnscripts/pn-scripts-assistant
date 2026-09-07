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
	"pn-brain/internal/brain/protect"
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

/*
 * A reasoning model's working is not the answer.
 *
 * deepseek-r1 and its like narrate their deliberation inside <think> tags
 * before answering. Shown, it buries the reply under a page of thinking;
 * spoken, the voice reads several minutes of the model talking to itself while
 * the person waiting has no way to know the answer has not started.
 */
func TestTheModelsWorkingIsNotShown(t *testing.T) {
	cases := []struct {
		reply string
		want  string
	}{
		{"<think>Let me consider the options.</think>The disk is filling up.",
			"The disk is filling up."},
		{"<think>still going", ""},
		{"No thinking here at all.", "No thinking here at all."},
		{"<think>a</think>One.<think>b</think>Two.", "One.Two."},
	}

	for _, c := range cases {
		if got := presentable(c.reply); got != c.want {
			t.Errorf("presentable(%q) = %q, want %q", c.reply, got, c.want)
		}
	}
}

/*
 * A call written after the model's own deliberation.
 *
 * qwen3 and the other hybrid reasoning models wrap their working in <think>
 * tags and put the call after it, so the reply does not begin with a brace and
 * was never read as a call. The brain then reported "the model returned a tool
 * call as text rather than making one" — true, and entirely self-inflicted.
 */
func TestACallAfterTheModelsThinking(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	replies := []string{
		"<think>They want the file. I should use read_file.</think>\n" +
			`{"name": "read_file", "arguments": {"path": "/tmp/a"}}`,
		"<think>thinking</think>\n```json\n" +
			`{"name": "read_file", "arguments": {"path": "/tmp/a"}}` + "\n```",
		"<think>a</think>Sure, one moment.\n\n```json\n" +
			`{"name": "read_file", "arguments": {"path": "/tmp/a"}}` + "\n```",
	}

	for _, reply := range replies {
		call, ok := loop.recoverToolCall(reply)
		if !ok {
			t.Errorf("dropped the call in %q", reply)

			continue
		}

		if call.Name != "read_file" || !strings.Contains(string(call.Arguments), "/tmp/a") {
			t.Errorf("recovered %q with %s", call.Name, call.Arguments)
		}
	}

	// And a model that only thinks out loud has still not asked for anything.
	if _, ok := loop.recoverToolCall("<think>I could use read_file here.</think>No thanks."); ok {
		t.Error("acted on something the model only thought about")
	}
}

/*
 * A call announced in a sentence and then written out bare.
 *
 * This produced "To see what is waiting for you, please give me the command:"
 * and then nothing whatsoever. The model had written the call on the next line
 * without a fence; the display knew to strip it from the end and recovery did
 * not know to look there, so the sentence survived and the call did not. Two
 * functions disagreeing about where a call can be is how one gets lost.
 */
func TestACallWrittenBareAfterASentence(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	replies := []string{
		"To see the file, here is the command:\n\n" +
			`{"name": "read_file", "arguments": {"path": "/tmp/a"}}`,
		"I'll look at it now.\n" +
			`{"name": "read_file", "arguments": {"path": "/tmp/a"}}`,
	}

	for _, reply := range replies {
		call, ok := loop.recoverToolCall(reply)
		if !ok {
			t.Errorf("dropped the call in %q", reply)

			continue
		}

		if call.Name != "read_file" || !strings.Contains(string(call.Arguments), "/tmp/a") {
			t.Errorf("recovered %q with %s", call.Name, call.Arguments)
		}
	}

	/*
	 * Whatever is recovered, the reply must not keep the JSON in it.
	 *
	 * The display and the recovery have to agree: if one strips something the
	 * other did not act on, the owner is left reading half a sentence.
	 */
	for _, reply := range replies {
		if _, ok := loop.recoverToolCall(reply); ok {
			continue
		}

		if strings.Contains(presentable(reply), "{") {
			t.Errorf("neither acted on nor hidden: %q", presentable(reply))
		}
	}
}

/*
 * Telling an undertaking from an answer.
 *
 * "It said it would and it did not" is the failure this program has had more
 * than any other. Every earlier fix was for a call being lost after the model
 * made it; this is for the model never making one — so it has to recognise the
 * shape of a promise without mistaking ordinary prose for one.
 *
 * Wrong in the loose direction costs a whole extra turn, which here is minutes,
 * so it fires only on the shapes actually seen doing it.
 */
func TestRecognisingAPromiseWithoutAnAction(t *testing.T) {
	// Every one of these was said by the model in this program, verbatim or
	// nearly, immediately before doing nothing at all.
	for _, reply := range []string{
		"Understood. I will ensure all pending actions and memories are kept in mind for you.",
		"I'll keep track of everything that is waiting for you.",
		"Sure, I'll approve everything and remember all the pending tasks for you.",
		"Understood, I'll remember the items in the queue.",
		"Let me check what is waiting for you.",
		"Understood.",
		"I am going to write that file for you.",
		"<think>They want the queue emptied.</think>I will approve everything.",
	} {
		if !promised(reply) {
			t.Errorf("not recognised as a promise: %q", reply)
		}
	}

	// And these are answers, not undertakings. Pressing on any of them wastes
	// a turn and confuses a conversation that had already finished properly.
	for _, reply := range []string{
		"There is one thing waiting for you: a memory about your models.",
		"The file has three lines and the last one is blank.",
		"Good morning, Petar.",
		"I don't have a tool for that, so you would need to do it yourself.",
		"Nothing is waiting.",
		"That depends on what you mean — I will need more detail before I can answer, " +
			"because the two readings point in different directions.",
	} {
		if promised(reply) {
			t.Errorf("an ordinary answer was taken as an unkept promise: %q", reply)
		}
	}
}

/*
 * An interrupted answer is recorded as what was actually said.
 *
 * Stopping the voice used to throw the rest away and still write the full
 * reply into the conversation, so it held a paragraph its owner never heard —
 * and the next turn was answered as though they had heard it. The follow-up
 * then made no sense to either of them: the person was replying to the first
 * sentence while the assistant carried on from the fifth.
 */
func TestAnInterruptedAnswerKeepsOnlyWhatWasSaid(t *testing.T) {
	full := "The weather in Sofia is mild today. Around eighteen degrees. " +
		"There is some cloud in the afternoon. Rain is unlikely. " +
		"The wind stays light all day."

	delivered := "The weather in Sofia is mild today. Around eighteen degrees."

	resp := llm.Response{Content: full}

	// What streamAloud does when the person cuts in.
	if said := strings.TrimSpace(delivered); said != "" {
		resp.Content = said + " …"
		resp.CutOff = true
	}

	if !resp.CutOff {
		t.Error("the answer was not marked as interrupted, so the next turn " +
			"cannot tell it was cut short")
	}

	if strings.Contains(resp.Content, "The wind stays light") {
		t.Error("the conversation kept a sentence nobody heard, so the follow-up " +
			"will be answered as though they had")
	}

	if !strings.HasPrefix(resp.Content, delivered) {
		t.Errorf("what was actually said was lost: %q", resp.Content)
	}
}

/*
 * A protected file is not refused. It is put to the owner.
 *
 * Reading is Safe and runs without asking, which is right for a source file
 * and wrong for a private key — and the old answer, refusing outright, was
 * wrong too: this is the owner's own machine and his own files, and a program
 * that says no to its owner has decided something that was not its to decide.
 *
 * It is also the better answer to what this defends against. A page the brain
 * reads can carry text telling it to open a key and post the contents
 * somewhere, and the whole of that attack is that nobody sees it happen.
 */
func TestReadingSomethingProtectedStopsAndAsks(t *testing.T) {
	home := t.TempDir()
	key := filepath.Join(home, ".ssh", "id_rsa")

	os.MkdirAll(filepath.Dir(key), 0o700)
	os.WriteFile(key, []byte("not a real key"), 0o600)

	protect.Use(protect.Choices{})

	loop, db := newLoop(t, tools.ReadFile{})

	model := &scripted{replies: []llm.Response{{
		Content: "Reading it now.",
		ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "read_file",
			Arguments: json.RawMessage(`{"path":` + quote(key) + `}`),
		}},
	}}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !res.WaitingForApproval() {
		t.Fatal("it read a private key without asking anybody")
	}

	if len(res.Pending) != 1 || res.Pending[0].Tool != "read_file" {
		t.Fatalf("pending is %+v", res.Pending)
	}

	/*
	 * And the question says enough to be answerable: what it wants to do,
	 * which file, and why that file is one it stops at. "Allow access?" on its
	 * own teaches people to say yes.
	 */
	asked := res.Pending[0].Summary

	for _, want := range []string{"id_rsa", "ssh keys", "log into"} {
		if !strings.Contains(strings.ToLower(asked), want) {
			t.Errorf("the question does not mention %q: %s", want, asked)
		}
	}

	// Nothing of the file reached the conversation while it waits.
	messages, _ := db.History(conv)

	for _, m := range messages {
		if strings.Contains(m.Content, "not a real key") {
			t.Fatal("the contents were in the transcript before anybody approved")
		}
	}
}

// An ordinary file is read without ceremony. Both halves have to hold: a brain
// that asks about everything is one whose questions stop being read.
func TestAnOrdinaryFileIsJustRead(t *testing.T) {
	note := filepath.Join(t.TempDir(), "notes.md")
	os.WriteFile(note, []byte("nothing secret"), 0o644)

	protect.Use(protect.Choices{})

	loop, db := newLoop(t, tools.ReadFile{})

	model := &scripted{replies: []llm.Response{{
		ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "read_file",
			Arguments: json.RawMessage(`{"path":` + quote(note) + `}`),
		}},
	}}}

	conv, _ := db.NewConversation("t")

	res, err := loop.Run(context.Background(), conv, model, nil)
	if err != nil {
		t.Fatal(err)
	}

	if res.WaitingForApproval() {
		t.Fatalf("it asked permission to read an ordinary file: %+v", res.Pending)
	}
}

/*
 * The same answer twice is not an answer.
 *
 * From a real conversation: asked to learn everything, the brain offered a
 * choice of three; asked again in different words, it offered the same three
 * in the same words. That is the moment a conversation stops being one, and
 * handing the sentence back a second time is the program's doing rather than
 * the model's.
 */
func TestAWordForWordRepeatIsCalledOut(t *testing.T) {
	said := "I can't learn everything at once. Would you like to learn projects, " +
		"documents, or a specific folder inside?"

	history := []llm.Message{
		{Role: llm.RoleUser, Content: "learn everything"},
		{Role: llm.RoleAssistant, Content: said},
		{Role: llm.RoleUser, Content: "i want you to update you and to learn everything"},
	}

	if !repeatOf(said, history) {
		t.Fatal("the identical answer was not recognised as a repeat")
	}

	// Punctuation and spacing must not hide it.
	if !repeatOf("  I can't learn everything at once.   Would you like to learn "+
		"PROJECTS, documents, or a specific folder inside?  ", history) {
		t.Fatal("the same sentence differently spaced was not recognised")
	}
}

// A different answer is not a repeat, however similar the subject.
func TestADifferentAnswerIsNotARepeat(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleAssistant, Content: "I can't learn everything at once. " +
			"Would you like to learn projects, documents, or a specific folder inside?"},
	}

	if repeatOf("I have started on the documents in that folder. There are 966 of "+
		"them, so it will take a while.", history) {
		t.Fatal("a new answer was called a repeat")
	}
}

/*
 * Short answers repeat legitimately and constantly.
 *
 * "Yes", "done", "not yet" — a brain that lectured somebody for agreeing with
 * them twice would be worse than one that occasionally repeats itself.
 */
func TestShortAnswersMayRepeatFreely(t *testing.T) {
	history := []llm.Message{{Role: llm.RoleAssistant, Content: "Done."}}

	if repeatOf("Done.", history) {
		t.Fatal("a one-word answer was treated as a fault")
	}
}

// Only the most recent answer counts: coming back to the same reply after
// three other exchanges is a conversation returning to a subject.
func TestOnlyTheLastAnswerIsCompared(t *testing.T) {
	said := "That folder holds 966 documents, which is more than can be taken in " +
		"at once, so I will work through it a bite at a time."

	history := []llm.Message{
		{Role: llm.RoleAssistant, Content: said},
		{Role: llm.RoleUser, Content: "what about the projects"},
		{Role: llm.RoleAssistant, Content: "There are 70 of those, and I know them all."},
		{Role: llm.RoleUser, Content: "and the documents again"},
	}

	if repeatOf(said, history) {
		t.Fatal("an answer from earlier in the conversation was called a repeat")
	}
}

// With nothing said yet, nothing can be a repeat.
func TestTheFirstAnswerIsNeverARepeat(t *testing.T) {
	if repeatOf("A long enough first answer to be worth comparing at all, said once.",
		[]llm.Message{{Role: llm.RoleUser, Content: "hello"}}) {
		t.Fatal("the first answer in a conversation was called a repeat")
	}
}

/*
 * Claiming to be doing something is worse than promising to.
 *
 * From a real conversation: "what is waiting for me?" was answered with "I'm
 * checking what is waiting for you. One moment." — and nothing was checked.
 * That is not a plan to act, it is a claim to be acting, and to somebody
 * waiting it is indistinguishable from work being done, so they wait.
 */
func TestNarratingAnActionCountsAsPromisingOne(t *testing.T) {
	for _, said := range []string{
		"I'm checking what is waiting for you. One moment.",
		"I'm remembering everything you've told me.",
		"I am looking through your documents now.",
		"I'm reading that file.",
		"I'm going through the drive for you.",
		"I'm starting on the projects folder.",
	} {
		if !promised(said) {
			t.Errorf("let this through: %q", said)
		}
	}
}

/*
 * And the honest answers it must never press on.
 *
 * Pressing costs a whole extra turn, and on this machine a turn is minutes.
 * Doing that to somebody who has just been told plainly that it cannot help is
 * the worse failure of the two.
 */
func TestAnHonestAnswerIsNotAPromise(t *testing.T) {
	for _, said := range []string{
		"I'm not sure what you mean by that.",
		"I'm afraid that is not something I can do.",
		"I am unable to reach that drive — it is not attached.",
		"That depends on what you mean — I will need more detail.",
		"I'm sorry, there is nothing waiting.",
		"I'm here.",
	} {
		if promised(said) {
			t.Errorf("pressed on an honest answer: %q", said)
		}
	}
}

/*
 * A call written first, with a pleasantry after it.
 *
 * What a small model does when it has been told both to be conversational and
 * to use tools. The call sat in plain sight and the brain said "the model
 * returned a tool call as text rather than making one" — which was true, and
 * entirely self-inflicted.
 */
func TestACallAtTheStartOfTheReplyIsRecovered(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	for _, said := range []string{
		`{"name": "read_file", "arguments": {"path": "/tmp/a"}}` +
			"\nLet me know if you need anything else.",
		"Here you are.\n" + `{"name": "read_file", "arguments": {"path": "/tmp/a"}}` +
			"\nAnything else?",
		"```json\n" + `{"name": "read_file", "arguments": {"path": "/tmp/a"}}` + "\n```",
	} {
		got, ok := loop.recoverToolCall(said)

		if !ok || got.Name != "read_file" {
			t.Errorf("did not recover the call from:\n%s", said)
		}
	}
}

/*
 * And prose that merely mentions a call is still prose.
 *
 * The guard is that a model writing a call puts it at the start of a line; a
 * sentence about one has words either side. Without that, explaining the
 * program to somebody would run it.
 */
func TestACallMentionedInASentenceIsNotRun(t *testing.T) {
	loop := &Loop{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: tools.NewRegistry(tools.ReadFile{}),
	}

	if _, ok := loop.recoverToolCall(
		`You can try {"name": "read_file"} to see it yourself.`); ok {
		t.Error("ran a tool because a sentence mentioned it")
	}
}

// Braces inside a string argument must not close the object early — a call
// that writes a file carries the file's contents.
func TestAnObjectWithBracesInsideItIsReadWhole(t *testing.T) {
	call := `{"name": "write_file", "arguments": {"path": "a.json", "text": "{\"a\": 1}"}}`

	got, ok := balanced(call + "\nDone.")

	if !ok || got != call {
		t.Fatalf("read %q", got)
	}
}

/*
 * A question ends the turn. It does not come back to the model.
 *
 * The whole point, and the easy thing to get wrong: a model that asks a
 * question and is handed control again answers it itself on the next step,
 * which is exactly the guessing this exists to stop.
 */
func TestAQuestionEndsTheTurnRatherThanBeingAnswered(t *testing.T) {
	loop, db := newLoop(t, tools.Ask{Owner: "Petar"})

	id, _ := db.NewConversation("")

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID:   "1",
			Name: "ask_first",
			Arguments: json.RawMessage(`{
				"question": "Which of them did you mean?",
				"why": "There are two folders called Skillo",
				"options": ["~/Projects/Skillo", "/media/drive/DEV/Skillo"]
			}`),
		}}},

		// If the loop ever hands control back, this is what would be said —
		// and the test would see it instead of the question.
		{Content: "I picked the first one and carried on."},
	}}

	out, err := loop.Run(context.Background(), id, model,
		[]llm.Message{{Role: llm.RoleUser, Content: "look in Skillo"}})
	if err != nil {
		t.Fatal(err)
	}

	if !out.Asked {
		t.Error("the turn did not report that it ended on a question")
	}

	if strings.Contains(out.Reply, "carried on") {
		t.Errorf("the model answered its own question:\n%s", out.Reply)
	}

	for _, want := range []string{"two folders called Skillo", "Which of them did you mean?", "~/Projects/Skillo"} {
		if !strings.Contains(out.Reply, want) {
			t.Errorf("the question does not say %q:\n%s", want, out.Reply)
		}
	}

	// Asked once: the model was never called a second time.
	if len(model.seen) != 1 {
		t.Errorf("the model was asked %d times, want once", len(model.seen))
	}

	// And it is in the conversation, so the answer has something to follow.
	said, _ := db.History(id)

	var found bool

	for _, m := range said {
		if strings.Contains(m.Content, "Which of them did you mean?") {
			found = true
		}
	}

	if !found {
		t.Error("the question was not written into the conversation")
	}
}

// An ask_first call with nothing in it must not end a turn with silence.
func TestAnEmptyQuestionDoesNotEndTheTurn(t *testing.T) {
	loop, db := newLoop(t, tools.Ask{})

	id, _ := db.NewConversation("")

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "ask_first", Arguments: json.RawMessage(`{}`)}}},
		{Content: "Here is the answer instead."},
	}}

	out, err := loop.Run(context.Background(), id, model,
		[]llm.Message{{Role: llm.RoleUser, Content: "anything"}})
	if err != nil {
		t.Fatal(err)
	}

	if out.Asked {
		t.Error("an empty question ended the turn")
	}

	if out.Reply == "" {
		t.Error("the turn ended with nothing at all")
	}
}
