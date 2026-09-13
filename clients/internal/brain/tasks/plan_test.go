package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * A plan is read out of whatever the model wrote around it.
 *
 * A small model asked for JSON writes a sentence first, or fences it, or wraps
 * its working in think tags and puts the object after — the same habits the
 * tool-call recovery already deals with, which is why this uses the same brace
 * counter rather than a second one.
 */
func TestAPlanIsReadOutOfWhateverIsAroundIt(t *testing.T) {
	for _, c := range []struct {
		what  string
		reply string
	}{
		{"bare", `{"name":"Tidy up","steps":[{"do":"one","kind":"look"},{"do":"two","kind":"do"}]}`},
		{"fenced", "Here is the plan:\n```json\n" +
			`{"name":"Tidy up","steps":[{"do":"one","kind":"look"},{"do":"two","kind":"do"}]}` +
			"\n```"},
		{"after thinking", "<think>I should break this in two</think>\n" +
			`{"name":"Tidy up","steps":[{"do":"one","kind":"look"},{"do":"two","kind":"do"}]}`},
	} {
		plan, ok := readPlan(c.reply, 12)

		if !ok {
			t.Errorf("%s: no plan was found", c.what)

			continue
		}

		if plan.Name != "Tidy up" || len(plan.Steps) != 2 {
			t.Errorf("%s: read %+v", c.what, plan)
		}

		if plan.Steps[0].Kind != store.StepLook {
			t.Errorf("%s: the kind was lost: %q", c.what, plan.Steps[0].Kind)
		}
	}
}

// A kind the model invented becomes "do". It chooses the model and narrows the
// tools, and it is what a named agent will be routed by — none of which
// survives a sixth value appearing.
func TestAnInventedKindBecomesDo(t *testing.T) {
	plan, ok := readPlan(`{"steps":[{"do":"one","kind":"deploy"},{"do":"two"}]}`, 12)
	if !ok {
		t.Fatal("no plan was found")
	}

	for i, s := range plan.Steps {
		if s.Kind != store.StepDo {
			t.Errorf("step %d has kind %q", i, s.Kind)
		}
	}
}

// Steps past the allowance are dropped, not obeyed. A model that answers with
// thirty steps has misunderstood the job, and the budget is not a suggestion
// it gets to argue with.
func TestStepsPastTheAllowanceAreDropped(t *testing.T) {
	var b strings.Builder

	b.WriteString(`{"steps":[`)

	for i := 0; i < 30; i++ {
		if i > 0 {
			b.WriteString(",")
		}

		b.WriteString(`{"do":"something"}`)
	}

	b.WriteString(`]}`)

	plan, ok := readPlan(b.String(), 12)
	if !ok {
		t.Fatal("no plan was found")
	}

	if len(plan.Steps) != 12 {
		t.Errorf("kept %d steps, want 12", len(plan.Steps))
	}
}

// Prose is not a plan, and is not retried. A model that answers a request for
// JSON with a sentence will not be talked round by asking twice, and each
// attempt is a whole call on a processor where that is most of a minute.
func TestProseIsNotAPlan(t *testing.T) {
	for _, reply := range []string{
		"I'll go through your projects and let you know what I find.",
		"",
		`{"name":"Nothing","steps":[]}`,
	} {
		if _, ok := readPlan(reply, 12); ok {
			t.Errorf("this was read as a plan: %q", reply)
		}
	}
}

/*
 * A question is answered, not planned.
 *
 * The cost of getting this wrong in one direction is one short model call and
 * the request being answered normally. In the other it is a panel of work with
 * "what time is it" in it, on a machine where planning is most of a minute.
 */
func TestAQuestionDoesNotBecomeAJob(t *testing.T) {
	for _, question := range []string{
		"what time is it",
		"hello",
		"what is the weather like today",
		"list my drives",
		"колко е часът",
	} {
		if shape, yes := llm.LooksLikeAJob(question); yes {
			t.Errorf("%q was taken for a job, on %q", question, shape)
		}
	}
}

// And work is recognised by its shape rather than its subject: several things
// joined, a sweep over a set, or a stated end.
func TestWorkIsRecognisedByItsShape(t *testing.T) {
	for _, job := range []string{
		"go through the Godot project in ~/Games and tell me what is broken",
		"read each of my invoices and then write me a summary",
		"look at all my projects and work out which have no readme",
		"мини през проектите ми и ми кажи кои са без документация",
	} {
		if _, yes := llm.LooksLikeAJob(job); !yes {
			t.Errorf("%q was not recognised as work", job)
		}
	}
}

/*
 * A one-step plan is a task only when somebody said so outright.
 *
 * The guess and the instruction are treated differently on purpose: a guess
 * that small should be dropped and the turn answered normally, and an
 * instruction should not be argued with.
 */
func TestAOneStepPlanIsOnlyATaskWhenAskedFor(t *testing.T) {
	model := &scripted{plan: `{"name":"One thing","steps":[{"do":"have a look","kind":"look"}]}`}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	_, started, err := c.Take(context.Background(), conv, "go through my projects and see", "scripted", false)
	if err != nil {
		t.Fatal(err)
	}

	if started {
		t.Error("a one-step plan became a task nobody asked for")
	}

	tasks, _ := db.Tasks(nil, 10)

	if len(tasks) != 0 {
		t.Errorf("%d task rows were written for something that is not a task", len(tasks))
	}

	// Asked for outright, the same plan is a task.
	if _, started, err = c.Take(context.Background(), conv, "have a look", "scripted", true); err != nil {
		t.Fatal(err)
	}

	if !started {
		t.Error("asking for a task outright did not produce one")
	}
}

// A plan of several steps is written down in order before any of it runs, so
// it can be read — and stopped — first.
func TestAPlanIsWrittenDownBeforeAnyOfItRuns(t *testing.T) {
	model := &scripted{plan: `{"name":"Going through your projects",
		"done_when":"every project has been looked at",
		"steps":[
			{"do":"list the folders in ~/Projects","done_when":"a list of folder names","kind":"look"},
			{"do":"read each project's readme","done_when":"what each project is for","kind":"look"},
			{"do":"write what is missing","done_when":"a summary naming each project","kind":"write"}]}`}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, started, err := c.Take(context.Background(), conv,
		"go through my projects and tell me what is broken", "scripted", false)
	if err != nil || !started {
		t.Fatalf("the task did not start: %v %v", started, err)
	}

	if task.Name != "Going through your projects" {
		t.Errorf("the task is called %q", task.Name)
	}

	if task.DoneWhen == "" {
		t.Error("nothing says when the whole job is finished")
	}

	if len(task.Steps) != 3 {
		t.Fatalf("%d steps were recorded", len(task.Steps))
	}

	for i, s := range task.Steps {
		if s.Position != i+1 {
			t.Errorf("step %d is at position %d", i, s.Position)
		}

		if s.DoneWhen == "" {
			t.Errorf("step %d has nothing to check it against", i+1)
		}
	}

	if task.Steps[2].Kind != store.StepWrite {
		t.Errorf("the last step's kind is %q", task.Steps[2].Kind)
	}
}

/*
 * A task that is running can be stopped.
 *
 * The model here blocks until the test lets it go, so this is never a race
 * with a task that finished on its own before Stop was called — which is
 * exactly the shape of test that passes alone and fails in company.
 */
func TestARunningTaskCanBeStopped(t *testing.T) {
	held := make(chan struct{})
	reached := make(chan struct{}, 1)

	model := &held3{hold: held, reached: reached,
		plan: `{"name":"A long job","steps":[{"do":"one"},{"do":"two"}]}`}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, started, err := c.Take(context.Background(), conv, "one and then two", "scripted", true)
	if err != nil || !started {
		t.Fatalf("the task did not start: %v %v", started, err)
	}

	// Wait until it is genuinely mid-step.
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the task never reached its first step")
	}

	if err := c.Stop(task.ID); err != nil {
		t.Fatal(err)
	}

	close(held)

	stopped, _ := db.Task(task.ID)

	if stopped.State != store.TaskStopped {
		t.Errorf("after stopping, the task is %q", stopped.State)
	}

	if stopped.Because == "" {
		t.Error("it does not say why it stopped")
	}
}

// held3 answers planning at once and then blocks in the middle of the work, so
// a test can be certain a task is running when it acts on it.
type held3 struct {
	plan    string
	hold    chan struct{}
	reached chan struct{}
}

func (h *held3) Name() string                   { return "scripted" }
func (h *held3) Available(context.Context) bool { return true }

func (h *held3) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	if planning(req) {
		return llm.Response{Content: h.plan}, nil
	}

	select {
	case h.reached <- struct{}{}:
	default:
	}

	select {
	case <-h.hold:
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}

	return llm.Response{Content: "done"}, nil
}
