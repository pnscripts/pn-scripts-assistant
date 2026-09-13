package tasks

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/jobs"
	"pn-scripts-assistant/internal/brain/lanes"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * busy is a model that takes a moment over everything and remembers the most
 * steps it was answering at once.
 *
 * Safe to call from several goroutines, which the scripted model is not —
 * the point here is precisely that calls arrive together.
 */
type busy struct {
	name string
	plan string

	mu        sync.Mutex
	now, most int
}

func (b *busy) Name() string                   { return b.name }
func (b *busy) Available(context.Context) bool { return true }

func (b *busy) Chat(_ context.Context, req llm.Request) (llm.Response, error) {
	if planning(req) {
		return llm.Response{Content: b.plan}, nil
	}

	b.mu.Lock()
	b.now++

	if b.now > b.most {
		b.most = b.now
	}

	b.mu.Unlock()

	time.Sleep(60 * time.Millisecond)

	b.mu.Lock()
	b.now--
	b.mu.Unlock()

	return llm.Response{Content: "Written."}, nil
}

const twoAtOnce = `{"name":"Launch","steps":[
	{"do":"Write the announcement","kind":"write","who":"writer"},
	{"do":"Write the release notes","kind":"write","who":"editor","with_previous":true}]}`

func runTwoAtOnce(t *testing.T, provider string) (*busy, *store.Task, *store.DB) {
	t.Helper()

	model := &busy{name: provider, plan: twoAtOnce}

	c, db, _ := newConductor(t, model)

	c.Lanes = lanes.New(3)
	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "assistant", Title: "Assistant"},
			{Name: "writer", Title: "Writer", For: "announcements"},
			{Name: "editor", Title: "Editor", For: "release notes"},
		}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "launch it", provider, true)
	if err != nil {
		t.Fatal(err)
	}

	return model, settled(t, db, task.ID), db
}

// Two steps that stand alone, on somebody else's computer, are done at the
// same time by the two people they belong to.
func TestStandaloneStepsOverlapOnAHostedService(t *testing.T) {
	model, done, db := runTwoAtOnce(t, "anthropic")

	if done.State != store.TaskDone {
		t.Fatalf("the task ended %s: %s", done.State, done.Because)
	}

	if model.most != 2 {
		t.Errorf("at most %d steps were being answered at once, want 2", model.most)
	}

	steps, _ := db.Steps(done.ID)

	if !steps[1].Together || steps[0].Together {
		t.Errorf("the plan did not keep which step stands beside which: %+v", steps)
	}
}

// The same two steps on this machine's model take their turn, because two
// calls on a processor are slower than one after the other.
func TestStandaloneStepsTakeTurnsOnThisMachine(t *testing.T) {
	model, done, _ := runTwoAtOnce(t, llm.Local)

	if done.State != store.TaskDone {
		t.Fatalf("the task ended %s: %s", done.State, done.Because)
	}

	if model.most != 1 {
		t.Errorf("%d local calls ran at once, want 1", model.most)
	}
}

/*
 * A machine that allows one background job still lets a task hand a step on.
 *
 * The task's own job is running when it starts its specialist's, and counted
 * against the same limit the specialist was refused — leaving the step waiting
 * for somebody to press a button. The lanes decide the pace of the thinking
 * now, so neither is counted there.
 */
func TestAMachineAllowingOneJobStillHandsWorkOn(t *testing.T) {
	runner := &jobs.Runner{AtMost: 1}

	first, err := runner.StartQueued("a task", func(ctx context.Context) (string, error) {
		time.Sleep(50 * time.Millisecond)

		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runner.StartQueued("its specialist", func(context.Context) (string, error) {
		return "", nil
	}); err != nil {
		t.Fatalf("a specialist's task was refused while its parent ran: %v", err)
	}

	// Ordinary background work is still held to the machine's limit.
	if _, err := runner.Start("reading a folder", func(context.Context) (string, error) {
		return "", nil
	}); err != nil {
		t.Fatalf("queued work counted against the ordinary limit: %v", err)
	}

	if _, err := runner.Start("and another", func(context.Context) (string, error) {
		time.Sleep(time.Millisecond)

		return "", nil
	}); err == nil || !strings.Contains(err.Error(), "already doing") {
		t.Errorf("ordinary background work went past its limit")
	}

	_ = first
}
