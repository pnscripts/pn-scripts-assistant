package team

import (
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/org"
)

/*
 * Who should do this — asked of the organisation in code, before any model is
 * asked to choose between people.
 *
 * This is the change that makes an organisation of any size possible, and it
 * is one number that decides it. The planner used to be shown every agent,
 * one line each, in every planning prompt. Six lines is free. Three hundred is
 * five to eight thousand tokens before planning starts, on a machine whose
 * own comments measure it at around ten tokens a second — ten minutes of
 * reading a staff list before a word of the job is considered.
 *
 * And it would choose worse for it. The finding is already written down in
 * this program about tools: thirty-one plausible options is a harder problem
 * than a small model can reliably solve. That is exactly as true about people.
 *
 * So the organisation may be enormous and what reaches a prompt stays small.
 * Code picks the handful worth considering — cheaply, from structured data,
 * with no model involved — and the model chooses between those. The same
 * shape the tool list already uses: a cheap filter, then the expensive
 * decision, on a short list.
 */

// Most is how many candidates a shortlist holds unless somebody says
// otherwise. Enough that the right person is almost always among them, few
// enough that choosing is easy.
const Most = 6

/*
 * Wanted is what a piece of work needs, as far as anybody knows yet.
 *
 * Every field is optional. With nothing but the words, this falls back to
 * exactly what the roster did before there were capabilities — which is what
 * a brain with no organisation gets, and it works.
 */
type Wanted struct {
	// Doing is the instruction, or the whole request when planning.
	Doing string

	// Kind is the shape of the step, when there is one: look, do, write.
	Kind string

	// Needs is capabilities the work is known to call for.
	Needs []string

	// Most caps the shortlist. Zero means Most.
	Most int
}

/*
 * Who is the shortlist for a piece of work, best first.
 *
 * Only people who can actually be given work: suspended and retired agents
 * are kept on the roster because what they did is still recorded against
 * their name, and they are not candidates for anything.
 *
 * The generalist is always last and always present. A shortlist that could
 * come back empty would be a plan with nobody in it, and the honest answer to
 * "nobody here specialises in this" is the person who does anything.
 */
func Who(roster []Agent, chart []org.Unit, known Occupations, box Toolbox, want Wanted) []Agent {
	text := strings.ToLower(want.Doing)

	type scored struct {
		agent Agent
		score int
	}

	found := []scored{}

	var generalist *Agent

	for _, a := range roster {
		if !a.Working() {
			continue
		}

		if a.Name == "assistant" {
			one := a
			generalist = &one

			continue
		}

		fit := Settle(a, chart, known, box)

		score := overlap(text, a.For) + prior(want.Kind, a.Name) +
			capabilityScore(text, want.Needs, fit.Can)

		found = append(found, scored{agent: a, score: score})
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })

	most := want.Most

	if most <= 0 {
		most = Most
	}

	out := make([]Agent, 0, most)

	for _, s := range found {
		if len(out) >= most-1 {
			break
		}

		/*
		 * Nobody who matched nothing at all.
		 *
		 * A candidate scoring zero is not a weak match, it is a name the code
		 * has no reason to put forward — and putting it forward anyway is how
		 * a shortlist of six becomes six arbitrary people. The generalist
		 * below is the answer when nothing matched.
		 */
		if s.score == 0 {
			break
		}

		out = append(out, s.agent)
	}

	if generalist != nil {
		out = append(out, *generalist)
	}

	return out
}

/*
 * capabilityScore is what somebody is good at, weighed against what the work
 * needs.
 *
 * Worth more than the words of a job description, because a capability is a
 * name somebody chose deliberately and a description is prose. Two points for
 * a capability the work was said to need, one for a capability the words
 * happen to mention.
 */
func capabilityScore(text string, needed, can []string) int {
	if len(can) == 0 {
		return 0
	}

	wanted := map[string]bool{}

	for _, one := range needed {
		wanted[one] = true
	}

	score := 0

	for _, one := range can {
		if wanted[one] {
			score += 2

			continue
		}

		if len(one) > 3 && strings.Contains(text, strings.ReplaceAll(one, "_", " ")) {
			score++
		}
	}

	return score
}

/*
 * Knowing is who here is good at something, which is a query rather than a
 * question for a model.
 *
 * "Who knows PostgreSQL?" is answered from the roster, the chart and the
 * taxonomy — structured data all the way down. Asking every agent would cost
 * minutes and produce opinions.
 */
func Knowing(roster []Agent, chart []org.Unit, known Occupations, box Toolbox, capability string) []Agent {
	capability = strings.ToLower(strings.TrimSpace(capability))

	out := []Agent{}

	for _, a := range roster {
		if !a.Working() {
			continue
		}

		for _, one := range Settle(a, chart, known, box).Can {
			if one == capability {
				out = append(out, a)

				break
			}
		}
	}

	return out
}
