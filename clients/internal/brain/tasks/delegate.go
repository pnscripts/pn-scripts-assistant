package tasks

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * When the person doing a step is not the one who should.
 *
 * Three moves, all made by the conductor rather than offered to a model as
 * tools — for the reason the planner is not a tool either: a tool is optional,
 * and "it said it would ask the specialist and it did not" is the failure
 * this program has fought hardest. So the code decides when, and the model
 * only ever does the work it is handed.
 *
 *   Hand on    A step its agent could not do goes to the best specialist on
 *              the organisation, as a task of its own. The parent waits for
 *              it and carries on with what came back.
 *   Escalate   When there is nobody better, it goes to whoever that agent's
 *              seat reports to, as the next step: here is what could not be
 *              done and why, decide how it should be.
 *   Review     A step at high or critical that did finish is looked at by
 *              somebody else before the plan moves on past it.
 *
 * The rule that makes all three safe is the one the organisation already
 * rests on: authority is a narrowing and never a widening. A specialist
 * handed a step may use its own tools only where they are also the tools of
 * the agent that handed it on, and may never do anything either was told
 * never to do. Handing work around the organisation can move who does it;
 * it cannot move what may be done.
 */

const (
	// MostDepth is how many handings-on deep a chain may go. Two is a
	// specialist asking a specialist; a third is an afternoon.
	MostDepth = 2

	// MostHandedOn is how many steps of one task may be handed on at all.
	MostHandedOn = 3
)

/*
 * specialistFor is who on the organisation is better placed for a step than
 * the agent who could not do it, or nobody.
 *
 * Asked of the same shortlist the planner is given, in code. Never the
 * generalist — handing a step from anybody to the person who does anything is
 * not finding a specialist, it is the fallback the step already had — and
 * never somebody already in this chain, or two agents could hand one step
 * back and forth until the budget ran out.
 */
func (c *Conductor) specialistFor(task *store.Task, step *store.TaskStep, exclude map[string]bool) (team.Agent, bool) {
	root := task.ID
	if top, err := c.DB.Root(task); err == nil && top != nil {
		root = top.ID
	}

	for _, candidate := range team.Who(c.roster(), c.chart(), c.Occupations, c.Toolbox,
		team.Wanted{Doing: step.Instruction, Kind: step.Kind, Record: c.record()}) {
		if candidate.Name == "assistant" || exclude[candidate.Name] || !availableFor(candidate, task, root) {
			continue
		}

		return candidate, true
	}

	return team.Agent{}, false
}

/*
 * availableFor is whether an agent may be given a piece of this task.
 *
 * Not one hired for another task only: it was hired for that, and is let go
 * with it. And for a project, not one who works under other packages than
 * the project's: a Godot game's step went to the three.js developer hired the
 * hour before, on the strength of the word "game".
 */
func availableFor(a team.Agent, task *store.Task, root int64) bool {
	if a.HiredFor != 0 && a.HiredFor != root && a.HiredFor != task.ID {
		return false
	}

	if task.Packages == "" || len(a.Packages) == 0 {
		return true
	}

	for _, p := range a.Packages {
		if slices.Contains(strings.Split(task.Packages, ","), p) {
			return true
		}
	}

	return false
}

// chainOf is everybody who has held this piece of work, up through the
// parents, so nobody in it is handed the step again.
func (c *Conductor) chainOf(task *store.Task, member team.Agent) map[string]bool {
	out := map[string]bool{member.Name: true}

	at := task

	for i := 0; at != nil && at.ParentStepID != 0 && i <= MostDepth; i++ {
		if step, err := c.DB.Step(at.ParentStepID); err == nil && step.Assignee != "" {
			out[step.Assignee] = true
		}

		parent, err := c.DB.Task(at.ParentTaskID)
		if err != nil {
			break
		}

		at = parent
	}

	return out
}

/*
 * handOn gives a step to a specialist as a task of its own. False means it
 * could not, and the caller carries on as if this had not been tried.
 *
 * The child is one step, the parent's step, done by the specialist. No
 * planning call: the step is already the size a step should be, and the
 * reason it failed is carried in so the specialist does not repeat it.
 */
func (c *Conductor) handOn(task *store.Task, step *store.TaskStep, member team.Agent, fit team.Fit, why string) (bool, error) {
	if task.Depth >= MostDepth {
		return false, nil
	}

	/*
	 * This program's own check, run or build is nobody's to be better at: it
	 * is the engine's answer, and the fixing between runs is already done by
	 * whoever the orchestrator chose. Handed on, it became a specialist asked
	 * to "check with game_check", who said it had — and the step counted as
	 * done on that word, twice in one Godot game.
	 */
	if step.Action != "" {
		return false, nil
	}

	children, err := c.DB.Children(task.ID)
	if err != nil || len(children) >= MostHandedOn {
		return false, err
	}

	chain := c.chainOf(task, member)

	specialist, ok := c.specialistFor(task, step, chain)

	// Nobody here, so perhaps somebody who could be. See hire.go.
	if !ok {
		specialist, ok = c.hireSpecialist(task, step, fit, chain)
	}

	if !ok {
		return false, nil
	}

	/*
	 * Half of what is left, from the parent's purse.
	 *
	 * Taken before the child exists, and refunded when it finishes with
	 * whatever it did not spend. A parent that cannot afford a child keeps
	 * the step and goes on to its other options.
	 */
	calls := task.CallsLeft / 2

	// The step and its one retry, which is what any step is allowed.
	steps := c.budget().MostAttempts

	if calls < 2 {
		return false, nil
	}

	afforded, err := c.DB.ShareBudget(task.ID, calls, steps)
	if err != nil || !afforded {
		return false, err
	}

	work, err := c.DB.NewTaskThread(specialist.Title + ": " + trimTo(step.Instruction, 50))
	if err != nil {
		return false, err
	}

	var goal strings.Builder

	goal.WriteString(step.Instruction)
	goal.WriteString("\n\nThis is one step of a larger job, handed to you because the " +
		strings.ToLower(orTitle(member)) + " could not do it: " + why)
	goal.WriteString("\nThe larger job: " + task.Goal)

	if established := c.established(task.ID); established != "" {
		goal.WriteString("\nWhat that job has established so far:\n" + established)
	}

	child, err := c.DB.NewTask(store.Task{
		Name:               "For " + task.Name + ": " + trimTo(step.Instruction, 60),
		Goal:               goal.String(),
		DoneWhen:           step.DoneWhen,
		State:              store.TaskWorking,
		ConversationID:     task.ConversationID,
		WorkConversationID: work,
		Provider:           task.Provider,
		StepsLeft:          steps,
		CallsLeft:          calls,
		Deadline:           task.Deadline,
		ParentTaskID:       task.ID,
		ParentStepID:       step.ID,
		Depth:              task.Depth + 1,
		Within:             narrowerOf(task.Within, fit),
		Never:              mergeLists(task.Never, fit.Never),
		Risk:               step.Risk,

		// A step of a project stays a step of that project: confined to its
		// folder, written by whoever the orchestrator chooses, judged on the
		// files. Without these a Godot game's step, handed on, was done with
		// none of that.
		Project:   task.Project,
		Packages:  task.Packages,
		Resources: task.Resources,
	})
	if err != nil {
		return false, err
	}

	if err := c.DB.AddSteps(child, []store.TaskStep{{
		Instruction: step.Instruction,
		DoneWhen:    step.DoneWhen,
		Kind:        step.Kind,
		Changes:     step.Changes,
		Assignee:    specialist.Name,

		// Never less serious than the step it is. The specialist may make it
		// more so, when it is known who they are.
		Risk: step.Risk,
	}}); err != nil {
		return false, err
	}

	if err := c.DB.HandOn(step.ID, child, member.Name, "handed to the "+strings.ToLower(orTitle(specialist))+
		" after: "+why); err != nil {
		return false, err
	}

	c.remember(member.Name, store.Learned, task, step,
		lessonOf(step, "went to the "+strings.ToLower(orTitle(specialist)), why))

	c.Log.Info("handed a step to a specialist",
		"task", task.ID, "step", step.ID, "from", member.Name, "to", specialist.Name, "child", child)

	c.say(task, fmt.Sprintf("%s: the %s could not do %q, so it has gone to the %s.",
		task.Name, strings.ToLower(orTitle(member)), trimTo(step.Instruction, 80),
		strings.ToLower(orTitle(specialist))))

	// Written last, after the sentence that explains it — see park.
	if err := c.DB.SetTaskState(task.ID, store.TaskWaiting,
		"waiting on the "+strings.ToLower(orTitle(specialist))); err != nil {
		return false, err
	}

	if c.Jobs != nil {
		job, err := c.Jobs.StartQueued("For "+task.Name, func(ctx context.Context) (string, error) {
			return "", c.Work(ctx, child)
		})
		if err != nil {
			c.DB.SetTaskState(child, store.TaskWaiting, plainly(err))

			return true, nil
		}

		c.DB.SetTaskJob(child, job.ID)
	}

	return true, nil
}

/*
 * narrowerOf is the most a child may use: what its parent was already held
 * to, narrowed again by the agent handing the step on.
 *
 * Nil is no limit, and stays nil only when both were unlimited. An agent
 * narrowed to nothing hands on a step that may use nothing — its work was
 * judgement, and a specialist doing it on its behalf is doing judgement too.
 */
func narrowerOf(within *[]string, fit team.Fit) *[]string {
	only, withTools := fit.Only()

	var mine *[]string

	switch {
	case !withTools:
		none := []string{}
		mine = &none
	case len(only) > 0:
		copied := append([]string{}, only...)
		mine = &copied
	}

	if within == nil {
		return mine
	}

	if mine == nil {
		copied := append([]string{}, (*within)...)

		return &copied
	}

	both := intersect(*within, *mine)

	return &both
}

// intersect keeps what is in both, in the first list's order.
func intersect(a, b []string) []string {
	in := map[string]bool{}

	for _, one := range b {
		in[one] = true
	}

	out := []string{}

	for _, one := range a {
		if in[one] {
			out = append(out, one)
		}
	}

	return out
}

func mergeLists(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}

	for _, one := range append(append([]string{}, a...), b...) {
		if one != "" && !seen[one] {
			seen[one] = true
			out = append(out, one)
		}
	}

	return out
}

/*
 * held is what a step may use once the task it belongs to is taken into
 * account: its agent's own tools, cut down to what the task was handed with.
 *
 * The same three states the agent's tools have — no limit, exactly these,
 * none — because the loop reads an empty list as "everything" and a child
 * handed on by an agent with no tools must not be given all of them.
 */
func held(task *store.Task, fit team.Fit) ([]string, bool) {
	only, withTools := fit.Only()

	if task.Within == nil {
		return only, withTools
	}

	if !withTools {
		return nil, false
	}

	if only == nil {
		limit := append([]string{}, (*task.Within)...)

		return limit, len(limit) > 0
	}

	both := intersect(only, *task.Within)

	return both, len(both) > 0
}

/*
 * childFinished carries a specialist's work back to the step it was for.
 *
 * Verified only if the specialist's own step was verified: a parent does not
 * get to call something established because a child said so. Whatever was
 * not spent goes back to the parent's budget, and the parent carries on from
 * where it was waiting.
 */
func (c *Conductor) childFinished(child *store.Task, state string) error {
	if child.ParentTaskID == 0 {
		return nil
	}

	fresh, err := c.DB.Task(child.ID)
	if err != nil || fresh == nil {
		return err
	}

	if err := c.DB.Refund(child.ParentTaskID, fresh.CallsLeft, fresh.StepsLeft); err != nil {
		c.Log.Warn("could not give a child's unspent budget back", "task", child.ID, "error", err)
	}

	step, err := c.DB.Step(child.ParentStepID)
	if err != nil || step == nil || step.State != store.StepHandedOn {
		return err
	}

	parent, err := c.DB.Task(child.ParentTaskID)
	if err != nil || parent == nil {
		return err
	}

	steps, err := c.DB.Steps(child.ID)
	if err != nil {
		return err
	}

	verified, done := false, false

	var answers, acted []string

	for _, s := range steps {
		if s.State == store.StepDone {
			done = true
			verified = verified || s.Verdict == store.Verified

			if s.Answer != "" {
				answers = append(answers, s.Answer)
			}
		}

		if s.Acted != "" {
			acted = append(acted, s.Acted)
		}

		// Whoever finally did it is who the parent's record names.
		if s.Assignee != "" {
			step.Assignee = s.Assignee
		}
	}

	step.Acted = withActed(step.Acted, acted)

	switch {
	case state == store.TaskDone && done:
		step.State = store.StepDone
		step.Answer = strings.Join(answers, "\n")
		step.Evidence = fresh.Report
		step.CheckedBy = "the specialist's own check"
		step.Verdict = store.Claimed
		step.Why = ""

		if verified {
			step.Verdict = store.Verified
		}
	default:
		step.State = store.StepFailed
		step.Verdict = store.Unmet
		step.Why = "the specialist it was handed to could not do it either: " + fresh.Because
	}

	if err := c.DB.FinishStep(*step); err != nil {
		return err
	}

	switch parent.State {
	case store.TaskStopped, store.TaskDone, store.TaskBlocked:
		return nil
	}

	if err := c.DB.SetTaskState(parent.ID, store.TaskWorking, ""); err != nil {
		return err
	}

	if c.Jobs == nil {
		return nil
	}

	job, err := c.Jobs.StartQueued(parent.Name, func(ctx context.Context) (string, error) {
		return "", c.Work(ctx, parent.ID)
	})
	if err != nil {
		return c.DB.SetTaskState(parent.ID, store.TaskWaiting, plainly(err))
	}

	return c.DB.SetTaskJob(parent.ID, job.ID)
}

/*
 * managerOf is who a working agent's seat reports to, as somebody who can be
 * given work: the nearest seat up the line that a working agent actually sits
 * in.
 *
 * A vacant seat is skipped rather than ending the search. An empty chair
 * between a developer and the head of engineering is not a reason for the
 * problem to go nowhere.
 */
func (c *Conductor) managerOf(member team.Agent) (team.Agent, bool) {
	if member.Position == "" {
		return team.Agent{}, false
	}

	roster := c.roster()

	for _, seat := range org.AnswersTo(c.chart(), member.Position) {
		for _, a := range roster {
			if a.Working() && a.Position == seat.Name && a.Name != member.Name {
				return a, true
			}
		}
	}

	return team.Agent{}, false
}

/*
 * escalate puts a failed step in front of the agent's manager, as the next
 * step. False means there is nobody to escalate to.
 *
 * Once. The manager's step is marked as an escalation, and one that fails is
 * not escalated again — up a chain of four seats is how a stuck step becomes
 * an afternoon of people saying they do not know either. From there it is its
 * owner's, which is where every problem this program cannot solve ends up.
 */
func (c *Conductor) escalate(task *store.Task, step *store.TaskStep, member team.Agent, why string) (bool, error) {
	if step.EscalatedFrom != 0 || step.ReviewOf != 0 {
		return false, nil
	}

	manager, ok := c.managerOf(member)
	if !ok {
		return false, nil
	}

	instruction := fmt.Sprintf("The %s could not do this step: %q. What went wrong: %s. "+
		"Decide how it should be done and do it — or, if it cannot be done without "+
		"the owner, say exactly what is needed from them.",
		strings.ToLower(orTitle(member)), step.Instruction, why)

	if _, err := c.DB.InsertStepAfter(task.ID, step.Position, store.TaskStep{
		Instruction:   instruction,
		DoneWhen:      step.DoneWhen,
		Kind:          step.Kind,
		Changes:       step.Changes,
		Assignee:      manager.Name,
		Risk:          step.Risk,
		EscalatedFrom: step.ID,
	}); err != nil {
		return false, err
	}

	c.remember(member.Name, store.Learned, task, step,
		lessonOf(step, "went up to the "+strings.ToLower(orTitle(manager)), why))

	c.Log.Info("escalated a step to a manager",
		"task", task.ID, "step", step.ID, "from", member.Name, "to", manager.Name)

	c.say(task, fmt.Sprintf("%s: the %s could not do %q, so it has gone up to the %s.",
		task.Name, strings.ToLower(orTitle(member)), trimTo(step.Instruction, 80),
		strings.ToLower(orTitle(manager))))

	return true, nil
}

/*
 * review puts a finished high or critical step in front of somebody else.
 *
 * The reviewer is the doer's manager when there is one, and otherwise the
 * best other person for the work. Reviews are never reviewed and escalations'
 * reviews are not escalated: one extra look at the steps that matter, and not
 * a committee.
 */
func (c *Conductor) review(task *store.Task, step *store.TaskStep, member team.Agent) error {
	if step.ReviewOf != 0 || !risk.Parse(step.Risk).AtLeast(risk.High) {
		return nil
	}

	if step.Kind == store.StepLook || step.Kind == store.StepCheck {
		return nil
	}

	reviewer, ok := c.managerOf(member)

	if !ok {
		reviewer, ok = c.specialistFor(task, step, map[string]bool{member.Name: true})
	}

	if !ok {
		return nil
	}

	instruction := fmt.Sprintf("Review what the %s did for this step: %q.\nWhat they said: %s\n"+
		"Begin your answer with \"Right:\" if it is correct and safe, or \"Wrong:\" and "+
		"exactly what is wrong if it is not. Do not redo the work.",
		strings.ToLower(orTitle(member)), step.Instruction, trimTo(step.Answer, 600))

	_, err := c.DB.InsertStepAfter(task.ID, step.Position, store.TaskStep{
		Instruction: instruction,
		DoneWhen:    "a clear verdict of right or wrong, with the reason",
		Kind:        store.StepCheck,
		Assignee:    reviewer.Name,
		Risk:        string(risk.Low),
		ReviewOf:    step.ID,
	})

	return err
}

/*
 * reviewed carries a reviewer's verdict back to the step it was about.
 *
 * "Wrong" takes the step's verdict away. The step still happened, and the
 * report says so — but what it rests on is no longer established, so no later
 * step builds on it and nobody reads the account as though it went well.
 */
func (c *Conductor) reviewed(review *store.TaskStep) error {
	if review.ReviewOf == 0 {
		return nil
	}

	verdict := strings.ToLower(strings.TrimSpace(review.Answer))

	if !strings.HasPrefix(verdict, "wrong") {
		return nil
	}

	reviewed, err := c.DB.Step(review.ReviewOf)
	if err != nil || reviewed == nil {
		return err
	}

	reviewed.Verdict = store.Unmet
	reviewed.Why = "the " + strings.ToLower(review.Assignee) + " reviewed it: " + trimTo(review.Answer, 300)

	// The lesson worth most: what somebody else found wrong with its work.
	if task, err := c.DB.Task(reviewed.TaskID); err == nil && task != nil {
		c.remember(reviewed.Assignee, store.Learned, task, reviewed,
			fmt.Sprintf("a review of %q found it wrong: %s", trimTo(reviewed.Instruction, 120),
				trimTo(review.Answer, 200)))
	}

	return c.DB.FinishStep(*reviewed)
}

func orTitle(a team.Agent) string {
	if a.Title != "" {
		return a.Title
	}

	return a.Name
}
