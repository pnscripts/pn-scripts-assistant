package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
)

// aProject is a folder with project settings, working under a package.
func aProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if err := workspace.Save(dir, workspace.Config{Name: "Blocks", Kind: "book", Roots: []string{"."},
		Outputs: []string{"export"}, Packages: []string{"writing.book"}}); err != nil {
		t.Fatal(err)
	}

	return dir
}

func projectTask(c *Conductor, conv int64, dir, instruction string) (*store.Task, bool, error) {
	return c.TakeWith(context.Background(), conv, instruction, "scripted", true, Taking{
		Project: dir, Packages: []string{"writing.book"},
		Steps: []store.TaskStep{{Instruction: instruction, Kind: store.StepWrite, Changes: true}},
		Name:  "Blocks",
	})
}

/*
 * A step of a project task that tries to write outside the project is refused
 * — whatever it has been allowed — and one that writes inside it leaves
 * evidence of exactly what it wrote.
 */
func TestAProjectTaskWritesOnlyInItsProject(t *testing.T) {
	dir := aProject(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	inside := filepath.Join(dir, "manuscript", "01.md")

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write_file",
			Arguments: json.RawMessage(`{"path":` + asJSON(outside) + `,"content":"# One"}`)}}},
		{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "write_file",
			Arguments: json.RawMessage(`{"path":` + asJSON(inside) + `,"content":"# Chapter one\n"}`)}}},
		{Content: "Chapter one is written in manuscript/01.md."},
	}}

	c, db, _ := newConductor(t, model, tools.WriteFile{})

	// Everything allowed, so only the project stands in the way.
	c.Agent.MayI = func(string, string, bool, risk.Level) permits.Answer { return permits.Allow }
	c.Packages = func() *capability.Set { return capability.Load("", capability.Known{}) }

	conv, _ := db.NewConversation("t")

	task, started, err := projectTask(c, conv, dir, "Draft chapter one in "+inside)
	if err != nil || !started {
		t.Fatalf("did not start: %v", err)
	}

	done := settled(t, db, task.ID)

	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a project task wrote outside its project")
	}

	if _, err := os.Stat(inside); err != nil {
		t.Fatalf("the chapter was not written inside the project: %v (%s: %s)", err, done.State, done.Because)
	}

	evidence, _ := db.EvidenceFor(task.ID)

	wrote := false

	for _, e := range evidence {
		if e.Kind == store.EvidenceFile && e.Subject == inside && e.OK {
			wrote = true
		}
	}

	if !wrote {
		t.Errorf("the file written is not in the evidence: %+v", evidence)
	}

	// A book is finished when there is a document — and there is one.
	if done.State != store.TaskDone || !strings.Contains(done.Report, "Evidence:") {
		t.Errorf("ended %s because %q:\n%s", done.State, done.Because, done.Report)
	}

	if memory := workspace.Memory(dir, 5); len(memory) == 0 || !strings.Contains(memory[0], "Blocks") {
		t.Errorf("the project's memory does not say what came of it: %v", memory)
	}
}

/*
 * Steps that say they are done, and nothing that shows it, is not finished.
 */
func TestAProjectIsNotFinishedWithoutItsEvidence(t *testing.T) {
	dir := aProject(t)

	model := &scripted{replies: []llm.Response{{Content: "I have written the whole book."}}}

	c, db, _ := newConductor(t, model)
	c.Packages = func() *capability.Set { return capability.Load("", capability.Known{}) }

	conv, _ := db.NewConversation("t")

	task, _, err := projectTask(c, conv, dir, "Write the book")
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskBlocked || !strings.Contains(done.Because, "a written document") {
		t.Errorf("a book with nothing written ended %s because %q", done.State, done.Because)
	}
}

// A project whose settings have gone does no work at all, rather than work
// with no limits.
func TestAProjectWithoutItsSettingsDoesNothing(t *testing.T) {
	dir := aProject(t)

	os.Remove(workspace.Path(dir))

	c, db, _ := newConductor(t, &scripted{}, tools.WriteFile{})

	conv, _ := db.NewConversation("t")

	task, _, err := projectTask(c, conv, dir, "Write the book")
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskBlocked || !strings.Contains(done.Because, "settings") {
		t.Errorf("ended %s because %q", done.State, done.Because)
	}
}

// widening is a model whose first step's answer arrives after the project's
// settings were given another integration — by the owner, a build script or
// anything else no single-call check sees.
type widening struct {
	*scripted
	dir string

	// Its own lock rather than the embedded one: scripted.Chat takes that,
	// and this calls through to it.
	mu   sync.Mutex
	done bool
}

func (w *widening) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	w.mu.Lock()
	first := !w.done
	w.done = true
	w.mu.Unlock()

	if first {
		cfg, _ := workspace.Load(w.dir)
		cfg.Integrations = append(cfg.Integrations, "fetch")
		workspace.Save(w.dir, cfg)
	}

	return w.scripted.Chat(ctx, req)
}

/*
 * Settings widened while a task runs stop it at its next step, whoever
 * widened them: a task approved under one list of integrations does not
 * carry on under a longer one.
 */
func TestSettingsWidenedMidTaskStopIt(t *testing.T) {
	dir := aProject(t)

	model := &widening{scripted: &scripted{replies: []llm.Response{
		{Content: "Chapter one."}, {Content: "Chapter two."},
	}}, dir: dir}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Two chapters", "scripted", true, Taking{
		Project: dir, Packages: []string{"writing.book"}, Name: "Blocks",
		Steps: []store.TaskStep{
			{Instruction: "Draft chapter one", Kind: store.StepWrite, Changes: true},
			{Instruction: "Draft chapter two", Kind: store.StepWrite, Changes: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskBlocked || !strings.Contains(done.Because, "integration fetch") {
		t.Errorf("ended %s because %q", done.State, done.Because)
	}
}

func TestNarrowedSettingsAreSimplyObeyed(t *testing.T) {
	before := workspace.Config{Roots: []string{"."}, Integrations: []string{"git", "fetch"},
		Commands: map[string][]string{"test": {"npm", "test"}}}

	narrower := workspace.Config{Roots: []string{"."}, Integrations: []string{"git"},
		Commands: map[string][]string{"test": {"npm", "test"}}}

	if why := workspace.Widened(before, narrower); why != "" {
		t.Errorf("narrowing counted as widening: %s", why)
	}

	wider := workspace.Config{Roots: []string{".", "../other"}, Commands: map[string][]string{"test": {"sh", "-c", "x"}}}

	why := workspace.Widened(before, wider)

	for _, want := range []string{"folder ../other", "the test command"} {
		if !strings.Contains(why, want) {
			t.Errorf("%q does not say %q", why, want)
		}
	}
}

/*
 * The audit's cases, each once: a pass followed by a failure is a failure; a
 * pass followed by an edit proves nothing about the edit; and work under a
 * package is judged on its evidence whether or not it has a project.
 */
func TestOnlyTheLatestFreshEvidenceCounts(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "b.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	set := capability.Load("", capability.Known{})
	c := &Conductor{DB: db, Packages: func() *capability.Set { return set }}

	judge := func(project string, rows ...store.Evidence) (string, string) {
		id, _ := db.NewTask(store.Task{Name: "tetris", Goal: "x", Project: project, Packages: "software.game.threejs"})

		for _, e := range rows {
			e.TaskID = id
			db.Record(e)
		}

		task, _ := db.Task(id)

		return c.judged(task, store.TaskDone, "")
	}

	good := []store.Evidence{
		{Kind: store.EvidenceFile, Subject: "/p/src/main.js", OK: true},
		{Kind: store.EvidenceCheck, Subject: "threejs check", OK: true},
		{Kind: store.EvidenceSmoke, Subject: "threejs smoke", OK: true},
		{Kind: store.EvidenceShot, Subject: "/p/shot.png", OK: true},
		{Kind: store.EvidenceVersion, Subject: "three.js", Detail: "r169", OK: true},
	}

	if state, why := judge(t.TempDir(), good...); state != store.TaskDone {
		t.Fatalf("complete, fresh evidence judged %s: %s", state, why)
	}

	failedLast := append(append([]store.Evidence{}, good...),
		store.Evidence{Kind: store.EvidenceCheck, Subject: "threejs check", Detail: "SyntaxError main.js:1"})

	if state, why := judge(t.TempDir(), failedLast...); state != store.TaskBlocked || !strings.Contains(why, "latest one failed") {
		t.Errorf("a failed latest check judged %s: %s", state, why)
	}

	editedAfter := append(append([]store.Evidence{}, good...),
		store.Evidence{Kind: store.EvidenceFile, Subject: "/p/src/main.js", Detail: "edited", OK: true})

	if state, why := judge(t.TempDir(), editedAfter...); state != store.TaskBlocked || !strings.Contains(why, "files changed after") {
		t.Errorf("evidence from before the last edit judged %s: %s", state, why)
	}

	if state, why := judge(""); state != store.TaskBlocked {
		t.Errorf("a package task with no project and no evidence judged %s: %s", state, why)
	}
}

// The time its owner approved for a project is the time it is given, and
// the hire goes in with the task rather than after it.
func TestAnApprovedProjectKeepsItsTimeAndItsHire(t *testing.T) {
	dir := aProject(t)

	c, db, _ := newConductor(t, &scripted{})
	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Blocks", "scripted", true, Taking{
		Project: dir, Packages: []string{"writing.book"}, Name: "Blocks",
		Steps:   []store.TaskStep{{Instruction: "Draft chapter one", Kind: store.StepWrite, Changes: true}},
		HowLong: 90 * time.Minute, Hired: "Writer — hired for this project",
	})
	if err != nil {
		t.Fatal(err)
	}

	row, _ := db.Task(task.ID)

	if left := time.Until(row.Deadline); left < 85*time.Minute || left > 91*time.Minute {
		t.Errorf("the task was given %s, not the ninety minutes approved", left)
	}

	if row.Hired != "Writer — hired for this project" {
		t.Errorf("the hire was not kept with the task: %q", row.Hired)
	}
}

// A file a specialist changed for one of the project's steps is a change to
// the project: the checks passed before it no longer stand. And the
// specialist's own one-step task is not judged as if it were the project.
func TestAChildsChangesCountInTheProjectsRecord(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "b.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	set := capability.Load("", capability.Known{})
	c := &Conductor{DB: db, Packages: func() *capability.Set { return set }}

	dir := t.TempDir()
	parent, _ := db.NewTask(store.Task{Name: "tetris", Goal: "x", Project: dir, Packages: "software.game.threejs"})

	for _, e := range []store.Evidence{
		{Kind: store.EvidenceFile, Subject: "/p/src/main.js", OK: true},
		{Kind: store.EvidenceCheck, Subject: "threejs check", OK: true},
		{Kind: store.EvidenceSmoke, Subject: "threejs smoke", OK: true},
		{Kind: store.EvidenceShot, Subject: "/p/shot.png", OK: true},
		{Kind: store.EvidenceVersion, Subject: "three.js", Detail: "r169", OK: true},
	} {
		e.TaskID = parent
		db.Record(e)
	}

	child, _ := db.NewTask(store.Task{Name: "for tetris", Goal: "y", Project: dir, Packages: "software.game.threejs",
		ParentTaskID: parent, Depth: 1})
	db.Record(store.Evidence{TaskID: child, Kind: store.EvidenceFile, Subject: "/p/src/main.js", OK: true})

	task, _ := db.Task(parent)
	if state, why := c.judged(task, store.TaskDone, ""); state == store.TaskDone || !strings.Contains(why, "changed after") {
		t.Errorf("a child's change left the project's older check standing: %s %s", state, why)
	}

	own, _ := db.Task(child)
	if state, why := c.judged(own, store.TaskDone, ""); state != store.TaskDone {
		t.Errorf("a one-step child was judged as the whole project: %s %s", state, why)
	}
}
