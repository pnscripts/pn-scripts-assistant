package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * A plan says how serious each step is before any of it runs.
 *
 * Before, because that is the only moment it is worth knowing: while the task
 * can still be read and stopped. And a look is never more than medium on its
 * words — finding out how a tax return is filed cannot file one.
 */
func TestAPlanIsWeighedBeforeItRuns(t *testing.T) {
	model := &scripted{plan: `{"name":"The bills","steps":[
		{"do":"Find out how to file the tax return","kind":"look"},
		{"do":"Pay the electricity invoice","kind":"do"},
		{"do":"Draft a short note about it","kind":"write"}]}`}

	c, db, _ := newConductor(t, model)

	conv, _ := db.NewConversation("t")

	task, started, err := c.Take(context.Background(), conv, "sort out the bills", "scripted", true)
	if err != nil || !started {
		t.Fatalf("the task did not start: %v %v", started, err)
	}

	want := []risk.Level{risk.Medium, risk.Critical, risk.Low}

	for i, s := range task.Steps {
		if risk.Parse(s.Risk) != want[i] {
			t.Errorf("step %d, %q, is %q — want %s", i+1, s.Instruction, s.Risk, want[i])
		}
	}

	if risk.Parse(task.Risk) != risk.Critical {
		t.Errorf("the task is %q, want the most serious of its steps", task.Risk)
	}

	settled(t, db, task.ID)
}

// oversightJobs is a taxonomy of one job, marked the way bookkeeping is.
type oversightJobs struct{}

func (oversightJobs) Occupation(id string) (*store.Occupation, error) {
	if id == "finance.bookkeeper" {
		return &store.Occupation{ID: id, Risk: store.RiskHigh, Oversight: true}, nil
	}

	return nil, fmt.Errorf("no such job")
}

/*
 * Who is doing it matters as much as what it says.
 *
 * "Enter the receipts" is nothing much in anybody's words. Done by a job the
 * taxonomy marks for human oversight, it is what that mark is for — the cost
 * of a wrong entry there is not a wrong answer but a wrong filing.
 */
func TestAJobMarkedForOversightMakesItsActionsCritical(t *testing.T) {
	c, _, _ := newConductor(t, &scripted{})

	c.Occupations = oversightJobs{}

	keeper := team.Agent{Name: "bookkeeper", Title: "Bookkeeper", Job: "finance.bookkeeper"}

	cases := map[string]risk.Level{
		store.StepDo:    risk.Critical,
		store.StepWrite: risk.High,
		store.StepLook:  risk.Low,
	}

	for kind, want := range cases {
		got := c.weigh(store.TaskStep{Instruction: "Enter the receipts", Kind: kind}, keeper)

		if got != want {
			t.Errorf("a %s step by the bookkeeper is %s, want %s", kind, got, want)
		}
	}

	researcher := team.Agent{Name: "researcher"}

	if got := c.weigh(store.TaskStep{Instruction: "Enter the receipts", Kind: store.StepDo}, researcher); got != risk.Low {
		t.Errorf("the same step by somebody with no such job is %s, want low", got)
	}
}

// gatedBy is the brain's own gate: a real permits book at a chosen freedom.
func gatedBy(book *permits.Book, freedom permits.Freedom) func(string, string, bool, risk.Level) permits.Answer {
	return func(who, tool string, changes bool, level risk.Level) permits.Answer {
		return book.DecideAt(who, tool, changes, freedom, level)
	}
}

/*
 * A critical step asks, even through a standing grant.
 *
 * "You may write files" was said about notes and drafts. A step about paying
 * somebody is a different question, and it is put — with the level in the
 * words of the question, so it is not read as one more routine approval.
 */
func TestACriticalStepAsksThroughAGrant(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "payment.csv")

	model := &scripted{
		plan: `{"name":"Pay","steps":[{"do":"Pay the supplier by writing the payment file","kind":"do"}]}`,
		replies: []llm.Response{{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: "write_file",
			Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"400"}`),
		}}}},
	}

	c, db, _ := newConductor(t, model, tools.WriteFile{})

	book, _ := permits.Load(t.TempDir())
	book.Remember("write_file", permits.Allow, "notes and drafts")

	c.Agent.MayI = gatedBy(book, permits.WhatIveAllowed)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "pay the supplier", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskWaiting {
		t.Fatalf("a critical step did not stop: %s (%s)", done.State, done.Because)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the payment file was written on a grant for writing notes")
	}

	waiting, _ := db.PendingInvocations()

	if len(waiting) != 1 || !strings.HasPrefix(waiting[0].Summary, "Critical risk — ") {
		t.Fatalf("the question does not say how serious it is: %+v", waiting)
	}
}

/*
 * On never stop, it does not stop — and says afterwards what it did.
 *
 * Its owner chose nothing asking, including the risky things. The other half
 * of that choice is being able to see what it amounted to, so the task's
 * account ends by naming each action that would have been asked about.
 */
func TestNeverStopNamesWhatItDidWithoutAsking(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "payment.csv")

	model := &scripted{
		plan: `{"name":"Pay","steps":[{"do":"Pay the supplier by writing the payment file","kind":"do"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{
				ID: "c1", Name: "write_file",
				Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"400"}`),
			}}},
			{Content: "Written."},
		},
	}

	c, db, spoken := newConductor(t, model, tools.WriteFile{})

	book, _ := permits.Load(t.TempDir())

	c.Agent.MayI = gatedBy(book, permits.Everything)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "pay the supplier", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("it stopped on never stop: %s (%s)", done.State, done.Because)
	}

	if _, err := os.Stat(target); err != nil {
		t.Fatal("the file was not written")
	}

	steps, _ := db.Steps(task.ID)

	if !strings.Contains(steps[0].Acted, "Critical — ") {
		t.Errorf("the step does not record what ran unasked: %q", steps[0].Acted)
	}

	for _, want := range []string{"Done without asking", "Critical — ", "payment.csv"} {
		if !strings.Contains(done.Report, want) || !strings.Contains(spoken.whole(), want) {
			t.Errorf("the account of the task does not say %q:\n%s", want, done.Report)
		}
	}
}
