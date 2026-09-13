package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * decide is what the brain does when somebody answers: run the approved tool
 * with the arguments that were stored, record what happened, and let whatever
 * was waiting on it carry on.
 *
 * Written out here rather than reached through the brain, because the brain
 * needs a window, a speech stack and a model to build. What matters for these
 * tests is the shape — decided first, executed second, resumed third — and
 * that shape is asserted against the real Conductor.
 */
func decide(t *testing.T, c *Conductor, db *store.DB, registry *tools.Registry, id int64, approve bool) {
	t.Helper()

	invocation, err := db.Invocation(id)
	if err != nil || invocation == nil {
		t.Fatalf("no such invocation: %v %v", invocation, err)
	}

	decision := store.InvocationDenied
	if approve {
		decision = store.InvocationApproved
	}

	if _, err := db.DecideInvocation(id, decision); err != nil {
		t.Fatal(err)
	}

	invocation.Status = decision

	if approve {
		tool, ok := registry.Get(invocation.Tool)
		if !ok {
			t.Fatalf("no such tool: %s", invocation.Tool)
		}

		out, execErr := tool.Execute(context.Background(), json.RawMessage(invocation.Arguments))

		status, result := store.InvocationDone, out
		if execErr != nil {
			status, result = store.InvocationFailed, execErr.Error()
		}

		if err := db.CompleteInvocation(id, status, result); err != nil {
			t.Fatal(err)
		}

		invocation.Status, invocation.Result = status, result
	}

	if err := c.Resolved(context.Background(), *invocation); err != nil {
		t.Fatal(err)
	}
}

/*
 * Approving what a task was waiting for gets it going again, and it can see
 * what happened.
 *
 * The second half of that is the part that was missing everywhere: deciding
 * ran the tool and wrote the output onto a row no model ever read, so the
 * thing somebody approved happened and the assistant never found out.
 */
func TestApprovingCarriesATaskOn(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "summary.txt")

	registry := tools.NewRegistry(tools.WriteFile{}, tools.ListDirectory{})

	model := &scripted{
		plan: `{"name":"A job","steps":[` +
			`{"do":"write the summary","kind":"write","changes":true},` +
			`{"do":"check the folder","done_when":"the file is listed","kind":"look"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write_file",
				Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"the summary"}`)}}},
			{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}},
			{Content: "The summary is there."},
		},
		checks: []string{`{"met":true,"evidence":"summary.txt","why":"the listing has it"}`},
	}

	c, db, spoken := newConductor(t, model, tools.WriteFile{}, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write a summary and then check it", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	parked := settled(t, db, task.ID)

	if parked.State != store.TaskWaiting {
		t.Fatalf("the task is %q, want waiting", parked.State)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file was written before anybody approved it")
	}

	pending, _ := db.PendingInvocations()

	if len(pending) != 1 {
		t.Fatalf("%d decisions waiting", len(pending))
	}

	decide(t, c, db, registry, pending[0].ID, true)

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("after approving, the task is %q because %q", done.State, done.Because)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("the approved action never happened: %v", err)
	}

	steps, _ := db.Steps(task.ID)

	// The approved action's own output is that step's evidence, which is what
	// lets the task know it happened rather than doing it again.
	if !strings.Contains(steps[0].Evidence, "summary.txt") {
		t.Errorf("the approved action's result is not the step's evidence: %q", steps[0].Evidence)
	}

	if steps[1].State != store.StepDone {
		t.Errorf("the step after the approval is %q", steps[1].State)
	}

	if !strings.Contains(spoken.whole(), "Finished") {
		t.Errorf("it never reported finishing: %q", spoken.whole())
	}
}

/*
 * Saying no is carried forward in your own words.
 *
 * The step is skipped rather than failed, and the refusal goes into the brief
 * every later step gets — so the model works around it rather than proposing
 * the same thing three steps later and being refused again.
 */
func TestRefusingIsCarriedForward(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "summary.txt")

	registry := tools.NewRegistry(tools.WriteFile{}, tools.ListDirectory{})

	model := &scripted{
		plan: `{"name":"A job","steps":[` +
			`{"do":"write the summary","kind":"write","changes":true},` +
			`{"do":"tell me what happened","kind":"write"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write_file",
				Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"x"}`)}}},
			{Content: "I could not write it, so here is what I have."},
		},
	}

	c, db, _ := newConductor(t, model, tools.WriteFile{}, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write a summary and then tell me", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	pending, _ := db.PendingInvocations()

	if len(pending) != 1 {
		t.Fatalf("%d decisions waiting", len(pending))
	}

	decide(t, c, db, registry, pending[0].ID, false)

	done := settled(t, db, task.ID)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file was written after being refused")
	}

	steps, _ := db.Steps(task.ID)

	if steps[0].State != store.StepSkipped {
		t.Errorf("a refused step is %q, want skipped", steps[0].State)
	}

	if !strings.Contains(steps[0].Why, "you said no") {
		t.Errorf("the refusal is not recorded in the owner's terms: %q", steps[0].Why)
	}

	// And the step after it was told.
	var told bool

	for _, req := range model.asked {
		if planning(req) || checking(req) {
			continue
		}

		for _, m := range req.Messages {
			if strings.Contains(m.Content, "you said no") {
				told = true
			}
		}
	}

	if !told {
		t.Error("the rest of the job was never told the action had been refused")
	}

	if done.State != store.TaskDone {
		t.Errorf("the task ended %q — a refusal is not a failure", done.State)
	}
}

/*
 * A task interrupted by the program closing is parked, and can be picked up.
 *
 * Not resumed on its own. Something halfway through changing files should not
 * carry on the moment somebody opens their assistant, before they have seen
 * that it is there.
 */
func TestAnInterruptedTaskWaitsToBePickedUp(t *testing.T) {
	dir := t.TempDir()

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look in the folder","kind":"look"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}},
			{Content: "Nothing in it."},
		},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	work, err := db.NewTaskThread("A job")
	if err != nil {
		t.Fatal(err)
	}

	// A task exactly as it would have been left by the program closing
	// mid-step: written down, working, with its plan and its workspace.
	id, err := db.NewTask(store.Task{
		Name: "A job", Goal: "look in the folder", State: store.TaskWorking,
		ConversationID: conv, WorkConversationID: work, Provider: "scripted",
		StepsLeft: 12, CallsLeft: 40, ReplansLeft: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AddSteps(id, []store.TaskStep{
		{Instruction: "look in the folder", Kind: store.StepLook},
	}); err != nil {
		t.Fatal(err)
	}

	// What the program does when it opens and finds this.
	n, err := db.InterruptWorkingTasks("PN Scripts Assistant was closed while this was running")
	if err != nil || n != 1 {
		t.Fatalf("parked %d tasks: %v", n, err)
	}

	parked, _ := db.Task(id)

	if parked.State != store.TaskWaiting || parked.Because == "" {
		t.Fatalf("the task is %q because %q", parked.State, parked.Because)
	}

	// And it picks up where it left off when asked.
	if err := c.Resume(id); err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, id)

	if done.State != store.TaskDone {
		t.Errorf("after resuming, the task is %q because %q", done.State, done.Because)
	}
}
