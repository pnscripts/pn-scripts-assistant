package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
)

// fakeAgent is a coding agent that does what the test says.
type fakeAgent struct {
	id    string
	calls *int
	do    func(job orchestrator.Job) orchestrator.Outcome
}

func (f fakeAgent) ID() string { return f.id }

func (f fakeAgent) Run(_ context.Context, job orchestrator.Job) orchestrator.Outcome {
	*f.calls++

	return f.do(job)
}

func codingAgent(id string, can int) orchestrator.Resource {
	return orchestrator.Resource{ID: id, Title: id, Kind: orchestrator.Agent, Tier: orchestrator.Subscription,
		State: orchestrator.Available, Executes: true, Can: map[string]int{orchestrator.Code: can, orchestrator.Edit: can, "tools": 1}}
}

func writes(name, text string) func(job orchestrator.Job) orchestrator.Outcome {
	return func(job orchestrator.Job) orchestrator.Outcome {
		path := filepath.Join(job.Dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(text), 0o644)

		return orchestrator.Outcome{OK: true, Said: "wrote " + name, Files: []string{path}}
	}
}

func projectFor(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if err := workspace.Save(dir, workspace.Config{Name: "Tetris", Kind: "game", Engine: "threejs"}); err != nil {
		t.Fatal(err)
	}

	return dir
}

func writeStep(dir string) Taking {
	return Taking{Project: dir, Name: "Tetris", Steps: []store.TaskStep{
		{Instruction: "Write the game in " + dir, DoneWhen: "the game's files are written", Kind: store.StepDo, Changes: true},
	}}
}

/*
 * The first choice is signed out; the next takes the same step over. Nothing
 * is started again, the switch is recorded with its reason, the owner is told,
 * and the step is judged on the file that is really there.
 */
func TestASignedOutAgentGivesWayAndTheWorkCarriesOn(t *testing.T) {
	dir := projectFor(t)

	c, db, spoken := newConductor(t, &scripted{})

	first, second := 0, 0

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db,
		Extra: []orchestrator.Resource{codingAgent("claude-code", 9), codingAgent("codex", 8)},
		Executors: map[string]orchestrator.Executor{
			"claude-code": fakeAgent{id: "claude-code", calls: &first, do: func(orchestrator.Job) orchestrator.Outcome {
				return orchestrator.Outcome{Failure: orchestrator.FailAuth, Detail: "OAuth session expired and could not be refreshed"}
			}},
			"codex": fakeAgent{id: "codex", calls: &second, do: writes("src/main.js", "console.log('tetris')")},
		}})

	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true, writeStep(dir))
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone || first != 1 || second != 1 {
		t.Fatalf("ended %s (%s); claude asked %d times, codex %d", done.State, done.Because, first, second)
	}

	kinds := map[string]string{}

	evidence, _ := db.EvidenceFor(task.ID)
	for _, e := range evidence {
		kinds[e.Kind] += e.Subject + " | " + e.Detail + "\n"
	}

	if !strings.Contains(kinds[store.EvidenceSwitch], "claude-code → codex") ||
		!strings.Contains(kinds[store.EvidenceSwitch], "OAuth session expired") {
		t.Errorf("the switch is not recorded with its reason: %q", kinds[store.EvidenceSwitch])
	}

	if !strings.Contains(kinds[store.EvidenceDecision], "Selected claude-code") ||
		!strings.Contains(kinds[store.EvidenceFile], "main.js") {
		t.Errorf("the decision or the file written is missing:\n%v", kinds)
	}

	if u, _ := db.UsageOf("claude-code"); u == nil || u.State != string(orchestrator.AuthRequired) {
		t.Errorf("the signed-out agent was not remembered as signed out: %+v", u)
	}

	if !strings.Contains(strings.Join(spoken.lines, "\n"), "Switched from claude-code to codex") {
		t.Errorf("the owner was not told of the switch: %v", spoken.lines)
	}

	// And the step says who wrote it: the one that took over.
	if steps, _ := db.Steps(task.ID); len(steps) != 1 || steps[0].Provider != "agent:codex" {
		t.Errorf("the step does not name who wrote it: %+v", steps)
	}
}

// Writing outside the project is not a failure to route around: it stops,
// and nobody else is given the step to try the same.
func TestAnAgentThatWritesOutsideStopsTheWork(t *testing.T) {
	dir := projectFor(t)

	c, db, _ := newConductor(t, &scripted{})

	first, second := 0, 0

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db,
		Extra: []orchestrator.Resource{codingAgent("claude-code", 9), codingAgent("codex", 8)},
		Executors: map[string]orchestrator.Executor{
			"claude-code": fakeAgent{id: "claude-code", calls: &first, do: func(orchestrator.Job) orchestrator.Outcome {
				return orchestrator.Outcome{Failure: orchestrator.FailOutside, Outside: []string{"/home/x/.bashrc"},
					Detail: "it wrote outside the project: /home/x/.bashrc"}
			}},
			"codex": fakeAgent{id: "codex", calls: &second, do: writes("src/main.js", "x")},
		}})

	conv, _ := db.NewConversation("t")

	task, _, _ := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true, writeStep(dir))

	done := settled(t, db, task.ID)

	if done.State == store.TaskDone || second != 0 {
		t.Errorf("ended %s after writing outside; codex was asked %d times", done.State, second)
	}
}

// gameCheck stands in for the engine: it passes once fixed.txt exists.
type gameCheck struct{ dir string }

func (gameCheck) Name() string                     { return "game_check" }
func (gameCheck) Description() string              { return "checks the game" }
func (gameCheck) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object"}`) }
func (gameCheck) Risk() tools.Risk                 { return tools.Safe }
func (gameCheck) Summarize(json.RawMessage) string { return "check" }
func (g gameCheck) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	out, _, err := g.ExecuteShowing(ctx, raw)

	return out, err
}

func (g gameCheck) ExecuteShowing(context.Context, json.RawMessage) (string, []store.Evidence, error) {
	if _, err := os.Stat(filepath.Join(g.dir, "fixed.txt")); err == nil {
		return "threejs check: passed", []store.Evidence{{Kind: store.EvidenceCheck, Subject: "threejs check", OK: true}}, nil
	}

	return "threejs check: failed\n  error: ReferenceError: Can't find variable: board (main.js:12)",
		[]store.Evidence{{Kind: store.EvidenceCheck, Subject: "threejs check", Detail: "1 errors"}}, nil
}

/*
 * A check is this program's, not a model's: it runs the engine, and what
 * fails goes back to be fixed — the error, file and line handed over — and
 * the engine runs again until it passes.
 */
func TestAFailingCheckGoesBackToBeFixed(t *testing.T) {
	dir := projectFor(t)

	c, db, _ := newConductor(t, &scripted{}, gameCheck{dir: dir})

	calls := 0
	var told string

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db,
		Extra: []orchestrator.Resource{codingAgent("claude-code", 9)},
		Executors: map[string]orchestrator.Executor{
			"claude-code": fakeAgent{id: "claude-code", calls: &calls, do: func(job orchestrator.Job) orchestrator.Outcome {
				told = job.Prompt

				return writes("fixed.txt", "fixed")(job)
			}},
		}})

	conv, _ := db.NewConversation("t")

	task, _, _ := c.TakeWith(context.Background(), conv, "Check the game", "scripted", true, Taking{
		Project: dir, Name: "Tetris", Steps: []store.TaskStep{{Instruction: "Check it", DoneWhen: "it passes",
			Kind: store.StepCheck, Changes: true, Action: "engine:check"}}})

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone || calls != 1 {
		t.Fatalf("ended %s (%s) after %d fixes", done.State, done.Because, calls)
	}

	if !strings.Contains(told, "main.js:12") {
		t.Errorf("the fixer was not told what failed and where:\n%s", told)
	}

	var order []string

	evidence, _ := db.EvidenceFor(task.ID)
	for _, e := range evidence {
		if e.Kind == store.EvidenceCheck || e.Kind == store.EvidenceFile {
			order = append(order, e.Kind+":"+map[bool]string{true: "ok", false: "failed"}[e.OK])
		}
	}

	if strings.Join(order, " ") != "check:failed file:ok check:ok" {
		t.Errorf("the record is not the failed check, the fix and the passing check, in order: %v", order)
	}

	// The fix passed this program's check, so its record says verified.
	runs, _ := db.Runs("claude-code", 10)
	if len(runs) != 1 || !runs[0].Verified {
		t.Errorf("the fix that passed the check is not recorded as verified: %+v", runs)
	}
}

// What a check points at is quoted from the file exactly, so the fix can
// copy it; nothing outside the project is read.
func TestTheLinesACheckPointsAtAreQuoted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "main.js"),
		[]byte("a();\nb();\nconst m = new M({ color: 1 };\nc();\nd();\n"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("do not read\n"), 0o644)

	found := "error: SyntaxError: missing ) after argument list (src/main.js:3)\nerror: ../secret.txt:1 and res://src/main.js:3"
	said := pointAt(dir, found)

	if !strings.Contains(said, "  3 | const m = new M({ color: 1 };") || !strings.Contains(said, "  2 | b();") {
		t.Errorf("the line was not quoted:\n%s", said)
	}

	if strings.Contains(said, "do not read") || strings.Count(said, "around line 3") != 1 {
		t.Errorf("read outside the project, or quoted twice:\n%s", said)
	}

	if pointAt(dir, "no places here") != "no places here" {
		t.Error("a report with no places was changed")
	}
}

/*
 * A model chosen to write a project that only looks at it, and says the files
 * are written, has not written them. Seen on this machine: its look at the
 * folder was taken as the proof, and the untouched template went on to be
 * checked as if it were the game.
 */
func TestAModelThatOnlyLooksHasWrittenNothing(t *testing.T) {
	dir := projectFor(t)

	look := llm.Response{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
		Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}}
	said := llm.Response{Content: "The game's files are written in the project."}

	model := &scripted{replies: []llm.Response{look, said, look, said}}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db, Extra: []orchestrator.Resource{{
		ID: "ollama:qwen", Title: "qwen (local)", Kind: orchestrator.Model, Tier: orchestrator.Local, Local: true,
		State: orchestrator.Available, Executes: true, Model: "qwen",
		Can: map[string]int{orchestrator.Code: 7, orchestrator.Edit: 7, "tools": 1}}}})

	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true,
		Taking{Project: dir, Name: "Tetris", Steps: []store.TaskStep{
			{Instruction: "Write the game in " + dir, Kind: store.StepDo, Changes: true}}})
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State == store.TaskDone {
		t.Fatal("a step that wrote nothing was accepted as having written the game")
	}

	steps, _ := db.Steps(task.ID)
	if len(steps) == 0 || steps[0].Verdict == store.Verified || !strings.Contains(steps[0].Why, "changed no files") {
		t.Errorf("the step was not judged on what it wrote: %+v", steps)
	}

	// Judged, and tried again — the same writer, being the only one — rather
	// than the task stopping at the first silence.
	if len(steps) > 0 && steps[0].Attempts < 2 {
		t.Errorf("a step that wrote nothing was not tried again: %d attempts", steps[0].Attempts)
	}

	// And its record says it failed, so no later check is credited to it.
	runs, _ := db.Runs("ollama:qwen", 10)
	failed := 0

	for _, r := range runs {
		switch {
		case r.Result == store.RunOK:
			t.Errorf("a run that wrote nothing is on record as done: %+v", r)
		case r.Result == store.RunFailed && strings.Contains(r.Detail+r.Failure, "changed no files"):
			failed++
		}
	}

	if failed == 0 {
		t.Errorf("no run is on record as having written nothing: %+v", runs)
	}
}

// A fix that changes nothing ends the fixing: the engine would only find the
// same thing again. And the fixer is asked to change files, not to run them.
func TestAFixThatChangesNothingStopsTheRounds(t *testing.T) {
	dir := projectFor(t)

	c, db, _ := newConductor(t, &scripted{}, gameCheck{dir: dir})

	calls := 0
	var told string

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db,
		Extra: []orchestrator.Resource{codingAgent("claude-code", 9)},
		Executors: map[string]orchestrator.Executor{
			"claude-code": fakeAgent{id: "claude-code", calls: &calls, do: func(job orchestrator.Job) orchestrator.Outcome {
				told = job.Prompt

				return orchestrator.Outcome{OK: true, Said: "Understood."}
			}},
		}})

	conv, _ := db.NewConversation("t")

	task, _, _ := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true, Taking{
		Project: dir, Name: "Tetris", Steps: []store.TaskStep{{Instruction: "Run the game with game_build",
			Kind: store.StepCheck, Changes: true, Action: "engine:check"}}})

	done := settled(t, db, task.ID)

	// One round of nothing per attempt at the step, where it used to be
	// every round there is.
	if done.State == store.TaskDone || calls > Sensible().MostAttempts {
		t.Fatalf("ended %s after %d rounds of changing nothing", done.State, calls)
	}

	// And the check, still failing, stays this program's: not handed to a
	// specialist to be declared done on their word.
	if children, _ := db.Children(task.ID); len(children) != 0 {
		t.Errorf("the program's own check was handed on: %+v", children[0])
	}

	if steps, _ := db.Steps(task.ID); len(steps) == 1 && steps[0].Verdict == store.Claimed {
		t.Error("a failing check was counted as done on somebody's word")
	}

	if !strings.Contains(told, "Change the project's files so that it passes") || strings.Contains(told, "Run the game with game_build") ||
		!strings.Contains(told, "Make me a Tetris game") {
		t.Errorf("the fixer was not asked to change the files for what was wanted:\n%s", told)
	}
}

// A model writing a project is given its files and nothing that runs them,
// and never, by an empty list, everything.
func TestAWriterIsGivenFilesNotEngines(t *testing.T) {
	got := writing([]string{"game_check", "game_build", "read_file", "write_file", "run_command"})
	if strings.Join(got, ",") != "read_file,write_file" {
		t.Errorf("the writer was given %v", got)
	}

	if got := writing(nil); len(got) == 0 || slices.Contains(got, "game_build") {
		t.Errorf("the generalist writer was given %v", got)
	}

	if got := writing([]string{"game_build"}); len(got) == 0 {
		t.Error("nothing left became everything")
	}
}

// busyAgent works until it is stopped.
type busyAgent struct{ started chan struct{} }

func (busyAgent) ID() string { return "claude-code" }

func (b busyAgent) Run(ctx context.Context, _ orchestrator.Job) orchestrator.Outcome {
	close(b.started)
	<-ctx.Done()

	return orchestrator.Outcome{Failure: orchestrator.FailError, Detail: "it was killed"}
}

// Stopping a task while its writer works ends it as stopped — not blocked,
// and not a failure on the writer's record.
func TestStoppingAWriterIsNotItFailing(t *testing.T) {
	dir := projectFor(t)

	c, db, _ := newConductor(t, &scripted{})

	busy := busyAgent{started: make(chan struct{})}

	c.Orchestrator = orchestrator.New(orchestrator.Sources{DB: db,
		Extra:     []orchestrator.Resource{codingAgent("claude-code", 9)},
		Executors: map[string]orchestrator.Executor{"claude-code": busy}})

	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true, writeStep(dir))
	if err != nil {
		t.Fatal(err)
	}

	<-busy.started

	if err := c.Stop(task.ID); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if done, _ := db.Task(task.ID); done.State != store.TaskStopped {
		t.Errorf("a stopped task ended %s: %s", done.State, done.Because)
	}

	if runs, _ := db.Runs("claude-code", 10); len(runs) != 0 {
		t.Errorf("being stopped went on the writer's record: %+v", runs)
	}
}

// A step that writes is told to write, with the tools, now; a step that looks
// is told to say what it found. The first used to be told the second, and a
// small model duly said what it would do and stopped.
func TestAWritingStepIsToldToWrite(t *testing.T) {
	c, db, _ := newConductor(t, &scripted{})

	conv, _ := db.NewConversation("t")
	task := &store.Task{Goal: "Make me a Tetris game", ConversationID: conv}

	last := func(step store.TaskStep) string {
		msgs := c.brief(task, &step, team.Agent{Name: "developer", Title: "Developer", For: "code"})

		return msgs[len(msgs)-1].Content
	}

	write := last(store.TaskStep{Instruction: "Write the game", Kind: store.StepDo, Changes: true})
	if !strings.Contains(write, "write_file") || strings.Contains(write, "say what you found") {
		t.Errorf("a writing step was not told to write:\n%s", write)
	}

	look := last(store.TaskStep{Instruction: "Find the slow query", Kind: store.StepLook})
	if !strings.Contains(look, "say what you found") || strings.Contains(look, "write_file") {
		t.Errorf("a looking step was told to write:\n%s", look)
	}
}
