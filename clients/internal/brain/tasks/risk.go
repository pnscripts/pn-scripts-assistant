package tasks

import (
	"strings"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * How serious a step is, worked out in code from three things.
 *
 * What shape of work it is. Finding something out cannot spend money or send
 * anything, whatever it is about — "find out how to file my tax return" is a
 * look, and calling it critical would teach its owner that critical means
 * nothing. So a look or a check is never more than medium on its words alone.
 *
 * What it says. The words of the instruction, in either language, read by the
 * risk package — pay, deploy, delete, sign.
 *
 * Who is doing it. A job carries its own seriousness in the taxonomy, and a
 * lawyer acting is not a researcher acting: the same "send the letter" is a
 * different thing when the letter is legal advice. A job marked for human
 * oversight makes anything it does critical, and anything it writes high —
 * that is what the mark means.
 *
 * None of this ever lowers a level. A step learns who is doing it after it
 * was planned, and can become more serious then; nothing learns it is less.
 */
func (c *Conductor) weigh(step store.TaskStep, member team.Agent) risk.Level {
	level := risk.Low

	if step.Changes {
		level = risk.Medium
	}

	said, _ := risk.InWords(step.Instruction)

	acting := step.Kind == store.StepDo || step.Kind == store.StepWrite || step.Kind == ""

	if !acting && said.AtLeast(risk.High) {
		said = risk.Medium
	}

	level = risk.Max(level, said)

	fit := team.Settle(member, c.chart(), c.Occupations, nil)

	if fit.Job == nil {
		return level
	}

	job := risk.Parse(fit.Job.Risk)

	switch step.Kind {
	case store.StepWrite:
		level = risk.Max(level, job.Below())

		if fit.Job.Oversight {
			level = risk.Max(level, risk.High)
		}
	case store.StepLook, store.StepCheck:
		// Looking is looking, whoever does it.
	default:
		level = risk.Max(level, job)

		if fit.Job.Oversight {
			level = risk.Max(level, risk.Critical)
		}
	}

	return level
}

/*
 * weighPlan sets every step's level before the plan is written down.
 *
 * Before, so the task view shows how serious the work is while it can still
 * be stopped — which is the only moment that knowing it is worth anything.
 */
func (c *Conductor) weighPlan(steps []store.TaskStep) risk.Level {
	most := risk.Low
	roster := c.roster()

	for i := range steps {
		member := team.For(roster, steps[i].Assignee, steps[i].Kind, steps[i].Instruction)
		level := c.weigh(steps[i], member)

		steps[i].Risk = string(level)
		most = risk.Max(most, level)
	}

	return most
}

/*
 * actedUnasked is what ran at high or critical in a turn without anybody being
 * asked.
 *
 * Which can only have happened on never stop, never refuse — at any other
 * setting a critical action asks, and a high one either asked or ran on a
 * grant somebody gave. Listing the high ones too is deliberate: "you told it
 * never to stop" is a choice worth being able to review afterwards, and a
 * review that only shows the critical actions hides most of what it chose.
 */
func actedUnasked(res agent.Result) []string {
	var out []string

	for _, s := range res.Steps {
		if s.Failed || !s.Level.AtLeast(risk.High) {
			continue
		}

		out = append(out, s.Level.Title()+" — "+s.Summary)
	}

	return out
}

// withActed adds this turn's unasked actions to what the step already did on
// an earlier attempt, so a retry does not erase the record of the first.
func withActed(before string, now []string) string {
	if len(now) == 0 {
		return before
	}

	if strings.TrimSpace(before) == "" {
		return strings.Join(now, "\n")
	}

	return before + "\n" + strings.Join(now, "\n")
}
