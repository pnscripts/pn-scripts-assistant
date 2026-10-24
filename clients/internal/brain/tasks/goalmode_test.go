package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/route"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

// With nothing installed that fits, goal mode downloads one and the step is
// thought with it; with goal mode off, nothing is downloaded.
func TestGoalModeDownloadsTheModelAStepNeeds(t *testing.T) {
	var here []route.Candidate

	var asked []string

	c := &Conductor{
		Sizes:  func() llm.Sizes { return llm.Sizes{Work: "llama3.2:3b"} },
		Models: func() []route.Candidate { return append([]route.Candidate{}, here...) },
		Provide: func(expert string, needs route.Needs) error {
			asked = append(asked, expert)
			here = append(here, route.Candidate{Name: "qwen2.5-coder:7b", Bytes: 4_700_000_000, Seconds: 9, ToolCall: "proper"})

			return nil
		},
	}

	dev := team.Agent{Name: "dev", Title: "Game developer", Uses: team.UsesWork}

	var used []string
	c.Used = func(model string) { used = append(used, model) }

	got := c.choose(&store.TaskStep{Instruction: "write the game loop"}, dev, llm.Local)
	if got.Model != "qwen2.5-coder:7b" || strings.Join(asked, ",") != "dev" {
		t.Fatalf("got %+v after asking for %v", got, asked)
	}

	// And it counts as used, so it is not removed as idle.
	if strings.Join(used, ",") != "qwen2.5-coder:7b" {
		t.Errorf("used: %v", used)
	}

	off := &Conductor{Sizes: c.Sizes, Models: func() []route.Candidate { return nil }}
	if got := off.choose(&store.TaskStep{}, dev, llm.Local); got.Model == "qwen2.5-coder:7b" {
		t.Error("a model appeared with goal mode off")
	}
}

// Work that makes something gets its own folder; work on the user's own
// files does not; and the report says what was prepared for it.
func TestAGoalWorksInItsOwnFolderAndReportsWhatWasPrepared(t *testing.T) {
	c, db, _ := newConductor(t, nil)

	var folders []string

	c.GoalFolder = func(id int64, name string) (string, error) {
		dir := t.TempDir()
		folders = append(folders, dir)

		return dir, nil
	}

	c.Prepared = func(since time.Time) []string {
		return []string{"• Godot 4.7.1 (from github.com, for game_developer)"}
	}

	steps := []store.TaskStep{{Instruction: "plan it"}, {Instruction: "build it"}}

	made, _, err := c.TakeWith(context.Background(), 0, "Make me a great space game", "", true, Taking{Steps: steps})
	if err != nil || made == nil {
		t.Fatal(err)
	}

	task, _ := db.Task(made.ID)
	if len(folders) != 1 || task.Project != folders[0] {
		t.Errorf("the game was not given its folder: %q %v", task.Project, folders)
	}

	sorted, _, _ := c.TakeWith(context.Background(), 0, "Sort the photos in ~/Pictures by year", "", true, Taking{Steps: steps})
	if again, _ := db.Task(sorted.ID); again.Project != "" || len(folders) != 1 {
		t.Errorf("work on the user's own files was moved into a folder: %q", again.Project)
	}

	report := c.report(task, nil, store.TaskDone, "")
	if !strings.Contains(report, "Prepared for this job:\n• Godot 4.7.1") {
		t.Errorf("report:\n%s", report)
	}
}
