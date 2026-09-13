package tasks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * A check cannot agree unless it can quote something that is really there.
 *
 * This is the whole trick, and the reason the checking is worth anything at
 * all. A model asked whether something worked will say yes; asking it more
 * sternly does not help, because agreeableness is not a wording problem. But a
 * model cannot agree its way past a rule that requires the agreement to be
 * quoted from a transcript it did not write — so a checker too small to check
 * produces a phrase that is not in the output, and is caught by a string
 * search rather than by judgement.
 */
func TestACheckCannotAgreeWithoutQuotingSomethingReal(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look in the folder",` +
			`"done_when":"a list of what is in it","kind":"look"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}},
			{Content: "It has one file in it."},
		},
		// A check that agrees, quoting something that was never returned.
		checks: []string{`{"met":true,"evidence":"everything looks correct","why":"it worked"}`},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})
	c.Budget = Budget{MostSteps: 12, MostAttempts: 1, MostReplans: 0, MostCalls: 40, HowLong: time.Minute}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look in "+dir+" and tell me", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	steps, _ := db.Steps(task.ID)

	if steps[0].Verdict != store.Unmet {
		t.Errorf("verdict %q — a check agreed with evidence that was not there", steps[0].Verdict)
	}

	/*
	 * And it says so in a way its owner can act on.
	 *
	 * This sentence tells somebody the model doing the checking on their
	 * machine is not up to it, which is a fact about their machine they cannot
	 * learn any other way.
	 */
	if !strings.Contains(steps[0].Why, "not in what the tools returned") {
		t.Errorf("it does not say why the check was rejected: %q", steps[0].Why)
	}
}

// And a check that quotes what was really returned is accepted, and says which
// model did the checking.
func TestACheckThatQuotesWhatIsThereIsAccepted(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "invoice.pdf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look in the folder",` +
			`"done_when":"a list of what is in it","kind":"look"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}},
			{Content: "There is one invoice."},
		},
		checks: []string{`{"met":true,"evidence":"invoice.pdf","why":"the listing has it"}`},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look in "+dir+" and tell me", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)
	steps, _ := db.Steps(task.ID)

	if steps[0].Verdict != store.Verified {
		t.Fatalf("verdict %q, want verified: %q", steps[0].Verdict, steps[0].Why)
	}

	if steps[0].CheckedBy == "" {
		t.Error("the step does not record which model checked it")
	}

	if done.State != store.TaskDone {
		t.Errorf("the task ended %q", done.State)
	}
}

/*
 * A step that fails its check is tried again — once — and told what was wrong.
 *
 * A third attempt is not offered. A model that has failed the same step twice
 * is not one attempt away, and each attempt is minutes on this machine.
 */
func TestAFailedStepIsTriedAgainOnce(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "ledger.csv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	call := llm.Response{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
		Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}}

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look in the folder",` +
			`"done_when":"a list of what is in it","kind":"look"}]}`,
		replies: []llm.Response{
			call, {Content: "First attempt."},
			call, {Content: "Second attempt."},
		},
		checks: []string{
			`{"met":false,"why":"the listing does not show what was asked for."}`,
			`{"met":true,"evidence":"ledger.csv","why":"it is there now"}`,
		},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})
	c.Budget = Budget{MostSteps: 12, MostAttempts: 2, MostReplans: 0, MostCalls: 40, HowLong: time.Minute}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look in "+dir+" and tell me", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)
	steps, _ := db.Steps(task.ID)

	if steps[0].Attempts != 2 {
		t.Errorf("the step was attempted %d times, want 2", steps[0].Attempts)
	}

	if steps[0].State != store.StepDone {
		t.Errorf("after its second attempt the step is %q: %q", steps[0].State, steps[0].Why)
	}

	if done.State != store.TaskDone {
		t.Errorf("the task ended %q", done.State)
	}
}

// The second attempt is told what was wrong with the first, rather than being
// left to make the same mistake again.
func TestTheSecondAttemptIsToldWhatWasWrong(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "ledger.csv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	call := llm.Response{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
		Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}}

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look in the folder",` +
			`"done_when":"a list of what is in it","kind":"look"}]}`,
		replies: []llm.Response{call, {Content: "First."}, call, {Content: "Second."}},
		checks: []string{
			`{"met":false,"why":"you listed the wrong folder entirely."}`,
			`{"met":true,"evidence":"ledger.csv","why":"right this time"}`,
		},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look in "+dir, "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	var told bool

	for _, req := range model.asked {
		if planning(req) || checking(req) {
			continue
		}

		for _, m := range req.Messages {
			if strings.Contains(m.Content, "wrong folder entirely") {
				told = true
			}
		}
	}

	if !told {
		t.Error("the second attempt was never told what was wrong with the first")
	}
}

// Evidence is matched on what it says, not on how it was spaced. A model
// copying a phrase out of a listing will not reproduce the whitespace, and a
// rule that fails over two spaces is not strictness, it is noise.
func TestEvidenceIsMatchedOnWhatItSaysNotItsSpacing(t *testing.T) {
	output := "list_directory /home/petar\nDocuments   Games\n  Projects\n"

	for _, c := range []struct {
		evidence string
		want     bool
	}{
		{"Documents Games", true},
		{"documents   games", true},
		{"\n Projects \n", true},
		{"Invoices", false},
		{"everything looks correct", false},
		{"ok", false}, // too short to mean anything
	} {
		if got := quoted(c.evidence, output); got != c.want {
			t.Errorf("quoted(%q) = %v, want %v", c.evidence, got, c.want)
		}
	}
}
