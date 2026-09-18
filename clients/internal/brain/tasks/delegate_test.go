package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"pn-scripts-assistant/internal/brain/workspace"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

// finished waits until a task is done or blocked, skipping the waiting it
// does while a specialist has one of its steps.
func finished(t *testing.T, db *store.DB, id int64) *store.Task {
	t.Helper()

	for i := 0; i < 20; i++ {
		task := settled(t, db, id)

		if task.State == store.TaskDone || task.State == store.TaskBlocked || task.State == store.TaskStopped {
			return task
		}

		if !strings.HasPrefix(task.Because, "waiting on") {
			return task
		}

		// Parked on its specialist: give the child time to finish and the
		// parent time to be picked back up.
		for j := 0; j < 50; j++ {
			again, _ := db.Task(id)

			if again.State != store.TaskWaiting {
				break
			}

			settledChildren(t, db, id)
		}
	}

	return settled(t, db, id)
}

func settledChildren(t *testing.T, db *store.DB, id int64) {
	children, _ := db.Children(id)

	for _, child := range children {
		settled(t, db, child.ID)
	}
}

/*
 * A step its agent could not do goes to somebody who can, and comes back.
 *
 * The specification's own example: a backend engineer with a database
 * problem, and a database specialist. The generalist tries twice without
 * looking anything up, the specialist is found on the organisation in code,
 * does it as a task of its own, and the parent carries on with the answer —
 * verified, because the specialist's step was.
 */
func TestAStepNobodyCouldDoGoesToASpecialist(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "slow.sql"), []byte("select * from orders"), 0o600)

	model := &scripted{
		plan: `{"name":"Slow queries","steps":[{"do":"Find why the postgresql queries are slow","kind":"look","who":"assistant"}]}`,
		replies: []llm.Response{
			{Content: "Probably an index."},
			{Content: "Probably an index, still."},
			{ToolCalls: []llm.ToolCall{{
				ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
			}}},
			{Content: "slow.sql scans orders without an index."},
		},
	}

	c, db, spoken := newConductor(t, model, tools.ListDirectory{})

	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "assistant", Title: "Assistant", For: "anything"},
			{Name: "dba", Title: "Database specialist",
				For:   "postgresql databases, slow queries and indexes",
				Tools: []string{"list_directory"}},
		}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "why are the queries slow", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := finished(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("the parent ended %s: %s", done.State, done.Because)
	}

	children, _ := db.Children(task.ID)

	if len(children) != 1 {
		t.Fatalf("%d child tasks, want 1", len(children))
	}

	child := children[0]
	steps, _ := db.Steps(task.ID)

	if child.ParentStepID != steps[0].ID || child.Depth != 1 {
		t.Errorf("the child does not say whose step it is: %+v", child)
	}

	if steps[0].Assignee != "dba" || steps[0].Verdict != store.Verified || steps[0].HandedTo != child.ID {
		t.Errorf("the parent's step does not carry the specialist's work: %+v", steps[0])
	}

	if !strings.Contains(spoken.whole(), "database specialist") {
		t.Errorf("it never said the step had gone to somebody else: %s", spoken.whole())
	}

	// And the budget came out of the parent's purse, with what was not spent
	// given back: a delegation does not mint thinking out of nothing.
	if done.CallsLeft >= Sensible().MostCalls {
		t.Errorf("the parent's budget did not pay for the child: %d left", done.CallsLeft)
	}
}

/*
 * Handing work on cannot move what may be done.
 *
 * The researcher may only list folders; the specialist may also read files.
 * On a step the researcher handed on, the specialist gets the overlap — and
 * nothing either was told never to do.
 */
func TestAChildMayUseOnlyWhatItsParentCould(t *testing.T) {
	narrow := team.Fit{Tools: []string{"list_directory"}, Narrowed: true, Never: []string{"send_email"}}

	within := narrowerOf(nil, narrow)

	if within == nil || len(*within) != 1 || (*within)[0] != "list_directory" {
		t.Fatalf("the child's limit is %v", within)
	}

	child := &store.Task{Within: within}
	wide := team.Fit{Tools: []string{"read_file", "list_directory"}, Narrowed: true}

	only, withTools := held(child, wide)

	if !withTools || len(only) != 1 || only[0] != "list_directory" {
		t.Errorf("the specialist was given %v on a step handed on by somebody who had only list_directory", only)
	}

	// The generalist, who has no list at all, gets the parent's list rather
	// than everything.
	if only, _ := held(child, team.Fit{}); len(only) != 1 {
		t.Errorf("a generalist handed a narrow step was given %v", only)
	}

	// Handed on by somebody allowed nothing, the step may use nothing — not,
	// as an empty list means elsewhere, everything.
	none := narrowerOf(nil, team.Fit{Narrowed: true})

	if _, withTools := held(&store.Task{Within: none}, wide); withTools {
		t.Error("a step handed on by an agent with no tools was given tools")
	}

	// And a second handing-on narrows again, never widens back.
	again := narrowerOf(within, wide)

	if len(*again) != 1 {
		t.Errorf("the grandchild's limit widened: %v", *again)
	}
}

/*
 * The child's limit is enforced where it runs, not only written down.
 *
 * The specialist reaches for read_file on a step handed on by an agent who
 * could only list folders. The loop refuses it at execute time, and nothing
 * of the file reaches the thread.
 */
func TestAChildIsRefusedWhatItsParentWasHeldFrom(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "notes.txt")
	os.WriteFile(secret, []byte("the contents of the notes"), 0o600)

	model := &scripted{replies: []llm.Response{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "read_file",
			Arguments: json.RawMessage(`{"path":` + asJSON(secret) + `}`),
		}}},
		{Content: "I could not read it."},
	}}

	c, db, _ := newConductor(t, model, tools.ReadFile{}, tools.ListDirectory{})

	c.Roster = func() []team.Agent {
		return []team.Agent{{Name: "reader", Title: "Reader", For: "reading",
			Tools: []string{"read_file", "list_directory"}}}
	}

	work, _ := db.NewTaskThread("child")
	limit := []string{"list_directory"}

	id, _ := db.NewTask(store.Task{Name: "child", Goal: "read the notes", State: store.TaskWorking,
		WorkConversationID: work, StepsLeft: 3, CallsLeft: 10, Within: &limit, Depth: 1})
	db.AddSteps(id, []store.TaskStep{{Instruction: "Read the notes", Kind: store.StepLook, Assignee: "reader"}})

	if err := c.Work(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	messages, _ := db.History(work)

	for _, m := range messages {
		if strings.Contains(m.Content, "the contents of the notes") {
			t.Fatal("a tool outside the child's limit ran")
		}
	}
}

/*
 * With nobody better placed, a failed step goes up the reporting line.
 *
 * Straight after the step that failed, not at the end of the plan, and to the
 * nearest seat above that somebody is actually sitting in.
 */
func TestAFailedStepGoesUpToTheManager(t *testing.T) {
	model := &scripted{
		plan: `{"name":"Widget","steps":[
			{"do":"Frobnicate the widget","kind":"look","who":"dev"},
			{"do":"Tell somebody","kind":"write","who":"dev"}]}`,
		replies: []llm.Response{{Content: "No idea."}, {Content: "Still no idea."}},
	}

	c, db, _ := newConductor(t, model)

	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "assistant", Title: "Assistant", For: "anything"},
			{Name: "dev", Title: "Developer", For: "code", Position: "dev"},
			{Name: "lead", Title: "Lead developer", For: "people", Position: "lead"},
		}
	}

	c.Chart = func() []org.Unit {
		return []org.Unit{{Name: "eng", Kind: org.Department, Seats: []org.Position{
			{Name: "lead"},
			{Name: "empty", ReportsTo: "lead"},
			{Name: "dev", ReportsTo: "empty"},
		}}}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "the widget", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	steps, _ := db.Steps(task.ID)

	// The manager's step may fail too, and then the plan is reconsidered as
	// it always was — so the count after that is not the point. What is: the
	// step straight after the failure is the manager's.
	if len(steps) < 3 {
		t.Fatalf("%d steps, want the manager's to have been added: %+v", len(steps), steps)
	}

	up := steps[1]

	if up.EscalatedFrom != steps[0].ID || up.Assignee != "lead" {
		t.Errorf("the second step is not the manager's answer to the first: %+v", up)
	}

	if !strings.Contains(up.Instruction, "Frobnicate the widget") {
		t.Errorf("the manager was not told what could not be done: %q", up.Instruction)
	}

	if steps[2].Instruction != "Tell somebody" {
		t.Errorf("the rest of the plan moved: %+v", steps[2])
	}
}

/*
 * A serious step that finished is looked at by somebody else.
 *
 * And "wrong" takes its verdict away. The deployment still happened and the
 * account says so, but nothing later rests on it as though it went well.
 */
func TestASeriousStepIsReviewedAndWrongMeansNotEstablished(t *testing.T) {
	dir := t.TempDir()

	model := &scripted{
		plan: `{"name":"Release","steps":[{"do":"Deploy the build to production","kind":"do","who":"assistant"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{
				ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
			}}},
			{Content: "Deployed."},
			{Content: "Wrong: that folder is the staging build, not production."},
		},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "assistant", Title: "Assistant", For: "anything"},
			{Name: "ops", Title: "Operations engineer", For: "deploy builds to production servers"},
		}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "release it", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)
	steps, _ := db.Steps(task.ID)

	if len(steps) != 2 || steps[1].ReviewOf != steps[0].ID || steps[1].Assignee != "ops" {
		t.Fatalf("the deployment was not reviewed by somebody else: %+v", steps)
	}

	if steps[0].Verdict != store.Unmet || !strings.Contains(steps[0].Why, "staging") {
		t.Errorf("a step reviewed as wrong kept its verdict: %+v", steps[0])
	}

	if !strings.Contains(done.Report, "found wrong when it was reviewed") {
		t.Errorf("the account does not say the review found it wrong:\n%s", done.Report)
	}
}

// A step is handed on only to somebody free to take it: not a hire for a
// different task, and for a project, somebody who works under its packages.
func TestAStepGoesOnlyToSomebodyFreeToTakeIt(t *testing.T) {
	godotGame := &store.Task{ID: 8, Packages: "software.game.godot"}

	webDev := team.Agent{Name: "web", HiredFor: 5, Packages: []string{"software.game.threejs"}}
	if availableFor(webDev, godotGame, 8) {
		t.Error("another task's three.js hire was given a Godot game's step")
	}

	webDev.HiredFor = 0
	if availableFor(webDev, godotGame, 8) {
		t.Error("a three.js developer was given a Godot game's step")
	}

	godotDev := team.Agent{Name: "godot", Packages: []string{"software.game.godot"}}
	anybody := team.Agent{Name: "writer"}
	ownHire := team.Agent{Name: "hired", HiredFor: 8, Packages: []string{"software.game.godot"}}

	for _, a := range []team.Agent{godotDev, anybody, ownHire} {
		if !availableFor(a, godotGame, 8) {
			t.Errorf("%s could not be given a step they are free to take", a.Name)
		}
	}
}

// A step of a project handed on stays a step of that project: the specialist's
// task carries the project and its packages, so it is confined to the folder
// and its files count in the project's record.
func TestAProjectStepHandedOnStaysInTheProject(t *testing.T) {
	dir := t.TempDir()
	if err := workspace.Save(dir, workspace.Config{Name: "Tetris", Kind: "game", Engine: "threejs"}); err != nil {
		t.Fatal(err)
	}

	model := &scripted{replies: []llm.Response{{Content: "Probably fine."}, {Content: "Still probably fine."}}}

	c, db, _ := newConductor(t, model)

	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "assistant", Title: "Assistant", For: "anything"},
			{Name: "games", Title: "Game developer", For: "writing tetris games in a project",
				Tools: []string{"list_directory"}},
		}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.TakeWith(context.Background(), conv, "Make me a Tetris game", "scripted", true, Taking{
		Project: dir, Packages: []string{"software.game.threejs"}, Name: "Tetris",
		Steps: []store.TaskStep{{Instruction: "Write the tetris game in the project", Kind: store.StepDo, Changes: true,
			Assignee: "assistant"}}})
	if err != nil {
		t.Fatal(err)
	}

	finished(t, db, task.ID)

	children, _ := db.Children(task.ID)
	if len(children) == 0 {
		t.Fatal("the step was not handed on, so this proves nothing")
	}

	if children[0].Project != dir || children[0].Packages != "software.game.threejs" {
		t.Errorf("the specialist's task left the project behind: project %q, packages %q",
			children[0].Project, children[0].Packages)
	}
}
