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
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Two agents in the same job remember different things.
 *
 * The distinction the whole organisation rests on, checked where it matters:
 * what a job knows is shared; what this one found is this one's. And it comes
 * back — a later step by the same agent is reminded, and a step by the other
 * is not.
 */
func TestTwoAgentsInOneJobRememberDifferentThings(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "orders.sql"), []byte("select"), 0o600)

	look := llm.Response{ToolCalls: []llm.ToolCall{{
		ID: "c1", Name: "list_directory",
		Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`),
	}}}

	model := &scripted{
		plan:    `{"name":"Queries","steps":[{"do":"Find the slow orders queries","kind":"look","who":"alex"}]}`,
		replies: []llm.Response{look, {Content: "The orders queries scan without an index."}},
	}

	c, db, _ := newConductor(t, model, tools.ListDirectory{})

	c.Roster = func() []team.Agent {
		return []team.Agent{
			{Name: "alex", Title: "Alex", For: "backend databases", Job: "software.backend_engineer"},
			{Name: "maria", Title: "Maria", For: "backend databases", Job: "software.backend_engineer"},
		}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "slow queries", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	alex, _ := db.Memories("alex", 10)
	maria, _ := db.Memories("maria", 10)

	if len(alex) != 1 || !strings.Contains(alex[0].Content, "without an index") || len(maria) != 0 {
		t.Fatalf("alex remembers %+v and maria %+v", alex, maria)
	}

	// A later step about the same thing: alex is reminded, maria is not.
	step := &store.TaskStep{Instruction: "Fix the slow orders queries"}

	if said := c.recalled(team.Agent{Name: "alex"}, step); !strings.Contains(said, "without an index") {
		t.Errorf("alex was not reminded of what alex found: %q", said)
	}

	if said := c.recalled(team.Agent{Name: "maria"}, step); said != "" {
		t.Errorf("maria was reminded of something maria never did: %q", said)
	}

	// Nothing unrelated comes back.
	if said := c.recalled(team.Agent{Name: "alex"}, &store.TaskStep{Instruction: "Write the newsletter"}); said != "" {
		t.Errorf("an unrelated step was reminded of the queries: %q", said)
	}
}

/*
 * What a review found wrong is remembered by whoever did the work, as a lesson
 * — and lessons come before findings.
 */
func TestAWrongReviewBecomesALesson(t *testing.T) {
	dir := t.TempDir()

	model := &scripted{
		plan: `{"name":"Release","steps":[{"do":"Deploy the build to production","kind":"do","who":"assistant"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_directory",
				Arguments: json.RawMessage(`{"path":` + asJSON(dir) + `}`)}}},
			{Content: "Deployed."},
			{Content: "Wrong: that folder is the staging build."},
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

	task, _, _ := c.Take(context.Background(), conv, "release it", "scripted", true)
	settled(t, db, task.ID)

	lessons, _ := db.Recall("assistant", "deploy the production build", 3)

	if len(lessons) == 0 || lessons[0].Kind != store.Learned || !strings.Contains(lessons[0].Content, "staging") {
		t.Fatalf("the review's finding was not remembered as a lesson first: %+v", lessons)
	}

	// And the record counts it against the one whose work it was.
	review, _ := db.HowItHasBeenGoing(task.CreatedAt.AddDate(0, 0, -1))

	for _, person := range review.People {
		if person.Name == "assistant" && person.FoundWrong != 1 {
			t.Errorf("the record does not count the step found wrong: %+v", person)
		}
	}
}

// The job's own knowledge goes into the brief: what it involves, not only
// what it is called.
func TestABriefCarriesTheJobsKnowledge(t *testing.T) {
	said := knowledgeFor(team.Fit{Job: &store.Occupation{
		Title: "Security engineer", Does: []string{"Find what could be attacked"},
		Knows: []string{"threat models"}, MeasuredBy: []string{"holes closed"},
	}})

	for _, want := range []string{"security engineer", "Find what could be attacked", "threat models", "holes closed"} {
		if !strings.Contains(said, want) {
			t.Errorf("the brief does not say %q: %s", want, said)
		}
	}

	if knowledgeFor(team.Fit{}) != "" {
		t.Error("an agent with no job was given somebody's knowledge")
	}
}
