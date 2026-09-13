package store

import "time"

/*
 * How the work has actually been going.
 *
 * Counted from the rows rather than asserted, and the counts that matter are
 * the uncomfortable ones. A panel that reports how many tasks finished is a
 * panel that always looks good: a task finishes when its plan runs out of
 * steps, whether or not anything in it was ever confirmed.
 *
 * So verified and claimed are counted apart everywhere here. On a machine with
 * one small model a great deal is claimed and very little verified, and its
 * owner should be able to see that for themselves instead of being told
 * everything went well.
 */

// Review is the whole picture, for the room.
type Review struct {
	Since time.Time `json:"since"`

	Tasks  Tally       `json:"tasks"`
	Steps  StepTally   `json:"steps"`
	People []AgentWork `json:"people"`
	Models []ModelWork `json:"models"`
}

type Tally struct {
	Done    int `json:"done"`
	Blocked int `json:"blocked"`
	Stopped int `json:"stopped"`
	Running int `json:"running"`
	Waiting int `json:"waiting"`
}

type StepTally struct {
	Verified int `json:"verified"`
	Claimed  int `json:"claimed"`
	Unmet    int `json:"unmet"`
	Skipped  int `json:"skipped"`

	/*
	 * Failed is a step that never got as far as being judged.
	 *
	 * Counted separately because it is not the same as failing a check: the
	 * model was unreachable, or a tool errored, and nothing formed an opinion
	 * about the work because there was no work. Left out, a week in which
	 * nothing ran at all reported no steps of any kind — which reads as
	 * "nothing to report" and is the exact opposite of what happened.
	 */
	Failed int `json:"failed"`

	// Running is what is still going, so a review taken mid-task does not
	// quietly under-report it.
	Running int `json:"running"`
}

// AgentWork is what one member of the team has actually got done.
type AgentWork struct {
	Name     string `json:"name"`
	Steps    int    `json:"steps"`
	Verified int    `json:"verified"`
	Claimed  int    `json:"claimed"`
	Failed   int    `json:"failed"`

	// Seconds is how long its steps took in total, which is the number that
	// says whether a model is worth what it costs in waiting.
	Seconds float64 `json:"seconds"`
}

// ModelWork is the same question asked of the models rather than the roles.
type ModelWork struct {
	Name     string  `json:"name"`
	Steps    int     `json:"steps"`
	Verified int     `json:"verified"`
	Seconds  float64 `json:"seconds"`
}

// HowItHasBeenGoing counts what has happened since a given time.
func (d *DB) HowItHasBeenGoing(since time.Time) (Review, error) {
	review := Review{Since: since}

	cutoff := since.UTC().Format(time.RFC3339)

	rows, err := d.sql().Query(
		`SELECT state, COUNT(*) FROM tasks WHERE created_at >= ? GROUP BY state`, cutoff)
	if err != nil {
		return review, err
	}

	for rows.Next() {
		var (
			state string
			n     int
		)

		if err := rows.Scan(&state, &n); err != nil {
			rows.Close()

			return review, err
		}

		switch state {
		case TaskDone:
			review.Tasks.Done = n
		case TaskBlocked:
			review.Tasks.Blocked = n
		case TaskStopped:
			review.Tasks.Stopped = n
		case TaskWorking, TaskPlanning:
			review.Tasks.Running += n
		case TaskWaiting:
			review.Tasks.Waiting = n
		}
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return review, err
	}

	/*
	 * Steps joined back to their tasks by date.
	 *
	 * task_steps has no created_at of its own worth filtering on — a step
	 * created with its plan and run an hour later belongs to when the task
	 * was asked for, which is what somebody means by "this week".
	 */
	stepRows, err := d.sql().Query(`
		SELECT COALESCE(s.assignee,''), COALESCE(s.model,''), s.state,
		       COALESCE(s.verdict,''),
		       COALESCE((julianday(s.ended_at) - julianday(s.started_at)) * 86400, 0)
		FROM task_steps s
		JOIN tasks t ON t.id = s.task_id
		WHERE t.created_at >= ?`, cutoff)
	if err != nil {
		return review, err
	}

	defer stepRows.Close()

	people := map[string]*AgentWork{}
	models := map[string]*ModelWork{}

	for stepRows.Next() {
		var (
			who, model, state, verdict string
			seconds                    float64
		)

		if err := stepRows.Scan(&who, &model, &state, &verdict, &seconds); err != nil {
			return review, err
		}

		switch {
		case state == StepSkipped:
			review.Steps.Skipped++
		case verdict == Verified:
			review.Steps.Verified++
		case verdict == Claimed:
			review.Steps.Claimed++
		case verdict == Unmet:
			review.Steps.Unmet++
		case state == StepFailed:
			review.Steps.Failed++
		case state == StepRunning:
			review.Steps.Running++
		}

		if who == "" {
			who = "assistant"
		}

		person, known := people[who]
		if !known {
			person = &AgentWork{Name: who}
			people[who] = person
		}

		person.Steps++
		person.Seconds += seconds

		switch {
		case verdict == Verified:
			person.Verified++
		case verdict == Claimed:
			person.Claimed++
		case state == StepFailed:
			person.Failed++
		}

		if model == "" {
			continue
		}

		using, known := models[model]
		if !known {
			using = &ModelWork{Name: model}
			models[model] = using
		}

		using.Steps++
		using.Seconds += seconds

		if verdict == Verified {
			using.Verified++
		}
	}

	if err := stepRows.Err(); err != nil {
		return review, err
	}

	for _, p := range people {
		review.People = append(review.People, *p)
	}

	for _, m := range models {
		review.Models = append(review.Models, *m)
	}

	sortBySteps(review.People, review.Models)

	return review, nil
}

// sortBySteps puts whoever did the most first, which is the order somebody
// reads a list like this in anyway.
func sortBySteps(people []AgentWork, models []ModelWork) {
	for i := range people {
		for j := i + 1; j < len(people); j++ {
			if people[j].Steps > people[i].Steps {
				people[i], people[j] = people[j], people[i]
			}
		}
	}

	for i := range models {
		for j := i + 1; j < len(models); j++ {
			if models[j].Steps > models[i].Steps {
				models[i], models[j] = models[j], models[i]
			}
		}
	}
}
