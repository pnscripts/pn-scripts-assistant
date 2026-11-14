package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/quiethours"
	"pn-scripts-assistant/internal/brain/rules"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * The coding agents on this machine, asked from a conversation (see
 * tools.AskCodingAgent): which ones there are and whether each can be asked,
 * and one task handed to one of them in a folder, with the user's rules in
 * its instructions and what it changed read from the folder afterwards.
 */

// agentOrder is the agents a conversation can ask, in the order they are
// offered.
var agentOrder = []string{"claude-code", "codex", "cursor-agent"}

// agentRun is one task at a time: two agents writing in the same folder,
// or one user's allowance spent twice at once, is never what was meant.
var agentRun sync.Mutex

// AgentTimeout is the longest one task may take before it is stopped.
const AgentTimeout = 30 * time.Minute

// CodingAgents is every coding agent this machine has, and whether it can
// be asked now.
func (b *Brain) CodingAgents() []tools.CodingAgent {
	list, _ := b.Orchestrator().Resources(context.Background())

	var out []tools.CodingAgent

	for _, id := range agentOrder {
		for _, r := range list {
			if r.ID != id || r.State == orchestrator.NotInstalled {
				continue
			}

			a := tools.CodingAgent{ID: r.ID, Title: r.Title, Why: r.Why}

			if u, err := b.DB.UsageOf(r.ID); err == nil {
				now := time.Now()
				pace := orchestrator.PaceOf(u, now)
				a.Pace, a.PaceBG = pace.Say(), pace.SayIn("bg")

				// Ahead of its pace, outside the quiet hours: the work
				// waits for a yes that says until when.
				a.Held = quiethours.Held(b.QuietHours(), pace.Ahead, now)
				a.HeldBG = quiethours.HeldIn(b.QuietHours(), pace.Ahead, now, "bg")
			}

			switch {
			case r.State == orchestrator.Available:
				a.Ready = true
			case r.ID == "cursor-agent" && r.Path != "":
				// Its sign-in cannot be asked without starting a login; it is
				// tried, and not being signed in is said when it is.
				a.Ready, a.Why = true, "whether it is signed in shows when it is asked"
			}

			if _, ok := b.Orchestrator().ExecutorFor(r); !ok {
				a.Ready, a.Why = false, "this program cannot hand it work"
			}

			out = append(out, a)
		}
	}

	return out
}

// AskCodingAgent hands one task to one agent and waits for it.
func (b *Brain) AskCodingAgent(ctx context.Context, id, folder, task, access string) (tools.AgentAnswer, error) {
	dir, err := b.agentFolder(folder, access)
	if err != nil {
		return tools.AgentAnswer{}, err
	}

	list, _ := b.Orchestrator().Resources(ctx)

	var resource orchestrator.Resource

	for _, r := range list {
		if r.ID == id {
			resource = r
		}
	}

	executor, ok := b.Orchestrator().ExecutorFor(resource)
	if resource.ID == "" || !ok {
		return tools.AgentAnswer{}, fmt.Errorf("%s cannot be asked from here", id)
	}

	if !agentRun.TryLock() {
		return tools.AgentAnswer{}, errors.New("a coding agent is already working on a task; ask again when it has finished")
	}
	defer agentRun.Unlock()

	before := orchestrator.Snap(dir)

	job := orchestrator.Job{
		Dir:     dir,
		Prompt:  agentBrief(task, dir, access, b.LiveRules()),
		Access:  access,
		Timeout: AgentTimeout,
		Progress: func(line string) {
			progress.Detail(resource.Title + ": " + line)
		},
	}

	progress.Detail(resource.Title + " is working in " + dir)

	outcome := executor.Run(ctx, job)

	changes := orchestrator.Changes(before, orchestrator.Snap(dir))

	answer := tools.AgentAnswer{
		Agent: resource.Title, Model: outcome.Model, Said: outcome.Said, Commands: outcome.Commands,
		Took: outcome.Took.String(), Partial: before.Partial,
	}

	for _, c := range changes {
		answer.Changed = append(answer.Changed, tools.AgentChange{Path: c.Path, What: c.What})
	}

	if outcome.Failure != "" {
		return answer, errors.New(agentFailure(resource.Title, id, outcome, len(changes)))
	}

	if access == tools.ReachRead && len(changes) > 0 {
		answer.Said = "Note: it was asked only to read, and files in the folder changed while it worked " +
			"(listed below).\n\n" + answer.Said
	}

	// What it ran is judged against the rules too: a "never push" holds for
	// a push an agent made on its own.
	answer.Said += b.agentBrokeRules(outcome.Commands)

	return answer, nil
}

/*
 * agentFolder is the folder a task may work in: a real, existing folder,
 * never the whole disk, never this assistant's own data — and, for an
 * agent that may write, never the home folder or a system folder as a
 * whole, since "the folder" is then everything.
 */
func (b *Brain) agentFolder(folder, access string) (string, error) {
	folder = strings.TrimSpace(folder)

	home, _ := os.UserHomeDir()

	if folder == "~" || strings.HasPrefix(folder, "~/") {
		folder = filepath.Join(home, strings.TrimPrefix(folder, "~"))
	}

	if folder == "" || !filepath.IsAbs(folder) {
		return "", errors.New("give the folder as a full path")
	}

	dir, err := filepath.EvalSymlinks(filepath.Clean(folder))
	if err != nil {
		return "", fmt.Errorf("there is no folder %s", folder)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", folder)
	}

	if dir == string(filepath.Separator) || filepath.Dir(dir) == dir {
		return "", errors.New("not the whole disk: choose the project's folder")
	}

	if root, err := filepath.EvalSymlinks(b.Root); err == nil && (dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) ||
		strings.HasPrefix(root, dir+string(filepath.Separator))) {
		return "", errors.New("not this assistant's own data folder, or a folder that holds it")
	}

	if access == tools.ReachRead {
		return dir, nil
	}

	if home != "" {
		if real, err := filepath.EvalSymlinks(home); err == nil {
			home = real
		}

		// The home folder, or a folder that holds it ("/home"): "the folder"
		// would then be the user's whole home.
		if dir == filepath.Clean(home) || strings.HasPrefix(home, dir+string(filepath.Separator)) {
			return "", errors.New("not the whole home folder for an agent that may write: choose the project's folder")
		}
	}

	for _, mounts := range []string{"/media", "/mnt", "/run", "/srv"} {
		if dir == mounts {
			return "", fmt.Errorf("not %s as a whole for an agent that may write: choose the project's folder", mounts)
		}
	}

	for _, system := range []string{"/etc", "/usr", "/bin", "/sbin", "/lib", "/boot", "/var", "/opt", "/sys", "/proc", "/dev", "/root"} {
		if dir == system || strings.HasPrefix(dir, system+"/") {
			return "", fmt.Errorf("not a system folder (%s) for an agent that may write", system)
		}
	}

	return dir, nil
}

// agentBrief is the task as the agent reads it: the work, where, how far it
// may reach, and its user's rules.
func agentBrief(task, dir, access string, live []rules.Rule) string {
	var b strings.Builder

	b.WriteString(strings.TrimSpace(task) + "\n\n")
	b.WriteString("You are working for this computer's user, asked through their assistant. Work in " + dir + ".\n")

	switch access {
	case tools.ReachRead:
		b.WriteString("Only read: do not create, change or delete any file, and run nothing that changes anything.\n")
	case tools.ReachFolder:
		b.WriteString("You may change files in that folder only. Do not run commands.\n")
	case tools.ReachFull:
		b.WriteString("You may do what the work needs, as the user would.\n")
	}

	if block := rules.Block(live, RulesInPrompt); block != "" {
		b.WriteString("\n" + block + "\n")
	}

	b.WriteString("\nWhen you finish, say in a few sentences what you found or did, and which files you changed.")

	return b.String()
}

// agentFailure says what went wrong in words its user can act on.
func agentFailure(title, id string, o orchestrator.Outcome, changed int) string {
	var why string

	switch o.Failure {
	case orchestrator.FailAuth:
		login := map[string]string{"claude-code": "claude", "codex": "codex login", "cursor-agent": "cursor-agent login"}[id]
		why = fmt.Sprintf("%s is not signed in. Open a terminal, run `%s` and sign in with your own account, then ask again", title, login)
	case orchestrator.FailLimit:
		why = title + " has used up its allowance for now; ask again when it is renewed, or ask another agent"
	case orchestrator.FailOffline:
		why = title + " could not reach its service; this computer may be offline"
	case orchestrator.FailTimeout:
		why = fmt.Sprintf("%s was still working after %s and was stopped", title, AgentTimeout)
	case orchestrator.FailOutside:
		why = title + " tried to write outside the folder and was stopped: " + strings.Join(o.Outside, ", ")
	default:
		why = title + " stopped with an error: " + orElse(o.Detail, "it did not say why")
	}

	if changed > 0 {
		why += fmt.Sprintf(" (%d files in the folder changed before it stopped)", changed)
	}

	return why
}

func orElse(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}

	return s
}

// agentBrokeRules is the note for commands an agent ran that went against a
// precise "never" rule, or empty.
func (b *Brain) agentBrokeRules(commands []string) string {
	if len(commands) == 0 {
		return ""
	}

	live := b.LiveRules()
	if len(live) == 0 {
		return ""
	}

	var broke []rules.Rule

	seen := map[int64]bool{}

	for _, command := range commands {
		raw, _ := json.Marshal(map[string][]string{"argv": {"sh", "-c", command}})
		act := rules.Act{Actions: tools.ActionsOf(tools.RunCommand{}, raw), Text: command, At: time.Now()}

		for _, r := range rules.After(live, act) {
			if !seen[r.ID] {
				seen[r.ID] = true
				broke = append(broke, r)
			}
		}
	}

	if len(broke) == 0 {
		return ""
	}

	var texts []string

	for _, r := range broke {
		texts = append(texts, "\""+r.Text+"\"")
	}

	return "\n\nWhat it ran went against your rule " + strings.Join(texts, " and ") +
		". Tell your user plainly what was done."
}
