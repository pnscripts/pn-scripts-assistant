package tasks

import (
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * Hiring for a job, and letting go when it is done.
 *
 * Its owner said to hire freely and say so afterwards, so the conductor does
 * — in two places, and only in two. When the planner names a role nobody on
 * the organisation holds, somebody is hired into it for this job. And when a
 * step has failed and nobody here is better placed, somebody may be hired who
 * is, before the step goes up to a manager.
 *
 * Three things keep that from being alarming rather than useful. A hire made
 * by a task is temporary, and is let go when the job ends, so the roster does
 * not silt up. It may never use more than whoever it is working for could.
 * And every one is written on the task as it happens and named in its account
 * at the end — including the ones already let go, which is exactly when
 * nobody could otherwise find out they existed.
 */

// MostHires is how many people one job may take on. Past it, steps go to
// whoever is already here, which is what happened before hiring existed.
const MostHires = 3

// hiresOn counts the hires a task has already recorded.
func hiresOn(task *store.Task) int {
	n := 0

	for _, line := range strings.Split(task.Hired, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}

	return n
}

// hireLine is how a hire is written on a task and read in its account.
func hireLine(hired team.Hired) string {
	return fmt.Sprintf("%s — %s", hired.Agent.Title, hired.Why)
}

/*
 * hireForPlan puts somebody real behind every role the planner named that
 * nobody holds.
 *
 * Only a name that is not on the roster, and only as a job title: the words
 * the planner wrote are looked up in the catalogue, and a role the catalogue
 * does not know goes to whoever the step's words suit, as it always did.
 */
func (c *Conductor) hireForPlan(task *store.Task, steps []store.TaskStep) {
	if c.Hire == nil {
		return
	}

	roster := c.roster()
	hires := hiresOn(task)
	decided := map[string]string{}

	for i := range steps {
		named := steps[i].Assignee

		if named == "" {
			continue
		}

		if _, here := team.Find(roster, named); here {
			continue
		}

		if already, ok := decided[named]; ok {
			steps[i].Assignee = already

			continue
		}

		if hires >= MostHires {
			continue
		}

		hired, err := c.Hire(team.Wish{Sentence: strings.ReplaceAll(named, "_", " "), ForTask: task.ID})
		if err != nil {
			c.Log.Info("the planner named a role nobody could be hired for", "role", named, "why", err)

			continue
		}

		decided[named] = hired.Agent.Name
		steps[i].Assignee = hired.Agent.Name

		if hired.Existing {
			continue
		}

		hires++

		if err := c.DB.RecordHire(task.ID, hireLine(hired)); err != nil {
			c.Log.Warn("could not record a hire", "task", task.ID, "error", err)
		}

		task.Hired = strings.TrimSpace(task.Hired + "\n" + hireLine(hired))
		roster = c.roster()

		c.Log.Info("hired for a task", "task", task.ID, "agent", hired.Agent.Name, "job", hired.Job)
	}
}

/*
 * hireSpecialist is somebody hired because nobody here could do a step.
 *
 * Recorded on the task at the top of the chain, and dissolved with it: a
 * specialist hired for a step of a step belongs to the job, not to whichever
 * piece of it happened to be running.
 */
func (c *Conductor) hireSpecialist(task *store.Task, step *store.TaskStep, fit team.Fit, exclude map[string]bool) (team.Agent, bool) {
	if c.Hire == nil {
		return team.Agent{}, false
	}

	root, err := c.DB.Root(task)
	if err != nil || root == nil || hiresOn(root) >= MostHires {
		return team.Agent{}, false
	}

	hired, err := c.Hire(team.Wish{
		Sentence: step.Instruction,
		ForTask:  root.ID,
		Within:   narrowerOf(task.Within, fit),
	})
	if err != nil || exclude[hired.Agent.Name] || hired.Agent.Name == "assistant" {
		return team.Agent{}, false
	}

	if !hired.Existing {
		if err := c.DB.RecordHire(root.ID, hireLine(hired)); err != nil {
			c.Log.Warn("could not record a hire", "task", root.ID, "error", err)
		}
	}

	return hired.Agent, true
}

/*
 * letGo dissolves a finished job's temporary hires, and says who went.
 *
 * Only for the task at the top of a chain. A specialist's task ending is one
 * step of the job ending, and the people hired for the job may still be
 * needed by the steps after it.
 */
func (c *Conductor) letGo(task *store.Task) []string {
	if c.Dissolve == nil || task.ParentTaskID != 0 {
		return nil
	}

	gone, err := c.Dissolve(task.ID)
	if err != nil {
		c.Log.Warn("could not let a task's hires go", "task", task.ID, "error", err)
	}

	names := []string{}

	for _, a := range gone {
		names = append(names, a.Title)
	}

	return names
}
