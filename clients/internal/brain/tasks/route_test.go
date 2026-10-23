package tasks

import (
	"errors"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/route"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

func installed() []route.Candidate {
	return []route.Candidate{
		{Name: "llama3.2:3b", Bytes: 2_000_000_000, Seconds: 4, ToolCall: "proper"},
		{Name: "qwen2.5-coder:7b", Bytes: 4_700_000_000, Seconds: 9, ToolCall: "proper"},
		{Name: "qwen2.5:14b", Bytes: 9_000_000_000, Seconds: 18, ToolCall: "proper"},
	}
}

// Each agent's step is thought with the model its work needs, chosen from
// what is installed at that moment, with the reason on the choice.
func TestEachStepIsThoughtWithTheModelItsAgentNeeds(t *testing.T) {
	models := installed()
	c := &Conductor{
		Sizes:  func() llm.Sizes { return llm.Sizes{Work: "llama3.2:3b"} },
		Models: func() []route.Candidate { return append([]route.Candidate{}, models...) },
	}

	step := &store.TaskStep{Instruction: "write the game loop"}

	dev := team.Agent{Name: "dev", Title: "Game developer", Uses: team.UsesWork}
	if got := c.choose(step, dev, llm.Local); got.Model != "qwen2.5-coder:7b" || !strings.Contains(got.Why, "made for code") {
		t.Fatalf("developer got %+v", got)
	}

	// A model removed while the task runs is not chosen at the next step.
	models = models[:1]
	if got := c.choose(step, dev, llm.Local); got.Model != "llama3.2:3b" {
		t.Fatalf("after the coder was removed the developer got %+v", got)
	}
}

func TestAModelThatFailsTwiceGivesWayToTheNext(t *testing.T) {
	c := &Conductor{Models: installed}
	step := &store.TaskStep{}
	dev := team.Agent{Name: "dev", Title: "Software developer"}

	first := c.choose(step, dev, llm.Local).Model

	c.outcome(first, errors.New("timed out"))
	c.outcome(first, errors.New("timed out"))

	if next := c.choose(step, dev, llm.Local).Model; next == first {
		t.Fatalf("still %s after two failures", next)
	}

	c.outcome(first, nil)

	if again := c.choose(step, dev, llm.Local).Model; again != first {
		t.Fatalf("a success did not restore %s (got %s)", first, again)
	}
}

func TestTheOwnersChoiceAndHostedServicesAreLeftAlone(t *testing.T) {
	c := &Conductor{
		Sizes:  func() llm.Sizes { return llm.Sizes{Work: "llama3.2:3b"} },
		Models: installed,
		Pins:   func() map[string]string { return map[string]string{"writer": "qwen2.5:14b"} },
	}
	step := &store.TaskStep{}

	if got := c.choose(step, team.Agent{Name: "dev", Prefers: []string{"mine:1b"}}, llm.Local); got.Model != "mine:1b" {
		t.Fatalf("an agent naming its model got %+v", got)
	}

	if got := c.choose(step, team.Agent{Name: "writer"}, llm.Local); got.Model != "qwen2.5:14b" || !strings.Contains(got.Why, "pinned") {
		t.Fatalf("a pin was not used: %+v", got)
	}

	if got := c.choose(step, team.Agent{Name: "dev"}, "anthropic"); got.Model != "" {
		t.Fatalf("a hosted step was given a local model: %+v", got)
	}
}

// Why the model was chosen is kept on the step, for the Tasks view.
func TestTheReasonForAModelIsKeptOnTheStep(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/brain.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.NewTask(store.Task{Name: "t", Goal: "g", State: store.TaskWorking})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AddSteps(id, []store.TaskStep{{Instruction: "write it"}}); err != nil {
		t.Fatal(err)
	}

	steps, _ := db.Steps(id)
	db.SetStepModelWhy(steps[0].ID, "qwen2.5-coder:7b: made for code")

	steps, _ = db.Steps(id)
	if steps[0].ModelWhy != "qwen2.5-coder:7b: made for code" {
		t.Fatalf("kept %q", steps[0].ModelWhy)
	}
}
