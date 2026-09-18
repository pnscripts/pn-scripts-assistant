package tasks

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

/*
 * Knowledge, memory and record, as a step meets them.
 *
 * Knowledge is the job's. What a security engineer does, knows, produces and
 * is judged on comes from the taxonomy and is the same for everyone who holds
 * the job; it goes into the brief so the model is told what the work consists
 * of rather than only its title, which it already knew.
 *
 * Memory is the agent's own. What it established, checked against what the
 * tools returned, and what went wrong — a review that found its work wrong, a
 * step it could not do. Recalled when a later step shares enough words with
 * it, lessons first.
 *
 * Organisational knowledge is its owner's profile, which the persona already
 * carries — under the same privacy rule as everything else about them.
 *
 * And the record is a tie-break, never a ranking. Two people equally suited to
 * a step are chosen between by how much of their work has been verified, and
 * only once each has done enough for the rate to mean anything.
 */

// remember writes one memory, quietly: a memory that could not be written is
// a worse next time, not a reason for this step to fail.
func (c *Conductor) remember(agent string, kind string, task *store.Task, step *store.TaskStep, content string) {
	if agent == "" || strings.TrimSpace(content) == "" {
		return
	}

	err := c.DB.Remember(store.Memory{
		Agent: agent, Kind: kind, Content: trimTo(content, 400),
		TaskID: task.ID, StepID: step.ID,
	})
	if err != nil {
		c.Log.Warn("could not remember something", "agent", agent, "error", err)
	}
}

// MostRecalled is how many memories a brief carries. Three is enough to be
// reminded; ten is a second job description.
const MostRecalled = 3

// knowledgeFor is what a brief says about the job, from the taxonomy.
func knowledgeFor(fit team.Fit) string {
	job := fit.Job

	if job == nil {
		return ""
	}

	var parts []string

	list := func(label string, items []string, most int) {
		if len(items) == 0 {
			return
		}

		if len(items) > most {
			items = items[:most]
		}

		parts = append(parts, label+": "+strings.Join(items, "; "))
	}

	list("The job involves", job.Does, 4)
	list("It knows about", job.Knows, 5)
	list("It produces", job.Makes, 3)
	list("Good work in it is judged on", job.MeasuredBy, 3)

	if len(parts) == 0 {
		return ""
	}

	return "What a " + strings.ToLower(job.Title) + " is for. " + strings.Join(parts, ". ") + "."
}

// recalled is what a brief says the agent remembers that bears on this step.
func (c *Conductor) recalled(member team.Agent, step *store.TaskStep) string {
	project := ""

	if task, err := c.DB.Task(step.TaskID); err == nil && task != nil {
		project = task.Project
	}

	memories, err := c.DB.RecallIn(member.Name, step.Instruction, project, MostRecalled)
	if err != nil || len(memories) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("What you remember from your own earlier work that may bear on this:\n")

	for _, m := range memories {
		prefix := "- "

		if m.Kind == store.Learned {
			prefix = "- a lesson: "
		}

		b.WriteString(prefix + m.Content + "\n")
	}

	return b.String()
}

/*
 * record is everybody's work over the last three months, read at most once a
 * minute.
 *
 * Asked for every shortlist, which is every plan and every handing-on, and it
 * is a scan of every step. A minute old is as good as a second old for a
 * tie-break, and far cheaper.
 */
func (c *Conductor) record() map[string]store.AgentWork {
	c.records.Lock()
	defer c.records.Unlock()

	if c.records.of != nil && time.Since(c.records.at) < time.Minute {
		return c.records.of
	}

	out := map[string]store.AgentWork{}

	review, err := c.DB.HowItHasBeenGoing(c.now().AddDate(0, -3, 0))
	if err == nil {
		for _, person := range review.People {
			out[person.Name] = person
		}
	}

	c.records.of, c.records.at = out, time.Now()

	return out
}

type records struct {
	sync.Mutex

	of map[string]store.AgentWork
	at time.Time
}

// lessonOf is how a step that went to somebody else is remembered by the one
// who could not do it.
func lessonOf(step *store.TaskStep, what, why string) string {
	return fmt.Sprintf("could not do %q (%s), so it %s", trimTo(step.Instruction, 120), trimTo(why, 120), what)
}
