package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Each step is done by whoever it was given to, on that person's model.
 *
 * The whole of what a roster buys. On this machine the coding model and the
 * fastest model are minutes apart per round and the one that writes best prose
 * is neither, so a job with three steps in it should not be three turns of the
 * same compromise.
 */
func TestEachStepRunsAsWhoeverItWasGivenTo(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	model := &scripted{
		plan: `{"name":"A job","steps":[
			{"do":"read the notes","done_when":"the text","kind":"look","who":"researcher"},
			{"do":"write it up","done_when":"a summary","kind":"write","who":"writer"}]}`,
		replies: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read_file",
				Arguments: json.RawMessage(`{"path":` + asJSON(filepath.Join(dir, "notes.txt")) + `}`)}}},
			{Content: "It says hello."},
			{Content: "Here is the summary: it says hello."},
		},
		checks: []string{`{"met":true,"evidence":"hello","why":"the file says so"}`},
	}

	c, db, _ := newConductor(t, model, tools.ReadFile{}, tools.ListDirectory{})

	// Two models installed, so the roles are genuinely different.
	c.Sizes = func() llm.Sizes {
		return llm.Sizes{Work: "coder:7b", Quick: "small:3b", Best: "big:14b"}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "read the notes and write it up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	steps, _ := db.Steps(task.ID)

	if len(steps) != 2 {
		t.Fatalf("%d steps", len(steps))
	}

	if steps[0].Assignee != "researcher" {
		t.Errorf("step one was done by %q", steps[0].Assignee)
	}

	if steps[1].Assignee != "writer" {
		t.Errorf("step two was done by %q", steps[1].Assignee)
	}

	// And on different models, which is the point of them being different people.
	if steps[0].Model != "small:3b" {
		t.Errorf("the researcher used %q, want the quick model", steps[0].Model)
	}

	if steps[1].Model != "big:14b" {
		t.Errorf("the writer used %q, want the best model", steps[1].Model)
	}
}

/*
 * An agent is only offered its own tools, and refused the rest.
 *
 * Hiding a tool from the list is what stops it being reached for. This is what
 * stops it anyway — a model that saw a tool earlier in the same conversation
 * will name it again, and limits that are only a shorter menu are a convention
 * rather than a rule.
 */
func TestAnAgentIsRefusedToolsThatAreNotItsOwn(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "written.txt")

	model := &scripted{
		plan: `{"name":"A job","steps":[{"do":"look it up","done_when":"an answer","kind":"look","who":"researcher"}]}`,
		replies: []llm.Response{
			// The researcher has no write_file. Asking for it anyway is the
			// case this exists for.
			{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "write_file",
				Arguments: json.RawMessage(`{"path":` + asJSON(target) + `,"content":"x"}`)}}},
			{Content: "I could not write it."},
		},
	}

	c, db, _ := newConductor(t, model, tools.WriteFile{}, tools.ReadFile{}, tools.ListDirectory{})
	c.Budget = Budget{MostSteps: 12, MostAttempts: 1, MostReplans: 0, MostCalls: 40, HowLong: time.Minute}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look something up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a researcher wrote a file")
	}

	// It was told no, in words, rather than the turn failing.
	var told bool

	for _, req := range model.asked {
		for _, m := range req.Messages {
			if m.Role == llm.RoleTool && strings.Contains(m.Content, "not one of the tools") {
				told = true
			}
		}
	}

	if !told {
		t.Error("it was never told why the tool did not run")
	}

	// And nothing was queued for approval: it never reached the gate, because
	// it never reached the tool.
	pending, _ := db.PendingInvocations()

	if len(pending) != 0 {
		t.Errorf("%d decisions are waiting for a tool that agent may not use", len(pending))
	}
}

// Only the tools it may use are described to it at all, which is what makes a
// small model choose better rather than only more safely.
func TestAnAgentIsOnlyShownItsOwnTools(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"look it up","kind":"look","who":"researcher"}]}`,
		replies: []llm.Response{{Content: "Nothing to report."}},
	}

	c, db, _ := newConductor(t, model,
		tools.WriteFile{}, tools.ReadFile{}, tools.ListDirectory{}, tools.RunCommand{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look something up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	var offered []string

	for _, req := range model.asked {
		if planning(req) || checking(req) {
			continue
		}

		for _, spec := range req.Tools {
			offered = append(offered, spec.Name)
		}
	}

	for _, name := range offered {
		if name == "write_file" || name == "run_command" {
			t.Errorf("the researcher was offered %q", name)
		}
	}

	if len(offered) == 0 {
		t.Error("it was offered no tools at all")
	}
}

/*
 * An agent pinned to a service is still asked for through the router.
 *
 * Several agents must still mean one choke point. A roster is a way of saying
 * who should do what, and must never become a way of reaching a provider that
 * privacy forbids.
 */
func TestAPinnedAgentStillGoesThroughTheRouter(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"write it up","kind":"write","who":"writer"}]}`,
		replies: []llm.Response{{Content: "Written."}},
	}

	c, db, _ := newConductor(t, model)

	var askedFor []string

	c.Provider = func(name string) (llm.Provider, error) {
		askedFor = append(askedFor, name)

		return model, nil
	}

	c.Roster = func() []team.Agent {
		return []team.Agent{{
			Name: "writer", Title: "Writer", For: "writing", Uses: team.UsesBest,
			Provider: "anthropic",
		}}
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write it up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	var pinned bool

	for _, name := range askedFor {
		if name == "anthropic" {
			pinned = true
		}
	}

	if !pinned {
		t.Errorf("the pinned provider was never asked for: %v", askedFor)
	}
}

// A task whose agent's provider is refused stops at that step rather than
// carrying on with somebody else's model.
func TestARefusedProviderStopsTheTask(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"write it up","kind":"write","who":"writer"}]}`,
		replies: []llm.Response{{Content: "Written."}},
	}

	c, db, _ := newConductor(t, model)

	c.Roster = func() []team.Agent {
		return []team.Agent{{
			Name: "writer", Title: "Writer", For: "writing", Uses: team.UsesBest,
			Provider: "anthropic",
		}}
	}

	c.Provider = func(name string) (llm.Provider, error) {
		if name == "anthropic" {
			// The real thing now, rather than a stand-in: privacy refusing
			// has a type, because the conductor has to tell it apart from a
			// service being down — and one of those it works around.
			return nil, llm.NotAllowed{Provider: "anthropic", Mode: llm.ModePrivate}
		}

		return model, nil
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write it up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State != store.TaskBlocked {
		t.Errorf("the task ended %q, want blocked", done.State)
	}
}

/*
 * A service that is merely down does not end the job.
 *
 * The other half of the rule above, and the distinction matters: privacy
 * refusing is a decision somebody made and working around it would overrule
 * them, while a company having an outage is bad luck. A task that blocked at
 * step four of nine because an API was unavailable would have thrown away four
 * steps of real work.
 *
 * It only ever falls back towards this machine, never away from it.
 */
func TestAServiceThatIsDownDoesNotEndTheJob(t *testing.T) {
	model := &scripted{plan: `{"name":"Writing it up","done_when":"it is written",
		"steps":[{"do":"write it up","done_when":"a file exists","kind":"write","who":"writer"}]}`}

	c, db, _ := newConductor(t, model)

	c.Roster = func() []team.Agent {
		return []team.Agent{{
			Name: "writer", Title: "Writer", For: "writing", Uses: team.UsesBest,
			Provider: "anthropic",
		}}
	}

	asked := []string{}

	c.Provider = func(name string) (llm.Provider, error) {
		asked = append(asked, name)

		if name == "anthropic" {
			return nil, errors.New("dial tcp: connection refused")
		}

		return model, nil
	}

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "write it up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	done := settled(t, db, task.ID)

	if done.State == store.TaskBlocked {
		t.Errorf("a service being down blocked the task: %q", done.Because)
	}

	// Anthropic first, because that is who the agent is pinned to, and then
	// this machine. The planner's own provider comes before both.
	if len(asked) < 2 || asked[len(asked)-2] != "anthropic" || asked[len(asked)-1] != llm.Local {
		t.Errorf("the providers asked for were %v", asked)
	}

	steps, _ := db.Steps(task.ID)

	if len(steps) != 1 || steps[0].Provider == "anthropic" {
		t.Errorf("the step ran on %+v", steps)
	}
}

// The step is told which part of the work it is, or a model given five tools
// and no explanation spends its first round asking for a sixth.
func TestAStepIsToldWhichPartOfTheWorkItIs(t *testing.T) {
	model := &scripted{
		plan:    `{"name":"A job","steps":[{"do":"look it up","kind":"look","who":"researcher"}]}`,
		replies: []llm.Response{{Content: "Nothing to report."}},
	}

	c, db, _ := newConductor(t, model, tools.ReadFile{})

	conv, _ := db.NewConversation("t")

	task, _, err := c.Take(context.Background(), conv, "look something up", "scripted", true)
	if err != nil {
		t.Fatal(err)
	}

	settled(t, db, task.ID)

	var whole string

	for _, req := range model.asked {
		if planning(req) || checking(req) {
			continue
		}

		for _, m := range req.Messages {
			whole += m.Content
		}
	}

	if !strings.Contains(whole, "you are the researcher") && !strings.Contains(whole, "You are the researcher") {
		t.Errorf("the step was not told who it is: %q", whole)
	}

	// And what the researcher is told to do differently.
	if !strings.Contains(whole, "where each thing came from") {
		t.Errorf("the agent's standing instructions were not given: %q", whole)
	}
}

/*
 * The planner is shown a shortlist, not the organisation.
 *
 * One line each, so it is choosing between people rather than reading a
 * configuration file — and a handful of lines rather than however many people
 * there are. Showing the whole roster is what stopped an organisation being
 * allowed to grow: every agent's description in every planning prompt, on a
 * machine that reads at about ten tokens a second.
 */
func TestThePlannerIsShownAShortlistAndNotTheRoster(t *testing.T) {
	model := &scripted{replies: []llm.Response{{Content: "done"}}}

	c, db, _ := newConductor(t, model)

	/*
	 * Three hundred of them, which is the number the shortlist exists for.
	 *
	 * Each one distinct, because a roster of identical agents would prove
	 * nothing: the question is whether the right few are found among many,
	 * not whether many can be ignored.
	 */
	roster := team.BuiltIn()

	for i := range 300 {
		roster = append(roster, team.Agent{
			Name:  fmt.Sprintf("specialist_%d", i),
			Title: fmt.Sprintf("Specialist %d", i),
			For:   fmt.Sprintf("something to do with subject %d and nothing else", i),
			Uses:  team.UsesWork,
		})
	}

	c.Roster = func() []team.Agent { return roster }

	conv, _ := db.NewConversation("t")

	if _, _, err := c.Take(context.Background(), conv,
		"read the code and write up what it does", "scripted", true); err != nil {
		t.Fatal(err)
	}

	shown := ""

	for _, req := range model.asked {
		if !planning(req) {
			continue
		}

		for _, m := range req.Messages {
			shown += m.Content
		}
	}

	if shown == "" {
		t.Fatal("the planner was never asked")
	}

	// The people the request actually implies, and the generalist behind them.
	for _, name := range []string{"developer", "writer", "assistant"} {
		if !strings.Contains(shown, name) {
			t.Errorf("the planner was not told about the %s", name)
		}
	}

	// And not three hundred lines of people it has no use for.
	if named := strings.Count(shown, "specialist_"); named > team.Most {
		t.Errorf("%d of the specialists were put in front of the planner", named)
	}
}

/*
 * A request that suits nobody in particular still has somebody in it.
 *
 * An empty shortlist would be a plan with nobody to carry it out. The honest
 * answer to "nobody here specialises in this" is the one who does anything.
 */
func TestAShortlistIsNeverEmpty(t *testing.T) {
	who := team.Who(team.BuiltIn(), nil, nil, nil, team.Wanted{Doing: "do a thing"})

	if len(who) == 0 {
		t.Fatal("nobody was put forward")
	}

	if who[len(who)-1].Name != "assistant" {
		t.Errorf("the last resort is %q", who[len(who)-1].Name)
	}
}
