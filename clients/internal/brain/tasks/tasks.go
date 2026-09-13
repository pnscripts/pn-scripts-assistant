/*
 * Package tasks turns a request into work that finishes.
 *
 * The agent loop answers a turn: ask the model, run what it asks for, feed the
 * result back, stop. That is the right shape for a question and the wrong one
 * for a job, because a job is several turns with judgement between them — what
 * to do next, whether the last thing worked, whether what was found changes
 * the plan. Eight rounds of one loop cannot hold that, and stretching it to
 * sixty would only mean the cap that stops a small model looping forever no
 * longer stops anything.
 *
 * So this sits above the loop rather than inside it. One turn per step, which
 * keeps every property the loop already has — the approval gate, the recovery
 * of a call written as prose, the refusal of what privacy forbids — and adds
 * only what a job needs: a plan, a record, a budget, and a check.
 */
package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/jobs"
	"pn-scripts-assistant/internal/brain/lanes"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Budget is every way a task is allowed to end without finishing.
 *
 * Five of them, and all five are persisted on the task's row rather than held
 * here. A budget in memory refills itself when the program restarts, and a
 * task that resumes with a full purse has no budget at all — which is the
 * failure that matters, because the whole point of a budget is the case where
 * something has gone wrong and nobody is watching.
 */
type Budget struct {
	MostSteps    int
	MostAttempts int
	MostReplans  int
	MostCalls    int
	HowLong      time.Duration
}

/*
 * TheOwnersOwn are the tools a task may never use, however wide its agent is.
 *
 * One entry, and it is worth the whole mechanism. decide_waiting carries out
 * the actions the approval gate is holding, and it is marked Safe for a good
 * reason: in a conversation it runs because its owner asked for it in the
 * sentence they just spoke, and making it need approval would mean an
 * approval needing an approval.
 *
 * A task breaks that reasoning in half. The sentence a step works from was
 * written by the planner rather than by anybody, so an agent reaching for
 * this would be approving — unattended — the very things its owner was asked
 * about. The generalist has no tool list at all, which is exactly why this
 * cannot be expressed as one.
 *
 * A task says what needs deciding and leaves it. Which is what it would have
 * had to do anyway, and is the whole shape of park and resume.
 */
var TheOwnersOwn = []string{"decide_waiting"}

// Sensible is what a task gets unless somebody says otherwise. Small, because
// the machine is one processor and the first version of anything like this
// should stop too early rather than too late.
func Sensible() Budget {
	return Budget{
		MostSteps:    12,
		MostAttempts: 2,
		MostReplans:  1,
		MostCalls:    40,
		HowLong:      30 * time.Minute,
	}
}

type Conductor struct {
	DB   *store.DB
	Log  *slog.Logger
	Jobs *jobs.Runner

	// Agent is the quiet loop. One turn of it is one step.
	Agent *agent.Loop

	/*
	 * Provider is asked for afresh at every step and never held.
	 *
	 * The router is the one place privacy is enforced, and it is enforced by
	 * being asked. A conductor holding a provider it obtained twenty minutes
	 * ago is a task that carries on sending to a hosted model after somebody
	 * has switched to private — which is the disclosure the setting exists to
	 * prevent, arriving late.
	 */
	Provider func(name string) (llm.Provider, error)

	// Sizes is which model fills which role on this machine.
	Sizes func() llm.Sizes

	/*
	 * Roster is who is available to do the work.
	 *
	 * A function rather than a list because it is read from files its owner
	 * can edit, and somebody who changes which model the developer uses should
	 * not have to restart the program to see it take effect.
	 */
	Roster func() []team.Agent

	/*
	 * What the organisation looks like, what the jobs are, and what tools
	 * exist — the three things an agent has to be resolved against before it
	 * can be given a step.
	 *
	 * All three optional. Without them an agent is what it was before there
	 * was an organisation: a name, a description and whatever tool list it
	 * was written with. That is not a degraded mode; it is the one every
	 * roster file written so far is in.
	 */
	Chart       func() []org.Unit
	Occupations team.Occupations
	Toolbox     team.Toolbox

	/*
	 * Prompt is the persona for a step, given the model that will answer it.
	 *
	 * The provider is a parameter because what may be told to a model depends
	 * on which one it is: what its owner wrote about themselves goes to the
	 * one on this machine in every mode, and to a paid service only if they
	 * have said so. A step doing somebody's work needs to know whose it is,
	 * and that is exactly the part that must not be sent by accident.
	 */
	Prompt func(provider string) string

	/*
	 * Say puts a line in the thread that asked, and reads it out if somebody
	 * is listening.
	 *
	 * The conversation is a parameter rather than a guess. A background job
	 * that finishes has to guess and picks the latest thread; a task recorded
	 * which one asked for it when it started, so a report can land under the
	 * question rather than under whatever was being said half an hour later.
	 */
	Say func(conversationID int64, line string)

	Budget Budget

	/*
	 * Lanes is how many model calls may run at once, and who is waiting.
	 *
	 * Nil runs every call as it comes, which is what a test with a scripted
	 * model wants and what this program did before there were lanes.
	 */
	Lanes *lanes.Lanes

	// Now exists so the deadline can be tested without waiting half an hour.
	Now func() time.Time
}

func (c *Conductor) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}

	return time.Now()
}

func (c *Conductor) say(task *store.Task, line string) {
	if c.Say != nil {
		c.Say(task.ConversationID, line)
	}
}

/*
 * Take turns a request into work and starts it.
 *
 * The second return says whether this was a job at all. False means the
 * request is an ordinary question and the caller should answer it exactly as
 * it always did — no task row is written, and the whole attempt has cost one
 * short model call. That asymmetry is deliberate: missing a job costs nothing,
 * because its owner can say "do that as a task", where taking a greeting for a
 * job produces a panel with "good morning" in it.
 *
 * forced is somebody having said so outright. It is the difference between a
 * guess and an instruction, and it is why a one-step plan can still be a task:
 * a guess that small should be dropped, and an instruction should not be
 * argued with.
 */
func (c *Conductor) Take(ctx context.Context, conversationID int64, request, provider string, forced bool) (*store.Task, bool, error) {
	plan, ok, err := c.plan(ctx, request, provider, forced)
	if err != nil || !ok {
		return nil, false, err
	}

	budget := c.budget()

	// How serious it is, decided before anything is written or started.
	serious := c.weighPlan(plan.Steps)

	work, err := c.DB.NewTaskThread(plan.Name)
	if err != nil {
		return nil, false, err
	}

	id, err := c.DB.NewTask(store.Task{
		Name:               plan.Name,
		Goal:               request,
		DoneWhen:           plan.DoneWhen,
		State:              store.TaskWorking,
		ConversationID:     conversationID,
		WorkConversationID: work,
		Provider:           provider,
		StepsLeft:          budget.MostSteps,
		CallsLeft:          budget.MostCalls,
		ReplansLeft:        budget.MostReplans,
		Deadline:           c.now().Add(budget.HowLong),
	})
	if err == nil {
		err = c.DB.RaiseRisk(id, 0, string(serious))
	}
	if err != nil {
		return nil, false, err
	}

	if err := c.DB.AddSteps(id, plan.Steps); err != nil {
		return nil, false, err
	}

	/*
	 * Silent, because the task writes its own account when it finishes.
	 *
	 * And on the Runner rather than the turn: a task takes minutes, and the
	 * whole point of it is that its owner can carry on talking while it runs.
	 */
	/*
	 * Read before the work starts, not after.
	 *
	 * What this returns is the plan as it was written down, which is what the
	 * caller is asking for: "here is what I am going to do". Reading it back
	 * afterwards raced the goroutine that had already begun — under load a
	 * step could fail, the task replan, and the caller be handed six steps for
	 * a three-step plan. It is also simply the wrong answer: by then it is no
	 * longer the plan, it is the progress.
	 */
	task, err := c.DB.TaskWithSteps(id)
	if err != nil {
		return nil, true, err
	}

	job, err := c.Jobs.StartQueued(plan.Name, func(ctx context.Context) (string, error) {
		return "", c.Work(ctx, id)
	})
	if err != nil {
		// The machine is already busy. The task stays written down, parked,
		// so it can be started from its own view rather than lost.
		c.DB.SetTaskState(id, store.TaskWaiting, plainly(err))

		task.State, task.Because = store.TaskWaiting, plainly(err)

		return task, true, nil
	}

	if err := c.DB.SetTaskJob(id, job.ID); err != nil {
		c.Log.Warn("could not record which job is running a task", "task", id, "error", err)
	}

	task.JobID = job.ID

	return task, true, nil
}

/*
 * TakeForGoal starts a piece of work towards a standing intention.
 *
 * The goal's own words are the job, because they are what somebody wrote when
 * they were thinking about it rather than about this particular Tuesday.
 * Whether it produced anything or not, the goal is marked as worked on: a goal
 * that stayed due because its last attempt found nothing to do would come
 * round again immediately and keep coming round.
 */
func (c *Conductor) TakeForGoal(ctx context.Context, goal store.Goal, conversationID int64, provider string) (*store.Task, bool, error) {
	request := strings.TrimSpace(goal.Name)

	if why := strings.TrimSpace(goal.Why); why != "" {
		request = strings.TrimSuffix(request, ".") + ". " + why
	}

	task, started, err := c.Take(ctx, conversationID, request, provider, true)

	if err := c.DB.GoalWorkedOn(goal.ID, c.now()); err != nil {
		c.Log.Warn("could not record that a goal was worked on", "goal", goal.ID, "error", err)
	}

	if err != nil || !started || task == nil {
		return task, started, err
	}

	if err := c.DB.SetTaskGoal(task.ID, goal.ID); err != nil {
		c.Log.Warn("could not tie a task to its goal", "task", task.ID, "error", err)
	}

	return task, started, nil
}

func (c *Conductor) budget() Budget {
	b := c.Budget

	if b.MostSteps <= 0 || b.MostCalls <= 0 || b.HowLong <= 0 {
		return Sensible()
	}

	return b
}

/*
 * Work runs a task to done, blocked, or out of budget.
 *
 * Called on the Runner's goroutine, and again on resume. Everything it needs
 * is read from the row at the top of each turn round the loop, so it does not
 * matter whether this is the first time or the third.
 */
func (c *Conductor) Work(ctx context.Context, taskID int64) error {
	for {
		if err := ctx.Err(); err != nil {
			// Stopped from the interface, or the program is closing. The row
			// keeps whatever it had; the startup sweep parks it.
			return nil
		}

		task, err := c.DB.Task(taskID)
		if err != nil {
			return err
		}

		if task == nil {
			return fmt.Errorf("task %d is gone", taskID)
		}

		switch task.State {
		case store.TaskStopped, store.TaskDone, store.TaskBlocked:
			return nil
		}

		if why := c.outOfBudget(task); why != "" {
			return c.finish(task, store.TaskBlocked, why)
		}

		step, err := c.DB.NextStep(taskID)
		if err != nil {
			return err
		}

		if step == nil {
			return c.finish(task, store.TaskDone, "")
		}

		/*
		 * The step allowance is for steps still to come, so it is asked only
		 * once there is one.
		 *
		 * Checked with the others, a plan that used exactly the steps it was
		 * allowed finished its last one, came back round, found the allowance
		 * at nought and was reported stuck — with nothing left to do. Time and
		 * thinking stay checked first: those can be overspent inside a step,
		 * and a task that did is stopped whatever is left.
		 */
		if task.StepsLeft <= 0 {
			return c.finish(task, store.TaskBlocked, "it reached the most steps it was allowed for one job")
		}

		/*
		 * The step and any after it that the plan said stand alone, at once.
		 *
		 * Only as many as there are steps left to spend. One is the ordinary
		 * case and runs exactly as a step always did.
		 */
		group, err := c.together(taskID, step, task.StepsLeft)
		if err != nil {
			return err
		}

		var carryOn bool

		if len(group) == 1 {
			carryOn, err = c.runStep(ctx, task, step)
		} else {
			carryOn, err = c.runTogether(ctx, task, group)
		}

		if err != nil {
			return err
		}

		if !carryOn {
			return nil
		}
	}
}

/*
 * together is the next step and the waiting steps straight after it that were
 * planned to stand alone, up to what may be spent.
 */
func (c *Conductor) together(taskID int64, first *store.TaskStep, most int) ([]*store.TaskStep, error) {
	group := []*store.TaskStep{first}

	steps, err := c.DB.Steps(taskID)
	if err != nil {
		return nil, err
	}

	for i, s := range steps {
		if s.ID != first.ID {
			continue
		}

		for _, next := range steps[i+1:] {
			if !next.Together || next.State != store.StepWaiting || len(group) >= most {
				break
			}

			next := next
			group = append(group, &next)
		}

		break
	}

	return group, nil
}

/*
 * runTogether works on several steps at once.
 *
 * Prepared in order, thought about side by side — each model call taking its
 * turn in its lane, so on this machine they still go one at a time and on a
 * hosted service they genuinely overlap — and settled in order again, because
 * settling is where a task retries, hands on, escalates and replans, and two
 * steps doing that at once would be two steps changing one plan.
 *
 * Once one of them stops the task, the rest are only written down: what they
 * found is kept, a question they raised is linked so its answer still reaches
 * the task, and a step that neither finished nor asked goes back to waiting to
 * be done when the task picks up again.
 */
func (c *Conductor) runTogether(ctx context.Context, task *store.Task, group []*store.TaskStep) (bool, error) {
	turns := []*turn{}

	for _, step := range group {
		t, err := c.prepare(task, step)
		if t == nil {
			if err != nil {
				return false, err
			}

			break
		}

		turns = append(turns, t)
	}

	if len(turns) == 0 {
		return false, nil
	}

	var wg sync.WaitGroup

	for _, t := range turns {
		wg.Add(1)

		go func(t *turn) {
			defer wg.Done()

			c.think(ctx, task, t)
		}(t)
	}

	wg.Wait()

	carryOn := true

	for _, t := range turns {
		fresh, err := c.DB.Task(task.ID)
		if err != nil {
			return false, err
		}

		if carryOn && fresh.State == store.TaskWorking {
			more, err := c.settle(ctx, fresh, t)
			if err != nil {
				return false, err
			}

			carryOn = more

			continue
		}

		if err := c.setDown(fresh, t); err != nil {
			return false, err
		}
	}

	return carryOn, nil
}

// setDown records a step that ran beside one that stopped the task. See
// runTogether.
func (c *Conductor) setDown(task *store.Task, t *turn) error {
	step := t.step

	if t.res.WaitingForApproval() {
		for _, p := range t.res.Pending {
			if err := c.DB.LinkInvocationToStep(p.ID, task.ID, step.ID); err != nil {
				return err
			}
		}

		step.State = store.StepNeedsYou
		step.Answer = t.res.Reply

		return c.DB.FinishStep(*step)
	}

	step.State = store.StepWaiting
	step.Answer = t.res.Reply

	return c.DB.FinishStep(*step)
}

/*
 * inLane is a provider whose calls queue in the right lane, named for the
 * person and the step, so the view can say who has the model and who is
 * waiting for it.
 */
func (c *Conductor) inLane(provider llm.Provider, member team.Agent, step *store.TaskStep) llm.Provider {
	if c.Lanes == nil {
		return provider
	}

	return lanes.Provider{Provider: provider, Lanes: c.Lanes, Seat: lanes.Seat{
		Who:  orTitle(member),
		What: trimTo(step.Instruction, 80),
		Key:  fmt.Sprintf("step:%d", step.ID),
	}}
}

// asking is a provider for the conductor's own calls — planning and checking
// — queued like everybody else's.
func (c *Conductor) asking(provider llm.Provider, who, what string) llm.Provider {
	if c.Lanes == nil {
		return provider
	}

	return lanes.Provider{Provider: provider, Lanes: c.Lanes, Seat: lanes.Seat{
		Who: who, What: trimTo(what, 80)}}
}

// outOfBudget says which allowance ran out, in the owner's terms rather than
// the program's.
func (c *Conductor) outOfBudget(task *store.Task) string {
	switch {
	case !task.Deadline.IsZero() && c.now().After(task.Deadline):
		return "it reached the half hour it was given and was not finished"
	case task.CallsLeft <= 0:
		return "it used up the thinking it was given for this"
	}

	return ""
}

/*
 * runStep is one step, which is one turn of the agent loop.
 *
 * The false return means stop rather than fail: a step that reached something
 * needing a decision has parked, and the task carries on when that decision is
 * made rather than when this function is called again.
 */
func (c *Conductor) runStep(ctx context.Context, task *store.Task, step *store.TaskStep) (bool, error) {
	t, err := c.prepare(task, step)
	if t == nil {
		return false, err
	}

	c.think(ctx, task, t)

	return c.settle(ctx, task, t)
}

/*
 * A turn is one step being worked on, from who is doing it to what came back.
 *
 * Split in three so that several can think at once while everything that
 * decides what happens next still happens one step at a time. Preparing
 * touches the row and the budget; thinking is the long part, and is all a
 * model call; settling checks, retries, hands on, escalates, reviews and
 * replans — any of which can change the task, and none of which may race
 * another step doing the same.
 */
type turn struct {
	step     *store.TaskStep
	member   team.Agent
	fit      team.Fit
	provider llm.Provider
	brief    agent.Brief

	res agent.Result
	err error
}

// prepare decides who does a step, with what, through which service. Nil
// means it could not be started, and the task has already been told why.
func (c *Conductor) prepare(task *store.Task, step *store.TaskStep) (*turn, error) {
	if c.Agent == nil || c.Provider == nil {
		return nil, c.finish(task, store.TaskBlocked, "this build cannot run tasks")
	}

	/*
	 * Who does this one.
	 *
	 * The planner's choice when it named somebody real, and worked out from
	 * the step when it did not — the same trust-but-validate shape as
	 * everything else the planner writes. A model inventing a job title costs
	 * nothing more than the step being done by the generalist.
	 */
	member := team.For(c.roster(), step.Assignee, step.Kind, step.Instruction)

	/*
	 * An agent may be pinned to a service, and it is still asked for by name.
	 *
	 * Several agents must still mean one choke point. Everything goes through
	 * the router, so a roster cannot become a way of reaching a provider that
	 * privacy forbids — the agent names who it wants and the router decides
	 * whether it may have them.
	 */
	// Which service, from the agent and the task together — and an agent that
	// must stay on this machine is held here whatever the task was started
	// with. A narrowing, like everything else about an agent.
	using := member.Through(task.Provider)

	choice := c.choose(step, member, using)

	provider, err := c.Provider(using)

	/*
	 * A service that cannot be reached falls back to this machine, once.
	 *
	 * A key was removed, a company is having an outage, the network is down —
	 * none of those are reasons to end a job that is otherwise fine, and a
	 * task that blocks at step four of nine because somebody's API was down
	 * has thrown away four steps of real work.
	 *
	 * Privacy refusing is the exception, and it is not a failure at all: it is
	 * a decision somebody made, and carrying on regardless would be overruling
	 * them in the one part of this program where that is unacceptable. That
	 * ends the task, and the report says which provider and why.
	 *
	 * Otherwise, only ever towards this machine and never away from it. The
	 * local provider is the one privacy never forbids, so this can only narrow
	 * where the work goes.
	 */
	if err != nil && using != llm.Local && !llm.Forbidden(err) {
		here, hereErr := c.Provider(llm.Local)

		if hereErr == nil {
			c.Log.Info("a service could not be reached, so the step stayed here",
				"task", task.ID, "wanted", using, "why", plainly(err))

			provider, err, using = here, nil, llm.Local
			choice = c.choose(step, member, using)
		}
	}

	if err != nil {
		// Nothing can answer at all. Stopping at the next step is the point of
		// asking every time.
		return nil, c.finish(task, store.TaskBlocked, plainly(err))
	}

	if err := c.DB.StartStep(step.ID, member.Name, provider.Name(), choice.Model); err != nil {
		return nil, err
	}

	step.Assignee = member.Name

	/*
	 * And how serious it is, now that it is known who is doing it.
	 *
	 * The planner's name for the doer was a guess and this is who turned up.
	 * A lawyer taking a step planned for the generalist makes it more
	 * serious, never less — so it is raised on the row as well, and the task
	 * with it, before any tool can run.
	 */
	if now := risk.Max(risk.Parse(step.Risk), c.weigh(*step, member)); now != risk.Parse(step.Risk) {
		step.Risk = string(now)

		if err := c.DB.RaiseRisk(task.ID, step.ID, step.Risk); err != nil {
			return nil, err
		}
	}

	/*
	 * The row counted this attempt, so the struct has to as well.
	 *
	 * Left at what NextStep read, the comparison below is always one behind:
	 * an allowance of one attempt gave two, and of two gave three. The extra
	 * attempt is minutes of a processor, and the step that runs with an empty
	 * script fails for a different reason than the real one — which is what
	 * ends up in the report.
	 */
	step.Attempts++

	if _, err := c.DB.SpendAStep(task.ID); err != nil {
		return nil, err
	}

	/*
	 * Who this one actually is, once the organisation and the taxonomy have
	 * been consulted: its job's capabilities, its seat's, its own, and the
	 * tools all of that adds up to.
	 *
	 * Worked out here rather than once at the top, because the roster and the
	 * chart are both files somebody may edit while a task is running — and a
	 * task holding who somebody was twenty minutes ago is the same mistake as
	 * a task holding a provider from twenty minutes ago.
	 */
	fit := team.Settle(member, c.chart(), c.Occupations, c.Toolbox)

	/*
	 * What it may use: its own tools, cut down to what this task was handed
	 * with when it is somebody else's step — and, for a review, only the
	 * tools that look. A reviewer who could change what it is reviewing is
	 * not reviewing it.
	 */
	only, withTools := held(task, fit)

	if step.ReviewOf != 0 {
		only, withTools = c.lookingOnly(only, withTools)
	}

	return &turn{
		step:     step,
		member:   member,
		fit:      fit,
		provider: c.inLane(provider, member, step),
		brief: agent.Brief{
			Choice: choice, WithTools: choice.Tools && withTools,
			Only:  only,
			Never: mergeLists(mergeLists(fit.Never, task.Never), TheOwnersOwn),
			As:    member.Name,
			Risk:  risk.Parse(step.Risk),
		},
	}, nil
}

// think is the model call, which is the part worth running side by side.
func (c *Conductor) think(ctx context.Context, task *store.Task, t *turn) {
	t.res, t.err = c.Agent.RunBrief(ctx, task.WorkConversationID, t.provider,
		c.brief(task, t.step, t.member), t.brief)

	// Whatever ran at high or critical without a question, kept on the row
	// whether or not the step goes on to succeed. It happened either way.
	t.step.Acted = withActed(t.step.Acted, actedUnasked(t.res))

	// Spent whether or not it worked. A call that failed still cost the time.
	if _, spendErr := c.DB.SpendOnTask(task.ID, atLeastOne(t.res.Rounds)); spendErr != nil {
		c.Log.Warn("could not record what a step cost", "task", task.ID, "error", spendErr)
	}
}

// settle is everything that follows from what came back.
func (c *Conductor) settle(ctx context.Context, task *store.Task, t *turn) (bool, error) {
	step, member, fit, res, err := t.step, t.member, t.fit, t.res, t.err

	if err != nil {
		step.State = store.StepFailed
		step.Why = plainly(err)

		if saveErr := c.DB.FinishStep(*step); saveErr != nil {
			return false, saveErr
		}

		return false, c.finish(task, store.TaskBlocked, plainly(err))
	}

	if res.WaitingForApproval() {
		return false, c.park(task, step, res)
	}

	if res.Asked {
		// It stopped and asked rather than guessing, which is a step doing the
		// right thing. The task waits for an answer like any other.
		step.State = store.StepNeedsYou
		step.Answer = res.Reply

		if err := c.DB.FinishStep(*step); err != nil {
			return false, err
		}

		// Said before the row says waiting, for the same reason the report is:
		// anything polling would otherwise see a task stop with no sign of
		// what it was stopping for.
		c.say(task, task.Name+" has a question: "+res.Reply)

		if err := c.DB.SetTaskState(task.ID, store.TaskWaiting, "it asked you something"); err != nil {
			return false, err
		}

		return false, nil
	}

	checked := c.check(ctx, task, step, res)

	if checked.Calls > 0 {
		if _, err := c.DB.SpendOnTask(task.ID, checked.Calls); err != nil {
			c.Log.Warn("could not record what a check cost", "task", task.ID, "error", err)
		}
	}

	step.Answer = res.Reply
	step.Evidence = checked.Evidence
	step.CheckedBy = checked.CheckedBy
	step.Verdict = checked.Verdict
	step.Why = checked.Why

	if checked.Verdict != store.Unmet {
		step.State = store.StepDone

		if err := c.DB.FinishStep(*step); err != nil {
			return false, err
		}

		// A review says what it found about the step it reviewed; anything
		// else serious enough is looked at by somebody else before the plan
		// moves on past it.
		if step.ReviewOf != 0 {
			return true, c.reviewed(step)
		}

		return true, c.review(task, step, member)
	}

	/*
	 * It did not do what it was for, so it is tried again — once.
	 *
	 * The reason is kept on the row and handed back on the next attempt, so
	 * the second try is told what was wrong with the first rather than making
	 * the same one. A third attempt is not offered: a model that has failed
	 * the same step twice is not one attempt away, and each one is minutes.
	 */
	if step.Attempts < c.budget().MostAttempts {
		step.State = store.StepWaiting

		return true, c.DB.FinishStep(*step)
	}

	/*
	 * Before giving up on it: somebody better placed, then somebody above.
	 *
	 * In that order because a specialist is an answer and a manager is a
	 * question. What was done on the way — including anything that ran
	 * without asking — is written down first, so handing the step on does
	 * not lose the record of the attempt.
	 */
	step.State = store.StepRunning

	if err := c.DB.FinishStep(*step); err != nil {
		return false, err
	}

	handed, err := c.handOn(task, step, member, fit, checked.Why)
	if err != nil || handed {
		return false, err
	}

	step.State = store.StepFailed

	if err := c.DB.FinishStep(*step); err != nil {
		return false, err
	}

	escalated, err := c.escalate(task, step, member, checked.Why)
	if err != nil || escalated {
		return escalated, err
	}

	/*
	 * Once, and only once, the rest of the plan is reconsidered.
	 *
	 * A step failing twice usually means the plan was wrong rather than the
	 * model — it asked for something that is not there, or in an order that
	 * cannot work. Replanning is given what has actually been established,
	 * never what was merely claimed, so it does not build three more steps on
	 * something that never happened.
	 */
	if task.ReplansLeft > 0 {
		replanned, err := c.replan(ctx, task, step)
		if err != nil {
			return false, err
		}

		if replanned {
			return true, nil
		}
	}

	return false, c.finish(task, store.TaskBlocked, checked.Why)
}

/*
 * replan asks for the rest of the job again, knowing what went wrong.
 *
 * Only steps not yet done, and still inside the same allowance — a replan is
 * not a way to buy more budget. False means it could not produce one, and the
 * task stops rather than trying a third time.
 */
func (c *Conductor) replan(ctx context.Context, task *store.Task, failed *store.TaskStep) (bool, error) {
	if _, err := c.DB.SpendReplan(task.ID); err != nil {
		return false, err
	}

	var b strings.Builder

	b.WriteString(task.Goal)
	b.WriteString("\n\nA previous attempt got stuck. What is already established:\n")

	if established := c.established(task.ID); established != "" {
		b.WriteString(established)
	} else {
		b.WriteString("- nothing yet")
	}

	b.WriteString("\n\nThe step that could not be done, and why:\n- " +
		failed.Instruction + " — " + failed.Why +
		"\n\nPlan the rest of the job a different way. Do not plan that step " +
		"again as it stands.")

	plan, ok, err := c.plan(ctx, b.String(), task.Provider, true)
	if err != nil || !ok || len(plan.Steps) == 0 {
		return false, err
	}

	/*
	 * The old rest of the plan is put aside before the new one is written.
	 *
	 * Without this a replan was not a replan: the steps nobody had started yet
	 * stayed waiting, so the task worked through the plan that had just got
	 * stuck and only then reached the new one. "Plan the rest of the job a
	 * different way" and then doing it the old way first is the worst of both.
	 *
	 * Set aside rather than deleted, because what was going to be done and why
	 * it stopped being the plan is worth keeping, and because nothing else in
	 * this program deletes a record to change its mind.
	 */
	put, err := c.DB.SetAsideUnstartedSteps(task.ID, "the job was planned again after: "+failed.Why)
	if err != nil {
		return false, err
	}

	serious := c.weighPlan(plan.Steps)

	if err := c.DB.AddSteps(task.ID, plan.Steps); err != nil {
		return false, err
	}

	if err := c.DB.RaiseRisk(task.ID, 0, string(serious)); err != nil {
		return false, err
	}

	c.Log.Info("replanned a task that got stuck",
		"task", task.ID, "steps", len(plan.Steps), "set aside", put)

	return true, nil
}

func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}

	return n
}

/*
 * park stops a step on somebody's decision without failing it.
 *
 * awaitApproval ends the turn, which for a conversation is the whole story:
 * the person answers and asks again. A task has a quarter of an hour behind it
 * and nobody to ask, so the decision is linked to the step and the work
 * carries on from there when it is made.
 */
func (c *Conductor) park(task *store.Task, step *store.TaskStep, res agent.Result) error {
	for _, p := range res.Pending {
		if err := c.DB.LinkInvocationToStep(p.ID, task.ID, step.ID); err != nil {
			return err
		}
	}

	step.State = store.StepNeedsYou
	step.Answer = res.Reply

	if err := c.DB.FinishStep(*step); err != nil {
		return err
	}

	var what []string
	for _, p := range res.Pending {
		what = append(what, p.Summary)
	}

	/*
	 * Said first, then parked.
	 *
	 * Every state a task stops in is written last, after whatever explains it
	 * has been delivered. The other order is a race with anything watching:
	 * the row says waiting, and the sentence saying what it is waiting for is
	 * not there yet.
	 */
	c.say(task, task.Name+" needs your decision before it can go on: "+strings.Join(what, "; "))

	return c.DB.SetTaskState(task.ID, store.TaskWaiting, "it needs your decision")
}

/*
 * choose picks the model for one step, which is the agent's own.
 *
 * Every model is good at something different, and on this machine the
 * differences are minutes rather than nuances: the coding model and the
 * fastest model are not close, and the one that writes best prose is neither.
 * A researcher reading six pages wants the quick one; a writer producing the
 * thing somebody will actually read wants the best one. Deciding it per agent
 * rather than per sentence is the whole of what the roster buys.
 *
 * The loop still holds that choice for the whole turn. A step is a turn, so
 * the rule that the model reading a tool's answer is the one that asked for it
 * survives exactly as it was.
 */
/*
 * choose is which model this step is thought with, on the service it is asked
 * through.
 *
 * The service is decided first and the model second, which is the way round it
 * has to be: the size roles resolve to what is installed on this machine, so
 * they are Ollama tags. Handing one of those to Anthropic was sending it a
 * model name that does not exist there — which nothing checked, and which
 * stayed invisible only because no agent had been pinned to a hosted service
 * yet.
 *
 * On a hosted service with nothing named, the model is left empty on purpose.
 * The provider then answers with its own default, which is the only model this
 * program can be certain that service has.
 */
func (c *Conductor) choose(step *store.TaskStep, member team.Agent, through string) llm.Choice {
	choice := llm.Choice{Why: member.Title, Tools: true}

	if c.Sizes == nil {
		return choice
	}

	sizes := c.Sizes()
	choice.Model = member.ModelOn(through, sizes)

	if choice.Model == "" && !llm.Elsewhere(through) {
		// Nothing installed answers that role, so fall back to judging the
		// sentence, which is what happened before there was a roster.
		chosen := llm.ChooseModel(step.Instruction, sizes)
		choice.Model = chosen.Model
	}

	return choice
}

// roster is who is available, or the generalist alone when nothing said.
/*
 * shortlist is who is worth considering for a request, in the planner's terms.
 *
 * Asked of the whole organisation and answered in code. Nothing here calls a
 * model: the point is to reduce what a model is shown, and a shortlist that
 * needed a model call to produce would have spent the saving before making it.
 */
func (c *Conductor) shortlist(request string) []team.Agent {
	return team.Who(c.roster(), c.chart(), c.Occupations, c.Toolbox,
		team.Wanted{Doing: request})
}

// chart is the organisation as it stands, or none — which is what a brain
// that has never opened the organisation has, and works exactly as before.
func (c *Conductor) chart() []org.Unit {
	if c.Chart == nil {
		return nil
	}

	return c.Chart()
}

func (c *Conductor) roster() []team.Agent {
	if c.Roster == nil {
		return team.BuiltIn()
	}

	if found := c.Roster(); len(found) > 0 {
		return found
	}

	return team.BuiltIn()
}

/*
 * brief is what a step is told, and it is not the conversation.
 *
 * A step handed the whole thread is coupled to it and to whoever was speaking,
 * and the prompt grows with every step until reading it is the whole of the
 * wait. It is also the thing that would have to be either duplicated or leaked
 * to give a second agent its own context — so the brief is the shape that
 * makes a team of them an addition rather than a rewrite.
 */
func (c *Conductor) brief(task *store.Task, step *store.TaskStep, member team.Agent) []llm.Message {
	var out []llm.Message

	if c.Prompt != nil {
		if persona := c.Prompt(task.Provider); persona != "" {
			out = append(out, llm.Message{Role: llm.RoleSystem, Content: persona})
		}
	}

	var b strings.Builder

	b.WriteString("You are working through a job for its owner, one step at a time.\n\n")

	/*
	 * Which part of the work this is, and what that person is for.
	 *
	 * Said even when there is one agent, because it is what makes the tool
	 * list make sense: a model told it is the researcher and given five tools
	 * behaves like one, where a model given five tools and no explanation
	 * spends its first round asking for a sixth.
	 */
	if member.Title != "" {
		b.WriteString("You are the " + strings.ToLower(member.Title) + " on this job — " +
			member.For + ".\n")
	}

	if member.Brief != "" {
		b.WriteString(member.Brief + "\n")
	}

	b.WriteString("\n")
	b.WriteString("The job: " + task.Goal + "\n")

	if task.DoneWhen != "" {
		b.WriteString("It is finished when: " + task.DoneWhen + "\n")
	}

	if established := c.established(task.ID); established != "" {
		b.WriteString("\nWhat has been established so far:\n" + established + "\n")
	}

	b.WriteString("\nDo only this step, and nothing beyond it:\n" + step.Instruction + "\n")

	/*
	 * And what went wrong last time, when there was a last time.
	 *
	 * Retrying without saying why is asking for the same answer. The reason
	 * comes off the row rather than being carried in memory, so it survives
	 * the program being closed between the two attempts.
	 */
	if step.Attempts > 1 && step.Why != "" {
		b.WriteString("\nA previous attempt at this step was rejected: " + step.Why +
			"\nDo it differently.\n")
	}

	if step.DoneWhen != "" {
		b.WriteString("\nThis step is done when: " + step.DoneWhen + "\n")
	}

	b.WriteString("\nUse a tool rather than guessing. When you have what the step " +
		"asked for, say what you found in a sentence or two and stop.")

	out = append(out, llm.Message{Role: llm.RoleUser, Content: b.String()})

	return out
}

/*
 * established is what earlier steps actually found, and only that.
 *
 * Verified evidence, never a claim. Carrying "I have written the file" forward
 * as though it were established is how a plan builds three more steps on
 * something that never happened. A step that was refused is carried too, in
 * its own words, so the model works around it rather than proposing it again.
 */
func (c *Conductor) established(taskID int64) string {
	steps, err := c.DB.Steps(taskID)
	if err != nil {
		return ""
	}

	var lines []string

	for _, s := range steps {
		switch s.State {
		case store.StepDone:
			if s.Verdict == store.Verified && s.Answer != "" {
				lines = append(lines, "- "+trimTo(s.Answer, 300))
			}
		case store.StepSkipped:
			if s.Why != "" {
				lines = append(lines, "- "+s.Why)
			}
		}
	}

	return strings.Join(lines, "\n")
}

/*
 * finish closes a task and writes the account it gives of itself.
 *
 * Verified and claimed are counted separately and said separately. On a
 * machine with one small model a great deal may be claimed and very little
 * verified, and its owner should be able to see that for themselves rather
 * than be told everything went well.
 */
func (c *Conductor) finish(task *store.Task, state, because string) error {
	steps, err := c.DB.Steps(task.ID)
	if err != nil {
		return err
	}

	report := c.report(task, steps, state, because)

	/*
	 * Said before the row says finished, not after.
	 *
	 * The other way round is a race with anything watching: the state flips to
	 * done, whoever is polling sees a finished task, and the sentence saying
	 * what it did is not in the conversation yet. It showed up first as a test
	 * that passed alone and failed in company, which is the same bug wearing a
	 * different hat.
	 */
	c.say(task, report)

	if err := c.DB.FinishTask(task.ID, state, report, because); err != nil {
		return err
	}

	// A specialist's task finishing is its parent's step finishing.
	return c.childFinished(task, state)
}

/*
 * report is the account a task gives of itself.
 *
 * Assembled as sections and joined, rather than written straight into a
 * builder. The builder version produced "Stopped on X.\n\n\n2 steps not done."
 * whenever a task got nowhere and had no bullets to put between the two — and
 * it did that twice, because the second time it was written the same way.
 */
func (c *Conductor) report(task *store.Task, steps []store.TaskStep, state, because string) string {
	var sections []string

	switch state {
	case store.TaskDone:
		sections = append(sections, "Finished: "+task.Name+".")
	case store.TaskBlocked:
		sections = append(sections, "Stopped on "+task.Name+" — "+because+".")
	default:
		sections = append(sections, task.Name+" ended: "+because+".")
	}

	var (
		bullets                       []string
		verified, claimed, unfinished int
	)

	wrong := 0

	for _, s := range steps {
		switch {
		case s.State == store.StepDone && s.Verdict == store.Verified:
			verified++
		case s.State == store.StepDone && s.Verdict == store.Unmet:
			wrong++
		case s.State == store.StepDone:
			claimed++
		case s.State != store.StepSkipped:
			unfinished++
		}

		if s.State == store.StepDone && s.Answer != "" {
			bullets = append(bullets, "• "+trimTo(s.Answer, 400))
		}
	}

	if len(bullets) > 0 {
		sections = append(sections, strings.Join(bullets, "\n"))
	}

	/*
	 * What the answer rests on, counted rather than asserted.
	 *
	 * Verified and claimed are said separately because on a machine with one
	 * small model a great deal may be claimed and very little verified, and
	 * its owner should be able to see that rather than be told it went well.
	 */
	var tail []string

	switch {
	case claimed == 0 && verified > 0:
		tail = append(tail, countOf(verified, "step")+" checked against what the tools returned.")
	case verified == 0 && claimed > 0:
		tail = append(tail, countOf(claimed, "step")+
			" taken on its own word — nothing was looked up to confirm it.")
	case claimed > 0:
		tail = append(tail, fmt.Sprintf("%s checked against what the tools returned, %d taken on its own word.",
			countOf(verified, "step"), claimed))
	}

	if wrong > 0 {
		tail = append(tail, countOf(wrong, "step")+" found wrong when it was reviewed.")
	}

	if unfinished > 0 {
		tail = append(tail, countOf(unfinished, "step")+" not done.")
	}

	if len(tail) > 0 {
		sections = append(sections, strings.Join(tail, " "))
	}

	/*
	 * And what it did that would have been asked about, had anything asked.
	 *
	 * Last, and always when there is any. Never stop is its owner's choice,
	 * and the account of a task run under it is where that choice is
	 * reviewed: which payments, which deletions, which messages went out
	 * without a question. Leaving them to be found in the step detail would
	 * be the setting working and nobody being able to tell what it did.
	 */
	var acted []string

	for _, s := range steps {
		for _, line := range strings.Split(s.Acted, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				acted = append(acted, "• "+line)
			}
		}
	}

	if len(acted) > 0 {
		sections = append(sections, "Done without asking, because it is set never to stop:\n"+
			strings.Join(acted, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func countOf(n int, thing string) string {
	if n == 1 {
		return "One " + thing
	}

	return fmt.Sprintf("%d %ss", n, thing)
}

// Stop ends a task at the next thing it does, and says so on the row so that
// a task parked on a decision is stopped too.
func (c *Conductor) Stop(taskID int64) error {
	task, err := c.DB.Task(taskID)
	if err != nil || task == nil {
		return err
	}

	if task.JobID != 0 && c.Jobs != nil {
		c.Jobs.Stop(task.JobID)
	}

	// And whatever it handed on. Stopping a job and leaving its specialists
	// working on pieces of it would be stopping nothing.
	if children, err := c.DB.Children(taskID); err == nil {
		for _, child := range children {
			if live(child.State) {
				c.Stop(child.ID)
			}
		}
	}

	return c.DB.SetTaskState(taskID, store.TaskStopped, "you stopped it")
}

// live is whether a task could still do anything.
func live(state string) bool {
	switch state {
	case store.TaskDone, store.TaskBlocked, store.TaskStopped:
		return false
	}

	return true
}

/*
 * lookingOnly narrows a turn to the tools that only look.
 *
 * From the registry rather than a list kept here, so a tool added later is
 * sorted by the risk it declares — the same declaration the gate trusts.
 */
func (c *Conductor) lookingOnly(only []string, withTools bool) ([]string, bool) {
	if !withTools || c.Agent == nil || c.Agent.Registry == nil {
		return nil, false
	}

	allowed := map[string]bool{}

	for _, name := range only {
		allowed[name] = true
	}

	out := []string{}

	for _, t := range c.Agent.Registry.All() {
		if t.Risk() != tools.Safe || (len(only) > 0 && !allowed[t.Name()]) {
			continue
		}

		out = append(out, t.Name())
	}

	return out, len(out) > 0
}

// Live is everything still going on.
func (c *Conductor) Live() ([]store.Task, error) { return c.DB.LiveTasks() }

// plainly turns an error into something worth reading when it is the only
// explanation somebody gets for work that stopped.
func plainly(err error) string {
	if err == nil {
		return ""
	}

	text := err.Error()

	switch {
	case strings.Contains(text, "connection refused"):
		return "the model it was using is not running"
	case strings.Contains(text, "context deadline exceeded"), strings.Contains(text, "timeout"):
		return "the model took longer to answer than it was given"
	}

	return text
}

// trimTo cuts by runes rather than bytes. Cyrillic is two bytes a letter, and
// cutting by bytes leaves half of one.
func trimTo(text string, most int) string {
	text = strings.TrimSpace(text)

	if utf8.RuneCountInString(text) <= most {
		return text
	}

	return string([]rune(text)[:most]) + "…"
}

/*
 * Resolved carries on with whatever was waiting on a decision.
 *
 * A conversation that hits the approval gate simply ends, and its owner asks
 * again. A task cannot do that: it has a quarter of an hour of work behind it
 * and nobody to ask. So the decision is linked to the step when the task
 * parks, and this is the way back.
 *
 * The result comes with it. Deciding used to run the approved tool and write
 * the output onto a row that no model ever read — so the thing somebody
 * approved happened, and the assistant never found out. For a conversation
 * that was merely disappointing; for a task it is the difference between
 * carrying on and repeating the step that was just done.
 */
func (c *Conductor) Resolved(ctx context.Context, invocation store.Invocation) error {
	step, err := c.DB.StepWaitingOn(invocation.ID)
	if err != nil || step == nil {
		// An ordinary approval from a conversation, with no task behind it.
		return err
	}

	task, err := c.DB.Task(step.TaskID)
	if err != nil || task == nil {
		return err
	}

	// Another decision in the same step is still outstanding. A step that
	// asked for two things must not carry on when one of them is answered.
	waiting, err := c.DB.TaskPendingCount(task.ID)
	if err != nil {
		return err
	}

	if waiting > 0 {
		return nil
	}

	switch task.State {
	case store.TaskStopped, store.TaskDone, store.TaskBlocked:
		// Decided after somebody stopped it. The action itself still happened
		// — that is the gate doing its job — but there is no work to resume.
		return nil
	}

	switch invocation.Status {
	case store.InvocationDenied:
		/*
		 * Refused, so the step is skipped and the refusal is carried forward.
		 *
		 * In its owner's words, into the brief every later step gets, so the
		 * model works around it rather than proposing the same thing again
		 * three steps later and being refused again.
		 */
		step.State = store.StepSkipped
		step.Verdict = store.Unmet
		step.Why = "you said no to: " + invocation.Summary

	case store.InvocationFailed:
		step.State = store.StepFailed
		step.Verdict = store.Unmet
		step.Why = "the action you approved did not work: " + trimTo(invocation.Result, 200)

	default:
		step.State = store.StepDone
		step.Verdict = store.Verified
		step.Evidence = strings.TrimSpace(invocation.Summary + "\n" + invocation.Result)
		step.Answer = firstSentence(invocation.Result, invocation.Summary)
		step.Why = ""
	}

	if err := c.DB.FinishStep(*step); err != nil {
		return err
	}

	if err := c.DB.SetTaskState(task.ID, store.TaskWorking, ""); err != nil {
		return err
	}

	/*
	 * And it picks up where it left off, on the Runner rather than here.
	 *
	 * This is called from the request that approved the action, and the rest
	 * of a task is minutes of work — nobody clicking Approve is asking to wait
	 * for it. The budget and the deadline are read from the row, so a task
	 * cannot buy itself more of either by having been parked.
	 */
	job, err := c.Jobs.StartQueued(task.Name, func(ctx context.Context) (string, error) {
		return "", c.Work(ctx, task.ID)
	})
	if err != nil {
		return c.DB.SetTaskState(task.ID, store.TaskWaiting, plainly(err))
	}

	return c.DB.SetTaskJob(task.ID, job.ID)
}

/*
 * Resume restarts a task that was parked, from its own view.
 *
 * For the one left waiting when the program was closed, and for one that could
 * not start because the machine was already busy. It refuses a task that is
 * still waiting on a decision, because the decision is the thing in the way.
 */
func (c *Conductor) Resume(taskID int64) error {
	task, err := c.DB.Task(taskID)
	if err != nil {
		return err
	}

	if task == nil {
		return fmt.Errorf("there is no task with that number")
	}

	if task.State != store.TaskWaiting && task.State != store.TaskStopped {
		return fmt.Errorf("that task is not waiting to be picked up")
	}

	outstanding, err := c.DB.TaskPendingCount(taskID)
	if err != nil {
		return err
	}

	if outstanding > 0 {
		return fmt.Errorf("it is waiting for your decision on something first")
	}

	// Picked up while a specialist still has one of its steps would carry on
	// past that step as if it were done.
	if children, err := c.DB.Children(taskID); err == nil {
		for _, child := range children {
			if live(child.State) && child.State != store.TaskStopped {
				return fmt.Errorf("it is waiting on work it handed on: %s", child.Name)
			}
		}
	}

	if err := c.DB.SetTaskState(taskID, store.TaskWorking, ""); err != nil {
		return err
	}

	job, err := c.Jobs.StartQueued(task.Name, func(ctx context.Context) (string, error) {
		return "", c.Work(ctx, taskID)
	})
	if err != nil {
		c.DB.SetTaskState(taskID, store.TaskWaiting, plainly(err))

		return err
	}

	return c.DB.SetTaskJob(taskID, job.ID)
}
