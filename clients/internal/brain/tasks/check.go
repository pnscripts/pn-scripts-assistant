package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Checking its own work, honestly.
 *
 * The obvious version of this is a second model call asking "did that work",
 * and the obvious version is worthless: a model asked whether something
 * succeeded will say yes. Asking it more sternly does not help, and neither
 * does a longer prompt — agreeableness is not a wording problem.
 *
 * So the honesty here is in code rather than in the prompt. Three of the four
 * rules never ask a model at all, and the fourth requires the model to produce
 * a string that must then be found in what the tools actually returned. A
 * checker cannot flatter if agreement has to be quoted.
 */

// Checked is what a check concluded and what it cost.
type Checked struct {
	Verdict   string
	Evidence  string
	Why       string
	CheckedBy string
	Calls     int
}

// Verdict is the shape the checking model is asked for.
type verdict struct {
	Met      bool   `json:"met"`
	Evidence string `json:"evidence"`
	Why      string `json:"why"`
}

const checkerBrief = `Decide whether one step of a job actually did what it was supposed to.

Answer with one JSON object and nothing else:

{"met":true,"evidence":"a short phrase copied word for word from what the tools returned","why":"one sentence"}

Rules:
- evidence must be copied exactly from what the tools returned below. Do not
  paraphrase it, do not summarise it, and do not write anything that is not
  there.
- If what the tools returned does not show the step worked, answer met:false.
- The assistant's own account of what it did is not evidence. It is a claim.

`

/*
 * check decides whether a step did what it said it would.
 *
 * The four rules, in the order they are cheapest:
 *
 * A step meant to find something out or make something happen, that ran no
 * tool at all, did not do it. That costs nothing to decide and is not
 * arguable — it is the direct descendant of the promised() check in the agent
 * loop, which exists because "I'll go and look" followed by nothing is the
 * failure this program has had more than any other.
 *
 * A step whose tool returned an error did not do it either.
 *
 * A step that produced only words is recorded as a claim, never as verified.
 * "I have written the file" is not evidence that a file exists.
 *
 * And where a model is asked, its answer only counts if the evidence it quotes
 * is genuinely in what came back.
 */
func (c *Conductor) check(ctx context.Context, task *store.Task, step *store.TaskStep, res agent.Result) Checked {
	evidence := evidenceFrom(res)

	// This program's own action: the engine's answer is the verdict, and no
	// model is asked what it thinks of it.
	if step.Action != "" {
		return actionVerdict(step, res, evidence)
	}

	// A project's writing, by a coding agent or a model the orchestrator
	// chose: what it wrote is what counts, never what it said it wrote — and
	// whether it works is for the checks that follow. A model that only
	// looked at the folder and said "the files are written" had its look
	// taken as the proof, until this.
	if strings.HasPrefix(res.Provider, "agent:") || c.orchestrated(task, step) {
		return agentVerdict(step, res, evidence)
	}

	for _, s := range res.Steps {
		if s.Failed {
			return Checked{
				Verdict:  store.Unmet,
				Evidence: evidence,
				Why:      "a tool it needed did not work: " + trimTo(s.Result, 160),
			}
		}
	}

	if len(res.Steps) == 0 {
		// Nothing was looked up. For a step that was supposed to look
		// something up or change something, that settles it.
		if step.Kind == store.StepLook || step.Kind == store.StepDo {
			return Checked{
				Verdict: store.Unmet,
				Why:     "it did not use any tool, so nothing was actually looked at",
			}
		}

		// A step that produces text legitimately has nothing but the text.
		return Checked{
			Verdict:  store.Claimed,
			Evidence: strings.TrimSpace(res.Reply),
			Why:      "nothing was looked up, so this is its own account of itself",
		}
	}

	// With nothing stated to check against, having used a tool without failing
	// is as far as this can honestly go.
	if strings.TrimSpace(step.DoneWhen) == "" {
		return Checked{Verdict: store.Verified, Evidence: evidence}
	}

	return c.ask(ctx, task, step, res, evidence)
}

// ask puts the question to a model, and then checks the answer against what is
// actually there.
func (c *Conductor) ask(ctx context.Context, task *store.Task, step *store.TaskStep, res agent.Result, evidence string) Checked {
	model, err := c.Provider(task.Provider)
	if err == nil {
		model = c.asking(model, "The checker", step.Instruction)
	}

	if err != nil {
		return Checked{Verdict: store.Claimed, Evidence: evidence,
			Why: "there was no model available to check this"}
	}

	using := c.checkerModel(step)

	// Something has to be recorded here. A blank in the interface where it
	// should say who checked reads as nobody having checked at all.
	if using == "" {
		using = model.Name()
	}

	var b strings.Builder

	b.WriteString(checkerBrief)
	b.WriteString("The step: " + step.Instruction + "\n")
	b.WriteString("It is done when: " + step.DoneWhen + "\n\n")
	b.WriteString("What the tools returned:\n" + trimTo(evidence, 2500) + "\n\n")
	b.WriteString("What the assistant says it did, which is a claim and not evidence:\n" +
		trimTo(res.Reply, 600) + "\n")

	reply, err := model.Chat(ctx, llm.Request{
		Model: using,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You check work. You answer with JSON and nothing else."},
			{Role: llm.RoleUser, Content: b.String()},
		},
	})
	if err != nil {
		return Checked{Verdict: store.Claimed, Evidence: evidence, CheckedBy: using, Calls: 1,
			Why: "the check could not be run: " + plainly(err)}
	}

	said, ok := readVerdict(reply.Content)
	if !ok {
		return Checked{Verdict: store.Claimed, Evidence: evidence, CheckedBy: using, Calls: 1,
			Why: "the check did not come back in a form that could be read"}
	}

	if !said.Met {
		return Checked{Verdict: store.Unmet, Evidence: evidence, CheckedBy: using, Calls: 1,
			Why: firstSentence(said.Why, "the check found the step had not done what it was for")}
	}

	/*
	 * And the evidence has to actually be there.
	 *
	 * This is the whole trick. A model cannot agree its way past a rule that
	 * requires the agreement to be quoted from a transcript it did not write —
	 * so a checker too small to check produces a phrase that is not in the
	 * output, and is caught by a string search rather than by judgement.
	 *
	 * Worth saying out loud when it happens, too: it tells somebody that the
	 * model doing the checking on their machine is not up to it, which is a
	 * fact about their machine they cannot otherwise learn.
	 */
	if !quoted(said.Evidence, evidence) {
		return Checked{Verdict: store.Unmet, Evidence: evidence, CheckedBy: using, Calls: 1,
			Why: "the check produced evidence that is not in what the tools returned"}
	}

	return Checked{
		Verdict:   store.Verified,
		Evidence:  strings.TrimSpace(said.Evidence),
		CheckedBy: using,
		Calls:     1,
	}
}

/*
 * checkerModel is a different model from the one that did the work, where
 * there is one.
 *
 * On a machine with one model installed there is not, and the step records
 * that it was checked by the same model that did the work. Saying so is better
 * than implying otherwise: it is the truth, and it is the kind of truth
 * somebody needs in order to know how much the word "verified" is worth on
 * their own machine.
 */
func (c *Conductor) checkerModel(step *store.TaskStep) string {
	if c.Sizes == nil {
		return step.Model
	}

	sizes := c.Sizes()

	if sizes.Best != "" {
		return sizes.Best
	}

	return step.Model
}

// evidenceFrom is what the tools actually returned, which is the only thing a
// verdict may rest on.
func evidenceFrom(res agent.Result) string {
	var parts []string

	for _, s := range res.Steps {
		parts = append(parts, s.Summary+"\n"+s.Result)
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func readVerdict(content string) (verdict, bool) {
	for _, candidate := range agent.JSONObjects(content) {
		var v verdict

		if err := json.Unmarshal([]byte(candidate), &v); err != nil {
			continue
		}

		return v, true
	}

	return verdict{}, false
}

/*
 * quoted reports whether the evidence is genuinely in what came back.
 *
 * Whitespace and case are normalised because a model copying a phrase out of a
 * directory listing will not reproduce the spacing, and refusing it over two
 * spaces would make the rule useless rather than strict.
 */
func quoted(evidence, output string) bool {
	needle := flatten(evidence)

	// Too short to mean anything. "ok" appears in almost any output, and a
	// rule that accepts it is not a rule.
	if len([]rune(needle)) < 4 {
		return false
	}

	return strings.Contains(flatten(output), needle)
}

func flatten(text string) string {
	var b strings.Builder

	space := false

	for _, r := range strings.ToLower(text) {
		if unicode.IsSpace(r) {
			space = true

			continue
		}

		if space && b.Len() > 0 {
			b.WriteRune(' ')
		}

		space = false

		b.WriteRune(r)
	}

	return b.String()
}

func firstSentence(text, fallback string) string {
	text = strings.TrimSpace(text)

	if text == "" {
		return fallback
	}

	if i := strings.IndexAny(text, ".!?\n"); i > 0 {
		return text[:i+1]
	}

	return trimTo(text, 200)
}

func actionVerdict(step *store.TaskStep, res agent.Result, evidence string) Checked {
	var last *agent.Step

	for i := range res.Steps {
		if res.Steps[i].Tool == "game_check" || res.Steps[i].Tool == "game_build" {
			last = &res.Steps[i]
		}
	}

	switch {
	case last == nil:
		return Checked{Verdict: store.Unmet, Why: "the engine was not run"}
	case last.Failed:
		return Checked{Verdict: store.Unmet, Evidence: evidence,
			Why: "it still does not pass: " + trimTo(last.Result, 300)}
	}

	return Checked{Verdict: store.Verified, Evidence: evidence, CheckedBy: "this program, with the engine"}
}

func agentVerdict(step *store.TaskStep, res agent.Result, evidence string) Checked {
	var wrote []string

	for _, s := range res.Steps {
		for _, e := range s.Evidence {
			if e.Kind == store.EvidenceFile && e.OK {
				if _, err := os.Stat(e.Subject); err == nil {
					wrote = append(wrote, filepath.Base(e.Subject))
				}
			}
		}
	}

	if len(wrote) == 0 && step.Changes {
		return Checked{Verdict: store.Unmet, Evidence: evidence, Why: "it changed no files in the project"}
	}

	return Checked{Verdict: store.Verified, Evidence: evidence, CheckedBy: "this program, reading the files",
		Why: fmt.Sprintf("%d files are there (%s); whether they work is what the checks that follow decide",
			len(wrote), trimTo(strings.Join(wrote, ", "), 200))}
}
