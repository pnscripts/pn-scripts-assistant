package brain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/tools"
)

// fakeAgent stands in for Claude Code: it writes one file where it was sent,
// says it ran a push, and keeps what it was told.
type fakeAgent struct {
	jobs    *[]orchestrator.Job
	failure string
}

func (fakeAgent) ID() string { return "claude-code" }

func (f fakeAgent) Run(_ context.Context, job orchestrator.Job) orchestrator.Outcome {
	*f.jobs = append(*f.jobs, job)

	if f.failure != "" {
		return orchestrator.Outcome{Failure: f.failure, Detail: "Not logged in"}
	}

	if job.Progress != nil {
		job.Progress("writing notes.md")
	}

	_ = os.WriteFile(filepath.Join(job.Dir, "notes.md"), []byte("done"), 0o644)

	return orchestrator.Outcome{OK: true, Said: "Wrote notes.md.", Model: "fake", Took: time.Second,
		Commands: []string{"git push origin main"}}
}

func agentBrain(t *testing.T, agent fakeAgent) *Brain {
	t.Helper()

	b := quietBrain(t)

	o := b.Orchestrator()
	o.Extra = []orchestrator.Resource{{ID: "claude-code", Kind: orchestrator.Agent, Title: "Claude Code",
		State: orchestrator.Available, Path: "/nowhere/claude"}}
	o.Executors = map[string]orchestrator.Executor{"claude-code": agent}
	o.Forget()

	return b
}

/*
 * A task handed to Claude Code from a conversation: it is told the folder,
 * how far it may reach and its user's rules; what it changed is read from
 * the folder; and a push it ran on its own against a rule is said.
 */
func TestACodingAgentIsAskedAndWhatItChangedIsShown(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs})

	if _, err := b.KeepRule("Never push anything.", ""); err != nil {
		t.Fatal(err)
	}

	agents := b.CodingAgents()
	if len(agents) != 1 || !agents[0].Ready || agents[0].Title != "Claude Code" {
		t.Fatalf("%+v", agents)
	}

	dir := t.TempDir()

	answer, err := b.AskCodingAgent(context.Background(), "claude-code", dir, "Write the notes.", tools.ReachFolder)
	if err != nil {
		t.Fatal(err)
	}

	if len(jobs) != 1 || jobs[0].Access != tools.ReachFolder || jobs[0].Dir != dir {
		t.Fatalf("%+v", jobs)
	}

	for _, want := range []string{"Write the notes.", "Work in " + dir, "You may change files in that folder only",
		"Never push anything."} {
		if !strings.Contains(jobs[0].Prompt, want) {
			t.Errorf("the brief does not say %q:\n%s", want, jobs[0].Prompt)
		}
	}

	text := answer.Text()
	for _, want := range []string{"Claude Code answered (fake)", "Wrote notes.md.", "- added: " + filepath.Join(dir, "notes.md"),
		"git push origin main", `went against your rule "Never push anything."`} {
		if !strings.Contains(text, want) {
			t.Errorf("the answer does not say %q:\n%s", want, text)
		}
	}
}

// Asked only to read, a changed folder is said rather than passed over.
func TestAnAgentAskedToReadThatChangedFilesIsSaid(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs})

	answer, err := b.AskCodingAgent(context.Background(), "claude-code", t.TempDir(), "Look.", tools.ReachRead)
	if err != nil || !strings.HasPrefix(answer.Said, "Note: it was asked only to read") {
		t.Fatalf("%+v %v", answer, err)
	}
}

// Not signed in is said with what to do, in the user's own terminal.
func TestAnAgentNotSignedInSaysHowToSignIn(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs, failure: orchestrator.FailAuth})

	_, err := b.AskCodingAgent(context.Background(), "claude-code", t.TempDir(), "Look.", tools.ReachRead)
	if err == nil || !strings.Contains(err.Error(), "run `claude` and sign in") {
		t.Fatalf("%v", err)
	}
}

// The folder is a real project folder: never the whole disk, never its own
// data, never the home folder or a system folder for an agent that writes.
func TestTheAgentsFolderIsChecked(t *testing.T) {
	var jobs []orchestrator.Job

	b := agentBrain(t, fakeAgent{jobs: &jobs})

	home, _ := os.UserHomeDir()

	for _, c := range []struct{ folder, access string }{
		{"/", tools.ReachRead},
		{"relative/path", tools.ReachRead},
		{filepath.Join(t.TempDir(), "missing"), tools.ReachRead},
		{b.Root, tools.ReachRead},
		{filepath.Dir(b.Root), tools.ReachRead},
		{home, tools.ReachFolder},
		{filepath.Dir(home), tools.ReachFolder},
		{"/media", tools.ReachFull},
		{"/etc", tools.ReachFull},
		{"/usr/share", tools.ReachFolder},
	} {
		if _, err := b.AskCodingAgent(context.Background(), "claude-code", c.folder, "x", c.access); err == nil {
			t.Errorf("%s with %s access was accepted", c.folder, c.access)
		}
	}

	if len(jobs) != 0 {
		t.Errorf("an agent was started: %+v", jobs)
	}
}
