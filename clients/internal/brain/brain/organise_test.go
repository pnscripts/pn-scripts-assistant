package brain

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/bootstrap"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/workspace"
)

// quietBrain is a brain whose model is nowhere, so a task started here stops
// at its first step instead of thinking on this machine's real model.
func quietBrain(t *testing.T) *Brain {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Petar"
	cfg.OllamaURL = "http://127.0.0.1:9"

	team.Forget()

	b := New(db, cfg, root, filepath.Join(root, "brain.sqlite"), slog.New(slog.NewTextHandler(io.Discard, nil)))

	t.Cleanup(b.Stop)

	return b
}

/*
 * "Hire an electrician to help me plan this installation", as a conversation:
 * a proposal, the one question, and then an approval — nothing hired until
 * that approval is given, and the hire then works on exactly that task.
 */
func TestHiringInConversation(t *testing.T) {
	b := quietBrain(t)

	conv, _ := b.DB.NewConversation("hiring")

	reply, pending, handled := b.handleOrganising(context.Background(), conv, "Hire an electrician to help me plan this installation")
	if !handled || len(pending) != 0 {
		t.Fatalf("not handled, or approval asked before the question: %v %v", handled, pending)
	}

	for _, want := range []string{"Proposal", "Electrical planning", "licensed electrician", "permanent, or only for one task"} {
		if !strings.Contains(reply, want) {
			t.Errorf("the proposal does not say %q:\n%s", want, reply)
		}
	}

	if entries, _ := os.ReadDir(filepath.Join(b.Root, team.FolderName)); len(entries) != 0 {
		t.Fatal("somebody was hired before anybody said so")
	}

	reply, pending, handled = b.handleOrganising(context.Background(), conv, "just for this task")
	if !handled || len(pending) != 1 || pending[0].Tool != "confirm_hire" {
		t.Fatalf("the answer did not become an approval: %q %+v", reply, pending)
	}

	if !strings.Contains(pending[0].Summary, "for one task") || !strings.Contains(pending[0].Summary, "Needs a qualified professional") {
		t.Errorf("the approval does not show the whole proposal: %q", pending[0].Summary)
	}

	done, err := b.Decide(context.Background(), pending[0].ID, true)
	if err != nil {
		t.Fatalf("approving the hire: %v", err)
	}

	if !strings.Contains(done.Result, "for this task only") {
		t.Errorf("the hire said %q", done.Result)
	}

	tasks, _ := b.DB.LiveTasks()
	all, _ := b.DB.Tasks(nil, 10)

	found := false

	for _, task := range append(tasks, all...) {
		if strings.Contains(task.Hired, "hired for this task") && strings.Contains(task.Goal, "help me plan this installation") {
			found = true
		}
	}

	if !found {
		t.Error("no task was started for the hire, or it does not name them")
	}

	// And approving it again does nothing.
	if _, err := b.Decide(context.Background(), pending[0].ID, true); err == nil {
		t.Error("an approval was carried out twice")
	}
}

/*
 * "Make me a Tetris game", as a conversation: where, then which engine, then
 * the plan, then an approval — and the project is laid out, confined to its
 * folder, only after it.
 */
func TestAProjectInConversation(t *testing.T) {
	b := quietBrain(t)

	conv, _ := b.DB.NewConversation("tetris")
	dir := filepath.Join(t.TempDir(), "tetris")

	steps := []struct {
		say, want string
	}{
		{"Make me a Tetris game", bootstrap.WhereQuestion},
		// The engine is chosen, not asked, and the proposal says why and
		// how the work will be done.
		{dir, "Shall I start it?"},
	}

	last := ""

	for _, step := range steps {
		reply, pending, handled := b.handleOrganising(context.Background(), conv, step.say)

		if !handled || len(pending) != 0 || !strings.Contains(reply, step.want) {
			t.Fatalf("after %q: handled %v, pending %v, reply:\n%s", step.say, handled, pending, reply)
		}

		last = reply
	}

	if _, err := os.Stat(dir); err == nil {
		t.Fatal("the folder was made before anybody said yes")
	}

	for _, want := range []string{"Chosen because", "How it will be done", "Checked, run and built by this program",
		"Time it is given"} {
		if !strings.Contains(last, want) {
			t.Errorf("the proposal does not say %q:\n%s", want, last)
		}
	}

	_, pending, handled := b.handleOrganising(context.Background(), conv, "yes, start it")
	if !handled || len(pending) != 1 || pending[0].Tool != "start_project" {
		t.Fatalf("yes did not become an approval: %+v", pending)
	}

	if !strings.HasPrefix(pending[0].Summary, "Always asked") {
		t.Errorf("starting a project could be skipped on a standing yes: %q", pending[0].Summary)
	}

	if _, err := b.Decide(context.Background(), pending[0].ID, true); err != nil {
		t.Fatalf("approving the project: %v", err)
	}

	// Set up in the background: wait for it.
	var task *store.Task

	for i := 0; i < 100 && task == nil; i++ {
		time.Sleep(100 * time.Millisecond)

		all, _ := b.DB.Tasks(nil, 10)

		for i := range all {
			if all[i].Project == dir {
				task = &all[i]
			}
		}
	}

	if task == nil {
		t.Fatal("no task was started for the project")
	}

	for _, file := range []string{"index.html", "src/main.js", "vendor/three.module.js", ".pn-assistant/project.json"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Errorf("%s was not laid out", file)
		}
	}

	c, err := workspace.Load(dir)
	if err != nil || c.Engine != "threejs" || len(c.Acceptance) == 0 {
		t.Errorf("the project's settings: %+v %v", c, err)
	}

	if task.Packages != "software.game.threejs" || !strings.Contains(task.Hired, "hired for this project") {
		t.Errorf("the task does not know its package or its hire: %+v", task)
	}

	// The task's row exists a moment before what led to it is written beside
	// it; under load that moment is long enough to read in between.
	kinds := map[string]int{}

	for i := 0; i < 100 && kinds[store.EvidenceApproval] == 0; i++ {
		evidence, _ := b.DB.EvidenceFor(task.ID)

		kinds = map[string]int{}

		for _, e := range evidence {
			kinds[e.Kind]++
		}

		if kinds[store.EvidenceApproval] == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	if kinds[store.EvidenceApproval] != 1 || kinds[store.EvidenceFile] < 5 {
		t.Errorf("what was approved and written is not in the evidence: %v", kinds)
	}
}

// A yes to a proposal that cannot start is answered with why, and starts
// nothing — not handed to a model to make sense of.
func TestAYesToABlockedProjectSaysWhy(t *testing.T) {
	b := quietBrain(t)

	conv, _ := b.DB.NewConversation("unity")
	body, _ := json.Marshal(bootstrap.Proposal{Name: "Maze",
		Blockers: []string{"Unity editor is installed, but not licensed for this use"}})

	b.DB.Propose(store.Proposal{Kind: store.ProposeProject, ConversationID: conv, Body: string(body)})

	reply, pending, handled := b.handleOrganising(context.Background(), conv, "yes, start it")
	if !handled || len(pending) != 0 || !strings.Contains(reply, "not licensed") ||
		!strings.Contains(reply, "Nothing has been started") {
		t.Errorf("handled %v, pending %v, reply:\n%s", handled, pending, reply)
	}
}

/*
 * Every package that ships loads on a real brain — with no mail and no smart
 * home set up — and every tool its role names is one this brain has.
 *
 * A package that fails its check is left out whole, so a single tool name
 * unknown to the registry would quietly take a whole profession's limits away
 * and leave its hires to capability-derived tools. That happened: forbidding
 * send_email refused every package on a machine with no mailbox.
 */
func TestEveryShippedPackageLoadsOnARealBrain(t *testing.T) {
	b := quietBrain(t)

	set := b.Packages()

	if len(set.Refused) > 0 {
		t.Fatalf("packages were refused: %v", set.Refused)
	}

	if len(set.All()) < 11 {
		t.Fatalf("%d packages loaded", len(set.All()))
	}

	for _, p := range set.All() {
		if p.Engine != "" && !b.Engines.Known(p.Engine) {
			t.Errorf("%s names the engine %s, which there is no adapter for", p.ID, p.Engine)
		}

		for _, tool := range p.Role.Tools {
			if _, ok := b.Agent.Registry.Get(tool); !ok {
				t.Errorf("%s's role may use %s, which this brain does not have", p.ID, tool)
			}
		}
	}
}
