package brain

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/orchestrator"
)

// A project is given the time whoever may end up writing it needs: longer
// when that could be a model on a processor with no graphics card, even as
// the fallback, and the usual half hour otherwise.
func TestAProjectIsGivenTimeForWhoMayWriteIt(t *testing.T) {
	b := quietBrain(t)

	claude := orchestrator.Resource{ID: "claude-code", Kind: orchestrator.Agent, Title: "Claude Code",
		Tier: orchestrator.Subscription, State: orchestrator.Available, Executes: true,
		Can: map[string]int{orchestrator.Code: 9, "tools": 1}}
	qwen := orchestrator.Resource{ID: "ollama:qwen", Kind: orchestrator.Model, Title: "qwen (local)",
		Tier: orchestrator.Local, Local: true, State: orchestrator.Available, Executes: true,
		Can: map[string]int{orchestrator.Code: 7, "tools": 1}}

	b.orch.once.Do(func() {})
	b.orch.o = orchestrator.New(orchestrator.Sources{Root: t.TempDir(), Extra: []orchestrator.Resource{claude, qwen}})

	project := capability.Package{Work: capability.Project}

	b.tier = "modest"

	if minutes, why := b.projectTime(t.TempDir(), project); minutes != 90 || !strings.Contains(why, "if it falls to qwen") {
		t.Errorf("a local fallback on a modest machine got %d minutes: %s", minutes, why)
	}

	b.tier = "generous"

	if minutes, _ := b.projectTime(t.TempDir(), project); minutes != 30 {
		t.Errorf("a machine with a card got %d minutes", minutes)
	}

	if minutes, _ := b.projectTime(t.TempDir(), capability.Package{Work: capability.Advisory}); minutes != 0 {
		t.Errorf("work that is not a project was given a project's time: %d", minutes)
	}
}

// When the writer has to be installed first, the proposal says who that
// will be and on what terms — not that nobody can write it, and not every
// reason twice.
func TestAnInstallFirstStackSaysWhoWillWrite(t *testing.T) {
	d := orchestrator.Decision{Blocked: "nothing that can do coding on a project is usable now: Claude Code — signed out",
		Install:  &orchestrator.ModelPlan{Name: "qwen2.5-coder:1.5b", Size: 940 << 20, Licence: "Apache License", Auto: true},
		Rejected: []orchestrator.Rejection{{ID: "claude-code", Title: "Claude Code", Why: "signed out"}}}

	said := strings.Join(stackLines(d, "threejs"), "\n")

	if !strings.Contains(said, "Writes it: qwen2.5-coder:1.5b (local), installed first") ||
		!strings.Contains(said, "940 MB") || strings.Contains(said, "Nobody") || strings.Count(said, "signed out") != 1 {
		t.Errorf("the stack does not say plainly who will write it:\n%s", said)
	}
}
