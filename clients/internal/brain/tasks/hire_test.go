package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

// hiring gives a conductor a real brain root, the real job catalogue, and
// the real hiring and dissolving — the organisation as the brain wires it.
func hiring(t *testing.T, c *Conductor, db *store.DB) string {
	t.Helper()

	root := t.TempDir()

	if _, err := occupations.Ensure(db); err != nil {
		t.Fatal(err)
	}

	team.Forget()

	c.Occupations = db
	c.Roster = func() []team.Agent { return team.Roster(root) }
	c.Chart = func() []org.Unit { return org.Chart(root) }

	c.Hire = func(w team.Wish) (team.Hired, error) {
		return team.Hire(root, team.Roster(root), org.Chart(root), db, nil, team.Templates(root), w)
	}

	c.Dissolve = func(task int64) ([]team.Agent, error) {
		return team.Dissolve(root, team.Roster(root), task)
	}

	return root
}

/*
 * The planner names a role nobody holds, and somebody is hired into it.
 *
 * The specification's "build me a SaaS" in small: a product manager is
 * needed, there is none, the catalogue has the job. They are hired for this
 * job only, do the step, are named in the account, and are gone at the end.
 */
func TestARoleThePlannerNamesIsHiredForTheJob(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"Requirements","steps":[{"do":"Write the product requirements","kind":"write","who":"product_manager"}]}`,
		replies: []llm.Response{{Content: "Requirements: sign-up, billing, a dashboard."}},
	}

	c, db, _ := newConductor(t, model)
	root := hiring(t, c, db)

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write the requirements", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	hiredName := task.Steps[0].Assignee

	if hiredName == "product_manager" || hiredName == "" {
		t.Fatalf("the step was not given to somebody real: %q", hiredName)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("the task ended %s: %s", done.State, done.Because)
	}

	for _, want := range []string{"Hired for this job", "Product manager", "Let go now that it is finished"} {
		if !strings.Contains(done.Report, want) {
			t.Errorf("the account does not say %q:\n%s", want, done.Report)
		}
	}

	if _, still := team.Find(team.Roster(root), hiredName); still {
		t.Error("the temporary hire is still on the roster after the job")
	}

	steps, _ := db.Steps(task.ID)

	if steps[0].Assignee != hiredName {
		t.Errorf("the record of who did the step was lost when they were let go: %q", steps[0].Assignee)
	}
}

/*
 * Nobody here could do a step, so somebody who could is hired, handed it, and
 * let go with the job.
 */
func TestAStepNobodyHereCouldDoHiresASpecialist(t *testing.T) {
	dir := t.TempDir()

	model := &scripted{
		plan: `{"name":"Slow","steps":[{"do":"Tune the postgresql performance","kind":"look","who":"assistant"}]}`,
		replies: []llm.Response{
			{Content: "No idea."},
			{Content: "Still no idea."},
			{ToolCalls: []llm.ToolCall{{
				ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
			}}},
			{Content: "The orders table needs an index."},
		},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})
	root := hiring(t, c, db)

	// Nobody but the generalist and whoever gets hired: the six that ship
	// include people who could have been handed this, and the point is that
	// nobody here could.
	c.Roster = func() []team.Agent {
		out := []team.Agent{{Name: "assistant", Title: "Assistant", For: "anything at all"}}

		for _, a := range team.Roster(root) {
			if !a.BuiltIn {
				out = append(out, a)
			}
		}

		return out
	}

	c.Hire = func(w team.Wish) (team.Hired, error) {
		return team.Hire(root, c.Roster(), org.Chart(root), db, nil, team.Templates(root), w)
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "why is it slow", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := finished(t, db, task.ID)

	if done.State != store.TaskDone {
		t.Fatalf("the task ended %s: %s", done.State, done.Because)
	}

	children, _ := db.Children(task.ID)

	if len(children) != 1 {
		t.Fatalf("%d specialists' tasks, want 1", len(children))
	}

	if !strings.Contains(done.Report, "Hired for this job") {
		t.Errorf("the account does not name the hire:\n%s", done.Report)
	}

	for _, a := range team.Roster(root) {
		if a.State == team.Temporary {
			t.Errorf("%s is still on the roster after the job", a.Name)
		}
	}
}
