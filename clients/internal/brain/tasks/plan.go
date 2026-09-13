package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
)

// Plan is what a task is going to do, before any of it is done.
type Plan struct {
	Name     string
	DoneWhen string
	Steps    []store.TaskStep
}

/*
 * The planner is a call of its own, not a tool the model may call.
 *
 * Four reasons, and the first two are the ones that decided it.
 *
 * A tool is optional. The failure this program has fought hardest is "it said
 * it would and it did not" — keepThePromise, promised, the whole of the
 * tool-call recovery. Making the plan something the code asks for, rather than
 * something the model might remember to ask for, is that same fix one level up.
 *
 * And every tool added is paid for by every turn. Tool choice falls off
 * sharply with model size, which is why the loop already narrows thirty-one
 * options down before offering them; a make_a_plan sitting in that list would
 * be picked for "read that file" and missed for "go through my projects".
 *
 * Beyond those: the planner needs no tools at all, which saves the schemas on
 * this call — and a planner holding a registry is not a planner, it is an
 * agent. It can also use a different model without breaking the one-model-per-
 * turn rule, because a whole call of its own is not a step in somebody's turn.
 * That is the right place to spend the best model: the plan is one short call
 * and the steps are many long ones.
 */
const plannerBrief = `Break the job below into steps that can each be done in one go,
and say who on the team should do each one.

Answer with one JSON object and nothing else:

{"name":"a short name for this job",
 "done_when":"one sentence: how anyone would know the whole job is finished",
 "steps":[{"do":"the instruction for this step","done_when":"one plain sentence naming something observable that shows this step worked","kind":"look","who":"researcher"}]}

kind is one of: look (find something out), do (make something happen),
write (produce text or a file).

Rules:
- Each step must be one action, not several.
- done_when must name something a person could check, not "the step is done".
- Do not plan steps that only restate the job or announce what you will do.
- If the job is really one action, answer with a single step. That is fine.
- Between two and eight steps for anything that needs more than one.
- who must be one of the names below. Pick on what the step needs doing to it,
  not on what the job is about as a whole.
- Add "with_previous":true to a step only when it needs nothing at all from the
  step before it, so the two can be done at the same time by different people.
  Leave it out whenever one step uses what another found.

The team:
%s
The job: `

/*
 * plan breaks a request into steps.
 *
 * The plan is written by a model and trusted by nobody: it is parsed strictly
 * and not retried, capped at what the budget allows, and a kind outside the
 * closed set becomes "do". A plan that will not parse degrades to one step,
 * which for an unforced request means there is no task at all.
 *
 * That last rule is the important one. The cost of deciding wrongly that
 * something is a job must be one short model call, never a machine that
 * ceremonially wraps "what time is it" in a plan — so unless its owner asked
 * for a task outright, a one-step plan means the request is answered the way
 * it always was.
 */
func (c *Conductor) plan(ctx context.Context, request, provider string, forced bool) (Plan, bool, error) {
	request = strings.TrimSpace(request)

	if request == "" {
		return Plan{}, false, nil
	}

	single := Plan{
		Name:  nameFor(request),
		Steps: []store.TaskStep{{Instruction: request, Kind: store.StepDo}},
	}

	model, err := c.Provider(provider)
	if err == nil {
		model = c.asking(model, "The planner", request)
	}

	if err != nil {
		// No model to plan with. Forced still gets a task, which will stop at
		// its first step and say the same thing in the report.
		return single, forced, nil
	}

	/*
	 * A shortlist goes in the prompt, not the organisation.
	 *
	 * One line each, so the planner is choosing between people rather than
	 * reading a configuration file — and a handful of lines rather than
	 * however many people there are, because the whole roster in every
	 * planning prompt is what stops an organisation being allowed to grow.
	 * Code picks who is worth considering; the model picks between them.
	 */
	asked := fmt.Sprintf(plannerBrief, team.Describe(c.shortlist(request))) + request

	reply, err := model.Chat(ctx, llm.Request{
		Model: c.plannerModel(),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You plan work. You answer with JSON and nothing else."},
			{Role: llm.RoleUser, Content: asked},
		},
	})
	if err != nil {
		c.Log.Info("could not plan this job", "error", err)

		return single, forced, nil
	}

	written, ok := readPlan(reply.Content, c.budget().MostSteps)
	if !ok {
		/*
		 * Not retried, deliberately.
		 *
		 * A model that answers a request for JSON with prose is not going to
		 * be talked round by asking twice, and each attempt is a whole call on
		 * a processor where that is most of a minute. A small model that
		 * cannot write a plan should mean tasks quietly do not happen on that
		 * machine, not that everything takes twice as long first.
		 */
		c.Log.Info("the model did not write a plan", "reply", trimTo(reply.Content, 200))

		return single, forced, nil
	}

	if written.Name == "" {
		written.Name = nameFor(request)
	}

	// One step is not a job unless somebody said it was.
	if len(written.Steps) < 2 && !forced {
		return written, false, nil
	}

	return written, true, nil
}

// plannerModel is the best this machine has. One short call with no tools, and
// the leverage of every step after it.
func (c *Conductor) plannerModel() string {
	if c.Sizes == nil {
		return ""
	}

	sizes := c.Sizes()

	if sizes.Best != "" {
		return sizes.Best
	}

	return sizes.Work
}

// writtenPlan is the shape the model is asked for, kept apart from Plan so
// that what a model may say and what the program stores are two things.
type writtenPlan struct {
	Name     string `json:"name"`
	DoneWhen string `json:"done_when"`
	Steps    []struct {
		Do       string `json:"do"`
		DoneWhen string `json:"done_when"`
		Kind     string `json:"kind"`
		Who      string `json:"who"`
		Changes  bool   `json:"changes"`

		// WithPrevious is the planner saying this step stands alone. See
		// migration 12.
		WithPrevious bool `json:"with_previous"`
	} `json:"steps"`
}

/*
 * readPlan takes the plan out of whatever the model wrote around it.
 *
 * Using the same brace counter tool-call recovery uses, because a small model
 * asked for JSON has all the same habits here: a sentence first, a fence, or
 * its working in think tags with the object after.
 */
func readPlan(content string, most int) (Plan, bool) {
	for _, candidate := range agent.JSONObjects(content) {
		var written writtenPlan

		if err := json.Unmarshal([]byte(candidate), &written); err != nil {
			continue
		}

		plan := Plan{
			Name:     strings.TrimSpace(written.Name),
			DoneWhen: strings.TrimSpace(written.DoneWhen),
		}

		for _, s := range written.Steps {
			instruction := strings.TrimSpace(s.Do)

			if instruction == "" {
				continue
			}

			// Beyond the allowance the steps are dropped, not obeyed. A model
			// that answers with thirty steps has misunderstood the job, and
			// the budget is not a suggestion it gets to argue with.
			if len(plan.Steps) >= most {
				break
			}

			plan.Steps = append(plan.Steps, store.TaskStep{
				Instruction: instruction,
				DoneWhen:    strings.TrimSpace(s.DoneWhen),
				Kind:        kindOf(s.Kind),

				// Taken as written and checked later, against a roster this
				// function does not have. A name nobody answers to costs the
				// step being done by the generalist, which is the right price
				// for a model inventing a job title.
				Assignee: strings.ToLower(strings.TrimSpace(s.Who)),
				Changes:  s.Changes,

				// Never on the first step, which has nothing before it to
				// stand beside.
				Together: s.WithPrevious && len(plan.Steps) > 0,
			})
		}

		if len(plan.Steps) == 0 {
			continue
		}

		return plan, true
	}

	return Plan{}, false
}

// kindOf keeps kind a closed set. It chooses the model and narrows the tools,
// and it is what a named agent will eventually be routed by — none of which
// survives the model inventing a sixth one.
func kindOf(written string) string {
	switch strings.ToLower(strings.TrimSpace(written)) {
	case store.StepLook:
		return store.StepLook
	case store.StepWrite:
		return store.StepWrite
	case store.StepCheck:
		return store.StepCheck
	default:
		return store.StepDo
	}
}

/*
 * nameFor is what the task is called in the list of them.
 *
 * The request itself, shortened, when the model did not name it. A generated
 * name is another call before anything has happened, and it is not enough
 * better than the sentence somebody actually said to be worth the wait.
 */
func nameFor(request string) string {
	name := strings.Join(strings.Fields(request), " ")

	name = trimTo(name, 60)

	if name == "" {
		return "A job"
	}

	return string(unicode.ToUpper([]rune(name)[0])) + string([]rune(name)[1:])
}
