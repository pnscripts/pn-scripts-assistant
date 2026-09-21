package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/orchestrator"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
	"pn-scripts-assistant/internal/protocol"
)

/*
 * A project's work, done by whoever the orchestrator chose, and checked by
 * this program itself.
 *
 * Writing a game is somebody's work: a coding agent its owner subscribes to,
 * or a model on this machine through this program's own tools — whichever the
 * orchestrator decides, with the rest in reserve. When the one doing it is
 * signed out, out of allowance or unreachable, the next takes over the same
 * step: the files already written stay written, the step carries on from
 * them, and the switch is recorded and said.
 *
 * Checking, running and building it are not anybody's word. They are this
 * program running the engine — an action on the step rather than a model
 * asked to call a tool — and when one fails, what it reported goes back to
 * whoever writes the game to fix, and the engine runs again.
 */

// MostFixRounds is how many times a failing check goes back to be fixed
// before the step is given up as failed.
const MostFixRounds = 3

// decideFor is who writes a project's code for this step, under the owner's
// policy narrowed by the project's settings and the task's own.
func (c *Conductor) decideFor(ctx context.Context, task *store.Task, work string) orchestrator.Decision {
	over := orchestrator.Override{}

	if task.Project != "" {
		if cfg, err := workspace.Load(task.Project); err == nil && len(cfg.Resources) > 0 {
			over, _ = orchestrator.ParseOverride(string(cfg.Resources))
		}
	}

	if task.Resources != "" {
		if own, err := orchestrator.ParseOverride(task.Resources); err == nil {
			over = orchestrator.Merge(over, own)
		}
	}

	return c.Orchestrator.Decide(ctx, orchestrator.Need{Work: work, Files: true, Without: c.failedOn(task)}, over)
}

// failedOn is the resources that already failed this task for a reason of
// their own, not a sign-in or an allowance — those are their state.
func (c *Conductor) failedOn(task *store.Task) []string {
	runs, err := c.DB.Runs("", 200)
	if err != nil {
		return nil
	}

	var out []string

	for _, r := range runs {
		// Writing nothing says nothing about whether it can: the step is
		// tried again, and it may be the only writer there is.
		if r.TaskID == task.ID && r.Result == store.RunFailed && r.Detail != nothingWritten &&
			(r.Failure == orchestrator.FailError || r.Failure == orchestrator.FailTimeout) {
			out = append(out, r.Resource)
		}
	}

	return out
}

// orchestrated is whether a step is written by whoever the orchestrator
// chooses: a project's own work that changes its files, and not a review.
func (c *Conductor) orchestrated(task *store.Task, step *store.TaskStep) bool {
	return c.Orchestrator != nil && task.Project != "" && step.Action == "" && step.Changes &&
		step.ReviewOf == 0 && (step.Kind == store.StepDo || step.Kind == store.StepWrite)
}

// decided keeps the decision as evidence, once for each resource chosen.
func (c *Conductor) decided(task *store.Task, step *store.TaskStep, d orchestrator.Decision) {
	if d.Primary == nil {
		return
	}

	// A step of this program's own — a check, a run — keeps its name; a
	// writing step says who is writing it.
	if step.Action == "" {
		provider, model := "agent:"+d.Primary.ID, orElse(d.Primary.Model, d.Primary.Title)
		if d.Primary.Kind == orchestrator.Model {
			provider = strings.TrimPrefix(d.Primary.ID, "api:")
			if d.Primary.Local {
				provider = llm.Local
			}
		}

		c.DB.SetStepWriter(step.ID, provider, model)
	}

	subject := "writing: " + d.Primary.Title

	if rows, err := c.DB.EvidenceFor(task.ID); err == nil {
		for _, e := range rows {
			if e.Kind == store.EvidenceDecision && e.Subject == subject {
				return
			}
		}
	}

	c.DB.Record(store.Evidence{TaskID: task.ID, StepID: step.ID, Kind: store.EvidenceDecision,
		Subject: subject, Detail: d.Explain(), OK: true})

	// Said as well as recorded: a decision is the thing somebody watching
	// most wants to see, and it is made long before the step finishes.
	kind := protocol.ModelSelected
	if d.Primary.Kind == orchestrator.Agent {
		kind = protocol.AgentSelected
	}

	c.happened(kind, task, step, subject, map[string]any{
		"resource": d.Primary.ID, "title": d.Primary.Title, "model": d.Primary.Model,
		"local": d.Primary.Local, "why": d.Reasons,
	})
}

// switched records one resource giving way to the next, and says so as
// loudly as the owner's policy asks.
func (c *Conductor) switched(task *store.Task, step *store.TaskStep, from orchestrator.Resource,
	to *orchestrator.Resource, why string) {
	next := "nothing — every way to do this has failed"
	if to != nil {
		next = to.Title
	}

	detail := fmt.Sprintf("%s gave way: %s. Continuing with %s; the task's state is kept — "+
		"files already written stay, and the step carries on from them.", from.Title, why, next)

	c.DB.Record(store.Evidence{TaskID: task.ID, StepID: step.ID, Kind: store.EvidenceSwitch,
		Subject: from.Title + " → " + next, Detail: detail})

	c.DB.RecordRun(store.ResourceRun{Resource: from.ID, Kind: string(from.Kind), Work: "code",
		TaskID: task.ID, StepID: step.ID, Result: store.RunSwitched, Detail: why})

	c.happened(protocol.ModelSelected, task, step, "switched to "+next,
		map[string]any{"from": from.ID, "to": next, "why": why})

	switch c.Orchestrator.Policy().Notify {
	case "minimal":
	case "verbose":
		c.say(task, "Resource changed. From: "+from.Title+". To: "+next+". Reason: "+why+
			". Task state preserved; no action needed.")
	default:
		c.say(task, c.inOwnWords(task.Provider, "one line telling somebody the work has moved to something else, mid-job",
			map[string]any{"was being done by": from.Title, "now being done by": next, "why it moved": why,
				"nothing already done is lost": true, "nothing is needed from them": true},
			"Switched from "+from.Title+" to "+next+" ("+why+"). Nothing is lost; no action needed."))
	}
}

/*
 * carryOut is a project's writing done by whoever was chosen, failing over
 * down the list. fix, when given, is what a check found — the writer is asked
 * to put that right rather than to start again.
 */
func (c *Conductor) carryOut(ctx context.Context, task *store.Task, t *turn, fix string) (agent.Result, error) {
	work := orchestrator.Code
	if fix != "" {
		work = orchestrator.Edit
	}

	d := c.decideFor(ctx, task, work)

	if d.Primary == nil && d.Install != nil {
		if !d.Install.Auto {
			return agent.Result{}, fmt.Errorf("nothing here can write it, and installing %s needs you first: %s",
				d.Install.Name, d.Install.Ask)
		}

		if err := c.installModel(ctx, task, t.step, *d.Install); err != nil {
			return agent.Result{}, err
		}

		d = c.decideFor(ctx, task, work)
	}

	if d.Primary == nil {
		return agent.Result{}, fmt.Errorf("nothing can write it now: %s", d.Blocked)
	}

	var steps []agent.Step

	for {
		r := *d.Primary

		c.decided(task, t.step, d)

		res, why, err := c.attempt(ctx, task, t, r, work, fix)

		steps = append(steps, res.Steps...)

		if why == "" {
			res.Steps = steps

			return res, err
		}

		if strings.HasPrefix(why, orchestrator.FailOutside) {
			// Not a failure to route around: a boundary crossed. It stops.
			return agent.Result{Steps: steps}, fmt.Errorf("stopped: %s", why)
		}

		next, ok := d.Next()

		if !ok {
			// Writing nothing is a step that did not do its work, for the
			// step's own check to judge and to try again or hand on — not a
			// task that cannot go on, nor a switch to nobody. Blocking here
			// ended a Godot game after one silent answer, where a second
			// attempt was still due.
			if strings.HasSuffix(why, nothingWritten) {
				res.Steps = steps

				return res, nil
			}

			c.switched(task, t.step, r, nil, why)

			return agent.Result{Steps: steps}, fmt.Errorf("every way to write it failed; the last: %s", why)
		}

		c.switched(task, t.step, r, next.Primary, why)
		d = next
	}
}

/*
 * attempt is one resource trying once. why is empty when it did the work,
 * and otherwise says why it gave way, starting with its kind of failure.
 */
func (c *Conductor) attempt(ctx context.Context, task *store.Task, t *turn, r orchestrator.Resource,
	work, fix string) (agent.Result, string, error) {
	switch r.Kind {
	case orchestrator.Agent:
		ex, ok := c.Orchestrator.ExecutorFor(r)
		if !ok {
			return agent.Result{}, orchestrator.FailError + ": this program has no way to hand it work", nil
		}

		progress.Detail(r.Title + " is writing " + task.Name)

		out := ex.Run(ctx, orchestrator.Job{Dir: task.Project, Prompt: c.agentPrompt(task, t.step, fix),
			Model: orchestrator.ModelFor(r, work)})

		// Ended, not failed: nothing about the agent is learned from it.
		if ctx.Err() != nil {
			return agent.Result{}, "", ctx.Err()
		}

		if out.Failure == "" && len(out.Files) == 0 && (t.step.Changes || fix != "") {
			out.OK, out.Failure, out.Detail = false, orchestrator.FailError, nothingWritten
		}

		c.Orchestrator.Note(r, work, task.ID, t.step.ID, out, false)

		summary := fmt.Sprintf("%s changed %d files in %s", r.Title, len(out.Files), out.Took)

		step := agent.Step{Tool: r.ID, Summary: summary, Result: out.Said, Millis: out.Took.Milliseconds(),
			Failed: out.Failure != "", Evidence: out.Evidence(r.Title)}

		res := agent.Result{Reply: out.Said, Provider: "agent:" + r.ID, Model: out.Model,
			Steps: []agent.Step{step}, Rounds: 1}

		if out.Failure == "" && (out.OK || len(out.Files) > 0) {
			return res, "", nil
		}

		return res, out.Failure + ": " + orElse(out.Detail, "it gave up without saying why"), nil
	case orchestrator.Model:
		name := llm.Local
		if !r.Local {
			name = strings.TrimPrefix(r.ID, "api:")
		}

		provider, err := c.Provider(name)
		if err != nil {
			return agent.Result{}, orchestrator.FailOffline + ": " + plainly(err), nil
		}

		brief := t.brief
		brief.Choice.Model = r.Model
		brief.Only = writing(brief.Only)

		step := *t.step
		if fix != "" {
			step.Instruction = fixInstruction(task, t.step) + "\n\nWhat it found:\n" + fix
			step.DoneWhen = "the project's files are changed so that it passes"
		}

		started := time.Now()

		res, err := c.Agent.RunBrief(ctx, task.WorkConversationID, provider, c.brief(task, &step, t.member), brief)

		if ctx.Err() != nil {
			return res, "", ctx.Err()
		}

		out := orchestrator.Outcome{OK: err == nil, Model: r.Model, Took: time.Since(started)}

		switch {
		case err != nil:
			out.Failure, out.Detail = orchestrator.FailError, plainly(err)
		case !wroteIn(res) && (t.step.Changes || fix != ""):
			out.OK, out.Failure, out.Detail = false, orchestrator.FailError, nothingWritten
		}

		c.Orchestrator.Note(r, work, task.ID, t.step.ID, out, false)

		if out.Failure != "" {
			return res, out.Failure + ": " + out.Detail, nil
		}

		return res, "", nil
	}

	return agent.Result{}, orchestrator.FailError + ": it is not something work can be given to", nil
}

// agentPrompt is the work as a coding agent is handed it: the project, the
// step, the rules it works under, and what a check found when fixing.
func (c *Conductor) agentPrompt(task *store.Task, step *store.TaskStep, fix string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "You are working on the project in %s.\n\n", task.Project)

	if about := c.aboutTheProject(task); about != "" {
		b.WriteString(about + "\n\n")
	}

	switch {
	case fix != "":
		b.WriteString("Your task: " + fixInstruction(task, step) + "\n")
	default:
		b.WriteString("Your task: " + step.Instruction + "\n")

		if step.DoneWhen != "" {
			b.WriteString("It is done when: " + step.DoneWhen + "\n")
		}
	}

	b.WriteString(`
Rules:
- Work only inside that folder. Never change .pn-assistant/project.json, and never write outside the folder.
- Do not run commands, servers or builds: when you finish, this program checks, runs, photographs and builds
  the project itself, and anything that fails comes back to you with what it found.
- Write complete, working files, not outlines or placeholders. The game must show something from its first frame.
- When you have finished, say in two or three sentences what you wrote.
`)

	if fix != "" {
		b.WriteString("\nThe last check of the project found this. Fix it, changing only what is needed:\n" + fix + "\n")
	}

	return b.String()
}

// installModel puts a local model on this machine within the owner's policy,
// keeping every proof of it as evidence.
func (c *Conductor) installModel(ctx context.Context, task *store.Task, step *store.TaskStep, plan orchestrator.ModelPlan) error {
	c.say(task, fmt.Sprintf("Installing %s (%s, licence %q) to do this on this machine, as your policy allows.",
		plan.Name, sizeWords(plan.Size), plan.Licence))

	err := orchestrator.InstallModel(ctx, c.Orchestrator.OllamaURL, plan, func(line string) { progress.Detail(line) },
		func(e store.Evidence) {
			e.TaskID, e.StepID = task.ID, step.ID
			c.DB.Record(e)
		})

	c.Orchestrator.Forget()

	if err != nil {
		return fmt.Errorf("installing %s failed: %w", plan.Name, err)
	}

	return nil
}

func sizeWords(bytes int64) string {
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	}

	return fmt.Sprintf("%d MB", bytes>>20)
}

/*
 * act is a step this program does itself: checking, running or building a
 * project with its engine. A failure goes back to be fixed by whoever writes
 * the project, and the engine runs again — until it passes or the rounds run
 * out — and every run and every fix is kept as evidence, in order.
 */
func (c *Conductor) act(ctx context.Context, task *store.Task, t *turn) (agent.Result, error) {
	action := strings.TrimPrefix(t.step.Action, "engine:")

	name, what := "game_build", action
	if action == "check" {
		name, what = "game_check", ""
	}

	tool, ok := c.Agent.Registry.Get(name)
	if !ok {
		return agent.Result{}, fmt.Errorf("this build cannot %s a game: it has no %s", action, name)
	}

	args, _ := json.Marshal(map[string]string{"project": task.Project, "what": what})
	confined := agent.Confined(ctx, t.brief.Within)

	engine := ""
	if cfg, err := workspace.Load(task.Project); err == nil {
		engine = cfg.Engine
	}

	res := agent.Result{Provider: "this program", Model: engine, Rounds: 1}

	for round := 0; ; round++ {
		progress.Detail(action + " " + task.Name)

		started := time.Now()

		out, evidence, err := tools.Perform(confined, tool, args)
		passed := err == nil && passedIn(evidence, action)

		if err != nil {
			out = err.Error()
		}

		res.Steps = append(res.Steps, agent.Step{Tool: name, Summary: action + " " + task.Project, Result: out,
			Failed: !passed, Evidence: evidence, Millis: time.Since(started).Milliseconds()})
		res.Reply = out

		c.noteEngine(engine, action, task, t.step, passed, evidence, out)

		// What was written has now been checked by this program, not only
		// finished by whoever wrote it: see orchestrator.History.
		if passed && action == "check" {
			c.DB.VerifyWriting(task.ID)
		}

		if passed || !t.step.Changes || c.Orchestrator == nil || round >= MostFixRounds {
			return res, nil
		}

		fixed, err := c.carryOut(ctx, task, t, pointAt(task.Project, out))
		res.Steps = append(res.Steps, fixed.Steps...)

		if err != nil {
			res.Reply = out + "\n\nIt could not be fixed: " + plainly(err)

			return res, nil
		}

		// Running the engine again on the same files finds the same thing.
		if !wroteIn(fixed) {
			res.Reply = out + "\n\nIt could not be fixed: whoever was given it changed no files."

			return res, nil
		}
	}
}

// fileLine is a place a check points at: main.js:21, res://main.gd:5.
var fileLine = regexp.MustCompile(`(?:res://)?([A-Za-z0-9_./-]+\.[A-Za-z0-9]+):(\d+)`)

/*
 * pointAt is what a check found, with the lines it points at quoted from the
 * project exactly. An edit is made by quoting the text to replace, and a small
 * model working from "main.js:21" retypes that line from memory, gets a
 * character wrong and is refused — seen on this machine, three times over, on
 * a missing bracket. Only files inside the project are read.
 */
func pointAt(project, found string) string {
	var quoted []string

	seen := map[string]bool{}

	for _, m := range fileLine.FindAllStringSubmatch(found, -1) {
		if len(quoted) == 4 || seen[m[1]+":"+m[2]] {
			continue
		}

		seen[m[1]+":"+m[2]] = true

		path := filepath.Join(project, filepath.FromSlash(m[1]))
		if rel, err := filepath.Rel(project, path); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		n, _ := strconv.Atoi(m[2])
		lines := strings.Split(string(raw), "\n")

		if n < 1 || n > len(lines) {
			continue
		}

		var b strings.Builder

		fmt.Fprintf(&b, "%s, around line %d:", m[1], n)

		for i := max(n-2, 1); i <= min(n+1, len(lines)); i++ {
			fmt.Fprintf(&b, "\n  %d | %s", i, strings.TrimRight(lines[i-1], "\r"))
		}

		quoted = append(quoted, b.String())
	}

	if len(quoted) == 0 {
		return found
	}

	return found + "\n\nThe lines it points at, exactly as they are now (copy them exactly when you replace them):\n" +
		strings.Join(quoted, "\n")
}

/*
 * nothingWritten is a writer's run that changed no file where changing files
 * was the work. It is recorded as failed, so that a check passing afterwards
 * on files somebody else wrote is never counted to its credit, and the work
 * goes to the next in line.
 */
const nothingWritten = "it changed no files in the project"

/*
 * fixInstruction is what whoever writes the game is asked when a check, a
 * run or a build of it failed: to change its files so that it passes.
 *
 * Not the failed step's own instruction with the failure added. That told a
 * seven-billion-parameter model to "run the game with game_build", which it
 * took as the work — and it answered in six words, changed nothing, and the
 * engine found the same thing three times.
 */
func fixInstruction(task *store.Task, step *store.TaskStep) string {
	what := map[string]string{"engine:smoke": "run and photographed", "engine:build": "built"}[step.Action]
	if what == "" {
		what = "checked"
	}

	return fmt.Sprintf("The game in %s was %s by this program, and it failed. Change the project's files so "+
		"that it passes: write whole files with write_file, or exact replacements with edit_file. Do not "+
		"only look, and do not run anything — this program runs it again when you have finished. "+
		"The game that was asked for: %s", task.Project, what, task.Goal)
}

// writerTools are what a model writing a project is given: its files and the
// engine's documentation, and nothing that runs, checks or builds them — that
// is this program's own work.
// With game_build in its hands a 1.5B model built the untouched template,
// twice, instead of writing the game.
var writerTools = []string{"inspect_project", "list_directory", "search_files", "read_file", "write_file", "edit_file",
	// Looking the engine's API up is writing, not running: the brief tells
	// a Godot writer to use the reference rather than its memory.
	"godot_docs", "game_docs"}

// writing is a step's tools cut down to writerTools. Never empty: an empty
// list means every tool.
func writing(only []string) []string {
	if len(only) == 0 {
		return writerTools
	}

	var out []string

	for _, name := range only {
		if slices.Contains(writerTools, name) {
			out = append(out, name)
		}
	}

	if len(out) == 0 {
		return []string{"list_directory"}
	}

	return out
}

// wroteIn is whether a piece of work changed any file.
func wroteIn(res agent.Result) bool {
	for _, s := range res.Steps {
		for _, e := range s.Evidence {
			if e.Kind == store.EvidenceFile && e.OK {
				return true
			}
		}
	}

	return false
}

// passedIn is whether a run's evidence shows the action passed.
func passedIn(evidence []store.Evidence, action string) bool {
	want := map[string][]string{
		"check": {store.EvidenceCheck},
		"smoke": {store.EvidenceSmoke},
		"build": {store.EvidenceBuild, store.EvidenceExport},
	}[action]

	for _, e := range evidence {
		for _, k := range want {
			if e.Kind == k {
				return e.OK
			}
		}
	}

	return false
}

// noteEngine records what an engine proved on this machine, for choosing it
// next time: a check, a run, a picture, a build.
func (c *Conductor) noteEngine(engine, action string, task *store.Task, step *store.TaskStep, passed bool,
	evidence []store.Evidence, out string) {
	if engine == "" {
		return
	}

	work := map[string]string{"check": "check", "smoke": "run", "build": "build"}[action]

	result := store.RunFailed
	if passed {
		result = store.RunOK
	}

	c.DB.RecordRun(store.ResourceRun{Resource: "engine:" + engine, Kind: "engine", Work: work, TaskID: task.ID,
		StepID: step.ID, Result: result, Detail: firstLine(out), Verified: passed})

	for _, e := range evidence {
		if e.Kind == store.EvidenceShot && e.OK {
			c.DB.RecordRun(store.ResourceRun{Resource: "engine:" + engine, Kind: "engine", Work: "picture",
				TaskID: task.ID, StepID: step.ID, Result: store.RunOK, Verified: true})
		}
	}
}

func orElse(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}

	return v
}
