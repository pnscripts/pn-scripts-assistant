package team

import (
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Working out who somebody actually is.
 *
 * An agent on its own is a name and a few lines. What it is good at comes from
 * three places that have to be put together in order: the job its seat names,
 * which is shared by everybody who holds that job; whatever the seat asks for
 * on top, which is true of the post rather than the person; and whatever this
 * one in particular has been given.
 *
 * Kept in one function because the order matters and getting it wrong is
 * invisible. Authority narrows at every step and widens at none — that is the
 * property the whole roster rests on, and it has to be true of the answer this
 * produces as much as of the lists it was built from.
 */

/*
 * Occupations is what this package needs from the taxonomy: one lookup.
 *
 * A small interface rather than the database, so a roster can be worked out in
 * a test without one. Named for what it holds rather than "jobs", because a
 * job in this program is already a piece of background work and two meanings
 * of one word in one package is how an hour gets lost.
 */
type Occupations interface {
	Occupation(id string) (*store.Occupation, error)
}

// Toolbox is what this package needs from the tool registry: which tools serve
// a set of capabilities.
type Toolbox interface {
	ForCapabilities(capabilities []string) []string
}

// Fit is one agent with everything about it decided.
type Fit struct {
	Agent Agent

	// Where it sits, when it sits anywhere. An agent with no position is
	// exactly what every agent was before there was an organisation, and
	// still works.
	Unit org.Unit
	Seat org.Position

	// Job is the definition its seat names, or nil when there is none — a
	// seat pointing at a job the taxonomy has not got, or an agent with no
	// seat at all.
	Job *store.Occupation

	// Can is everything it is good at: the job's, the seat's, its own.
	Can []string

	/*
	 * Tools is what it may use, and Narrowed says whether that list means
	 * anything.
	 *
	 * Three states rather than two, because the difference between the last
	 * two is the difference between the generalist and a solicitor:
	 *
	 *   Narrowed false            — no limit. Everything the assistant may do.
	 *   Narrowed true, list full  — exactly those.
	 *   Narrowed true, list empty — none at all. Its work is judgement rather
	 *                               than action, and the turn should be run
	 *                               with no tools offered rather than with all
	 *                               of them.
	 *
	 * That last one is why this is not a bare slice. An empty list means
	 * "everything" everywhere else in this program, and a bookkeeper whose
	 * capabilities happen to map to no tool would have quietly been handed
	 * all sixty-six of them.
	 */
	Tools    []string
	Narrowed bool

	// Never is subtracted whatever else is decided, from the seat and from
	// the agent both.
	Never []string
}

/*
 * Settle works out who an agent is, given the organisation and the taxonomy.
 *
 * Every argument may be absent. No chart, no taxonomy and no registry gives
 * back the agent as it was written, which is what a brain that has never
 * opened the organisation should get.
 */
func Settle(agent Agent, chart []org.Unit, known Occupations, box Toolbox) Fit {
	fit := Fit{Agent: agent}

	if agent.Position != "" {
		if unit, seat, ok := org.Seat(chart, agent.Position); ok {
			fit.Unit, fit.Seat = unit, seat
		}
	}

	// The agent's own job wins over the seat's. Somebody hired to do a thing
	// before there was a seat for it should not be renamed by sitting down.
	wanted := agent.Job

	if wanted == "" {
		wanted = fit.Seat.Job
	}

	if wanted != "" && known != nil {
		if found, err := known.Occupation(wanted); err == nil {
			fit.Job = found
		}
	}

	fit.Can = capabilitiesOf(fit)
	fit.Never = mergeNames(fit.Seat.Never, agent.Never)
	fit.Tools, fit.Narrowed = toolsOf(agent, fit, box)

	return fit
}

/*
 * capabilitiesOf puts the three sources together, in the order somebody would
 * read them: what the job is, what the post adds, what this one brings.
 *
 * Essential capabilities first within the job, because that ordering survives
 * into the brief and into the shortlist, and "what this job is" should come
 * before "what it often also involves".
 */
func capabilitiesOf(fit Fit) []string {
	out := []string{}

	if fit.Job != nil {
		for _, need := range fit.Job.Needs {
			if need.Essential {
				out = append(out, need.ID)
			}
		}

		for _, need := range fit.Job.Needs {
			if !need.Essential {
				out = append(out, need.ID)
			}
		}
	}

	out = mergeNames(out, fit.Seat.Needs)
	out = mergeNames(out, fit.Agent.Can)

	if len(out) == 0 {
		return nil
	}

	return out
}

/*
 * toolsOf decides what it may reach for, most specific first.
 *
 * A list written on the agent beats one written on the seat, which beats
 * whatever its capabilities imply. That order is deliberate: the six lists
 * that shipped were arrived at by watching small models choose badly, and a
 * list derived from a job definition is a different list. Where somebody has
 * said exactly what they mean, that is what is used.
 */
func toolsOf(agent Agent, fit Fit, box Toolbox) ([]string, bool) {
	if len(agent.Tools) > 0 {
		return agent.Tools, true
	}

	if len(fit.Seat.Tools) > 0 {
		return fit.Seat.Tools, true
	}

	// Nothing said, and nothing to derive from: the generalist.
	if len(fit.Can) == 0 || box == nil {
		return nil, false
	}

	return box.ForCapabilities(fit.Can), true
}

/*
 * Only is the tool list to hand a turn, and WithTools whether to offer any.
 *
 * The translation from three states into the two the loop understands, kept
 * here so that every caller gets it the same way round. An agent with no
 * tools at all is run with none offered, rather than with an empty list that
 * the loop would read as "all of them".
 */
func (f Fit) Only() ([]string, bool) {
	if !f.Narrowed {
		return nil, true
	}

	return f.Tools, len(f.Tools) > 0
}

// mergeNames unions two lists, keeping the first spelling and the first
// order. Used for capabilities and for limits both, where a duplicate is
// harmless and an order that jumps around is not.
func mergeNames(have, adding []string) []string {
	seen := map[string]bool{}

	for _, one := range have {
		seen[one] = true
	}

	out := have

	for _, one := range adding {
		if one == "" || seen[one] {
			continue
		}

		seen[one] = true
		out = append(out, one)
	}

	return out
}
