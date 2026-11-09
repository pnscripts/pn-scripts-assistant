package tasks

import (
	"context"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

// breaks plans normally and then fails in the middle of the work, the way a
// bug or a database that stopped answering would.
type breaks struct{ scripted }

func (b *breaks) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	if planning(req) {
		return b.scripted.Chat(ctx, req)
	}

	panic("the work broke")
}

/*
 * Work that fails does not stay "working" for ever.
 *
 * The job underneath recorded the failure, but the task's own row still said
 * working: the page showed it busy with nothing behind it, nobody was told,
 * and it was taken up again only at the next start. Seen as a test that, under
 * a full parallel run, now and then waited twenty seconds for a task that had
 * long since stopped.
 */
func TestWorkThatFailsIsNotLeftWorking(t *testing.T) {
	c, db, spoken := newConductor(t, &breaks{})

	conv, _ := db.NewConversation("t")

	task, started, err := c.Take(context.Background(), conv, "do the thing", "scripted", true)
	if err != nil || !started {
		t.Fatalf("the work did not start: %v %v", started, err)
	}

	got := settled(t, db, task.ID)

	if got.State != store.TaskBlocked {
		t.Fatalf("work that broke ended %q, want blocked", got.State)
	}

	if !strings.Contains(got.Because, "the work broke") {
		t.Errorf("it does not say why: %q", got.Because)
	}

	if !strings.Contains(spoken.whole(), "the work broke") {
		t.Errorf("the conversation that asked was not told: %q", spoken.whole())
	}
}

// A job somebody stopped is stopped, not blocked by an error.
func TestStoppedWorkIsStillStopped(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "Nothing to report."}}}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "do the thing", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Stop(task.ID); err != nil {
		t.Fatal(err)
	}

	if got := settled(t, db, task.ID); got.State != store.TaskStopped && got.State != store.TaskDone {
		t.Errorf("a stopped job ended %q", got.State)
	}
}
