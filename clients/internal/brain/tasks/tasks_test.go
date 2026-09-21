package tasks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/activity"
	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/jobs"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/protocol"
)

// scripted answers with queued replies, so a whole task can be driven without
// a model.
type scripted struct {
	replies []llm.Response
	asked   []llm.Request

	// plan is what the planning call is answered with. Empty means one step,
	// which is what most of these tests want.
	plan string

	// checks are what the checking calls are answered with, in order. Empty
	// means the check agrees and quotes the whole of what the tools returned.
	checks []string
}

func (s *scripted) Name() string                   { return "scripted" }
func (s *scripted) Available(context.Context) bool { return true }

func (s *scripted) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	s.asked = append(s.asked, req)

	/*
	 * Planning is answered separately from the work.
	 *
	 * The planner is a real call, so a test that queues replies for the steps
	 * would have the first of them swallowed by it. Answering it here keeps
	 * each test about the work it is testing, and still runs the real planner
	 * — including its parse, its caps and its closed set of kinds.
	 */
	if planning(req) {
		if s.plan != "" {
			return llm.Response{Content: s.plan}, nil
		}

		return llm.Response{Content: `{"name":"A job","steps":[{"do":"do the thing","kind":"do"}]}`}, nil
	}

	if checking(req) {
		if len(s.checks) > 0 {
			reply := s.checks[0]
			s.checks = s.checks[1:]

			return llm.Response{Content: reply}, nil
		}

		// Agrees, quoting what it was actually shown — which is what an
		// honest check looks like and what the rule is written against.
		return llm.Response{Content: `{"met":true,"evidence":` +
			asJSON(firstLineOfEvidence(req)) + `,"why":"the tool returned it"}`}, nil
	}

	if len(s.replies) == 0 {
		return llm.Response{Content: "done"}, nil
	}

	r := s.replies[0]
	s.replies = s.replies[1:]

	return r, nil
}

func checking(req llm.Request) bool {
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "Decide whether one step of a job") {
			return true
		}
	}

	return false
}

// firstLineOfEvidence pulls a phrase out of what the checker was shown, so an
// agreeable answer is one that could genuinely be quoted.
func firstLineOfEvidence(req llm.Request) string {
	for _, m := range req.Messages {
		_, after, found := strings.Cut(m.Content, "What the tools returned:\n")
		if !found {
			continue
		}

		for _, line := range strings.Split(after, "\n") {
			if line = strings.TrimSpace(line); len(line) > 6 {
				return line
			}
		}
	}

	return ""
}

func planning(req llm.Request) bool {
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "Break the job below into steps") {
			return true
		}
	}

	return false
}

type said struct {
	lines []string
	db    *store.DB
}

// say records the line the way the brain does: into the thread that asked.
func (s *said) say(conversationID int64, line string) {
	s.lines = append(s.lines, line)

	if s.db != nil && conversationID != 0 {
		s.db.AddMessage(conversationID, "assistant", "", "", line)
	}
}

func (s *said) whole() string { return strings.Join(s.lines, "\n") }

func newConductor(t *testing.T, model llm.Provider, ts ...tools.Tool) (*Conductor, *store.DB, *said) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	spoken := &said{db: db}

	return &Conductor{
		DB:   db,
		Log:  quiet,
		Jobs: &jobs.Runner{},
		Agent: &agent.Loop{
			DB: db, Log: quiet, Quietly: true,
			Registry: tools.NewRegistry(ts...),

			// The same gate a conversation gets: safe tools run, anything that
			// changes something stops and asks.
			MayI: func(_, _ string, changes bool, _ risk.Level) permits.Answer {
				if changes {
					return permits.Ask
				}

				return permits.Allow
			},
		},
		Provider: func(string) (llm.Provider, error) { return model, nil },
		Prompt:   func(string) string { return "You are an assistant." },
		Say:      spoken.say,
		Budget:   Sensible(),
	}, db, spoken
}

// settled waits for a task to stop moving, so a test never depends on how fast
// a goroutine got going.
func settled(t *testing.T, db *store.DB, id int64) *store.Task {
	t.Helper()

	for i := 0; i < 200; i++ {
		task, err := db.Task(id)
		if err != nil {
			t.Fatal(err)
		}

		switch task.State {
		case store.TaskDone, store.TaskBlocked, store.TaskWaiting, store.TaskStopped:
			return task
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("the task never settled")

	return nil
}

/*
 * A request becomes work that runs on its own and says what it did.
 *
 * The whole spine in one test: the job is written down before any of it runs,
 * it works in a thread of its own, it uses a tool, and it reports back in
 * words to the conversation that asked for it.
 */
func TestARequestBecomesWorkThatFinishes(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "list_directory",
			Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
		}}},
		{Content: "There is one file, a.txt."},
	}}

	c, db, spoken := newConductor(t, model, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	task, started, err := c.Take(context.Background(), conv, "list what is in "+dir, "scripted", true)
	if err != nil || !started {
		t.Fatalf("the task did not start: %v %v", started, err)
	}

	// Written down before any of it runs, so it can be read and stopped first.
	if len(task.Steps) != 1 {
		t.Fatalf("the plan was not recorded: %+v", task.Steps)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("the task ended %q because %q", done.State, done.Because)
	}

	steps, _ := db.Steps(task.ID)

	if steps[0].State != store.StepDone {
		t.Errorf("the step ended %q", steps[0].State)
	}

	// It used a tool, so what it says rests on something.
	if steps[0].Verdict != store.Verified {
		t.Errorf("verdict %q, want verified — a tool ran", steps[0].Verdict)
	}

	if !strings.Contains(steps[0].Evidence, "a.txt") {
		t.Errorf("the evidence does not contain what the tool returned: %q", steps[0].Evidence)
	}

	if !strings.Contains(spoken.whole(), "a.txt") {
		t.Errorf("it never said what it found: %q", spoken.whole())
	}

	if !strings.Contains(done.Report, "checked against what the tools returned") {
		t.Errorf("the report does not say what the answer rests on: %q", done.Report)
	}
}

/*
 * A task works in a thread of its own.
 *
 * Ten steps of tool output in the owner's conversation would be read back into
 * the prompt of their next question — where, on this machine, reading the
 * prompt is most of the wait.
 */
func TestATaskDoesNotWorkInTheOwnersConversation(t *testing.T) {
	dir := t.TempDir()

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "list_directory",
			Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
		}}},
		{Content: "Nothing in it."},
	}}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")
	db.AddMessage(conv, "user", "", "", "have a look in there")

	task, _, err := c.Take(context.Background(), conv, "list what is in "+dir, "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	if task.WorkConversationID == conv || task.WorkConversationID == 0 {
		t.Fatalf("the task worked in conversation %d", task.WorkConversationID)
	}

	// And its working notes are not offered as a conversation somebody had.
	recent, _ := db.RecentConversations(10)

	for _, r := range recent {
		if r.ID == task.WorkConversationID {
			t.Error("the task's workspace was listed as a conversation")
		}
	}

	// The report goes to the thread that asked, not the one it worked in.
	history, _ := db.History(conv)

	var reported bool

	for _, m := range history {
		if m.Role == "assistant" && strings.Contains(m.Content, "Finished") {
			reported = true
		}
	}

	if !reported {
		t.Error("the report never reached the conversation that asked for it")
	}
}

/*
 * A step that was supposed to look something up, and did not, did not do it.
 *
 * The cheapest of the four honesty rules and the one that needs no model at
 * all: prose where a tool should have run is the direct descendant of the
 * promised() check in the agent loop, which exists because "I'll go and look"
 * followed by nothing is the failure this program has had more than any other.
 */
func TestWordsWhereAToolShouldHaveRunAreNotAnAnswer(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"find the invoices","kind":"look"}]}`,
		replies: []llm.Response{{Content: "I have taken care of it."}},
	}

	c, db, _ := newConductor(t, model)
	c.Budget = Budget{MostSteps: 12, MostAttempts: 1, MostReplans: 0, MostCalls: 40, HowLong: time.Minute}

	// Nobody to hand the failed step to, so it stops here rather than going
	// to a specialist — which is a different test. See delegate_test.go.
	c.Roster = func() []team.Agent {
		return []team.Agent{{Name: "assistant", Title: "Assistant", For: "anything"}}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "find the invoices and list them", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)
	steps, _ := db.Steps(task.ID)

	if steps[0].Verdict != store.Unmet {
		t.Errorf("verdict %q, want unmet — nothing was looked up", steps[0].Verdict)
	}

	if !strings.Contains(steps[0].Why, "did not use any tool") {
		t.Errorf("it does not say what was wrong: %q", steps[0].Why)
	}

	if done.State != store.TaskBlocked {
		t.Errorf("the task ended %q — saying done here would be a lie", done.State)
	}
}

/*
 * A step that was only ever going to produce words is taken at its word, and
 * recorded as exactly that.
 *
 * "I have written the summary" is not evidence that a summary exists, so this
 * is claimed and never verified. The two are kept apart from the beginning
 * because a record that cannot tell them apart is a record that flatters.
 */
func TestWordsAloneAreAClaimAndSaidToBe(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"write the summary","kind":"write"}]}`,
		replies: []llm.Response{{Content: "Here is the summary of the quarter."}},
	}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write me a summary", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)
	steps, _ := db.Steps(task.ID)

	if steps[0].Verdict != store.Claimed {
		t.Errorf("verdict %q, want claimed", steps[0].Verdict)
	}

	if done.State != store.TaskDone {
		t.Errorf("the task ended %q", done.State)
	}

	if !strings.Contains(done.Report, "its own word") {
		t.Errorf("the report does not say the answer rests on nothing: %q", done.Report)
	}
}

/*
 * A task stops when the budget runs out, and says which one.
 *
 * The case that matters is the one where something has gone wrong and nobody
 * is watching, so the number that stops it lives on the row rather than in
 * memory — a budget that refills on restart is not a budget.
 */
func TestATaskStopsWhenItRunsOutOfThinking(t *testing.T) {
	dir := t.TempDir()

	// A model that never stops asking for the same thing.
	var replies []llm.Response

	for i := 0; i < 40; i++ {
		replies = append(replies, llm.Response{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "list_directory",
			Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
		}}})
	}

	c, db, spoken := newConductor(t, &scripted{replies: replies}, tools.ListDirectory{})

	c.Budget = Budget{MostSteps: 12, MostAttempts: 2, MostReplans: 1, MostCalls: 3, HowLong: time.Minute}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "keep looking in "+dir, "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskBlocked {
		t.Fatalf("the task ended %q, want blocked", done.State)
	}

	if !strings.Contains(done.Because, "thinking it was given") {
		t.Errorf("it did not say which allowance ran out: %q", done.Because)
	}

	if !strings.Contains(spoken.whole(), "Stopped on") {
		t.Errorf("nobody was told it stopped: %q", spoken.whole())
	}
}

/*
 * A task is not a way round the approval gate.
 *
 * The same gate, from the same book, reached by the same loop. What differs is
 * only what happens afterwards: a conversation ends and its owner asks again,
 * where a task has work behind it and parks until the decision is made.
 */
func TestATaskParksRatherThanActingWithoutApproval(t *testing.T) {
	target := filepath.Join(t.TempDir(), "must-not-exist.txt")

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "write_file",
			Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"hello"}`),
		}}},
	}}

	c, db, spoken := newConductor(t, model, tools.WriteFile{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write the summary", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	parked := settled(t, db, task.ID)

	if parked.State != store.TaskWaiting {
		t.Fatalf("the task is %q, want waiting", parked.State)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file was written without anybody approving it")
	}

	// The decision knows which step it belongs to, which is the only route
	// back to work that has been going for a quarter of an hour.
	pending, _ := db.PendingInvocations()

	if len(pending) != 1 {
		t.Fatalf("%d decisions are waiting, want 1", len(pending))
	}

	step, err := db.StepWaitingOn(pending[0].ID)
	if err != nil || step == nil {
		t.Fatalf("the decision does not lead back to a step: %v %v", step, err)
	}

	if step.TaskID != task.ID {
		t.Errorf("the decision leads to task %d, want %d", step.TaskID, task.ID)
	}

	if !strings.Contains(spoken.whole(), "needs your decision") {
		t.Errorf("nobody was told it was waiting: %q", spoken.whole())
	}
}

// A step is told what it is for, not who said it. A brief rather than the
// conversation is what keeps the prompt small and what makes a second agent
// with its own model an addition rather than a rewrite.
func TestAStepIsGivenABriefRatherThanTheConversation(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "Done."}}}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")
	db.AddMessage(conv, "user", "", "", "something private I said earlier")

	task, _, err := c.Take(context.Background(), conv, "tidy the notes", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	if len(model.asked) == 0 {
		t.Fatal("the model was never asked anything")
	}

	var whole string

	for _, m := range model.asked[0].Messages {
		whole += m.Content
	}

	if strings.Contains(whole, "something private I said earlier") {
		t.Error("the step was handed the conversation it came from")
	}

	if !strings.Contains(whole, "tidy the notes") {
		t.Errorf("the step was not told what the job is: %q", whole)
	}
}

// asJSON quotes a path for use inside a tool call's arguments.
func asJSON(s string) string {
	raw, _ := json.Marshal(s)

	return string(raw)
}

/*
 * A task says what it is doing as it does it, in order and with numbers.
 *
 * This is what a phone that was asleep, or a window opened halfway through,
 * catches up from. The test is written against the record rather than the
 * live feed, because the record is the part that has to survive the program
 * being closed.
 */
func TestATaskSaysWhatIsHappening(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "There are three files."}}}

	c, db, _ := newConductor(t, model)
	c.Happens = activity.New(db)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "count the files", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	said, err := db.HappeningsFor(task.ID)
	if err != nil {
		t.Fatal(err)
	}

	var order []string

	for _, h := range said {
		order = append(order, string(h.Type))

		if h.Seq == 0 || h.At.IsZero() {
			t.Errorf("an event has no place in the order: %+v", h)
		}
	}

	line := strings.Join(order, " ")

	for _, want := range []string{string(protocol.TaskStarted), string(protocol.StepStarted),
		string(protocol.StepFinished)} {
		if !strings.Contains(line, want) {
			t.Errorf("nothing said %s:\n%s", want, line)
		}
	}

	if !strings.Contains(line, string(protocol.TaskCompleted)) && !strings.Contains(line, string(protocol.TaskFailed)) {
		t.Errorf("the task ended without saying so:\n%s", line)
	}

	if !strings.HasPrefix(line, string(protocol.TaskStarted)) {
		t.Errorf("it did not start by saying it started:\n%s", line)
	}
}
