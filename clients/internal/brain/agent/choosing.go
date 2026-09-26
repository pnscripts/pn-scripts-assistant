package agent

import (
	"sort"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * Which tools a turn is given, and why not all of them.
 *
 * Everything the assistant has is available; this is about what fits in one
 * question. Two costs decide it, and both were measured on this machine
 * rather than guessed at:
 *
 *   - Money's worth of time. A turn with thirty-nine tools offered is 6,338
 *     tokens, of which about 5,100 is the tool descriptions, and this
 *     processor reads eight tokens a second. Offering all seventy-three would
 *     be roughly 9,500 tokens: twenty minutes before the first word of an
 *     answer.
 *   - How well a model chooses. It falls off sharply with size. Asked what it
 *     thought of a website, the small model here called list_models; asked to
 *     say hello in four words it proposed creating a file. Thirty-nine
 *     plausible options is already a harder problem than it can reliably
 *     solve.
 *
 * So the offered set is chosen, not truncated: what every turn needs, plus
 * what this one plausibly needs, up to a budget — and once something has been
 * offered in a conversation it stays offered.
 *
 * That last rule is not tidiness. Ollama keeps what it has already read only
 * while the prompt begins the same way, and the tools are part of that
 * beginning. A set that changed with every message would throw the reading
 * away every time: measured here, 598 seconds against 58 for a turn whose
 * prefix was reused. Growing only, never shrinking, costs one reading when
 * something new is wanted and nothing at all afterwards.
 */

/*
 * HowMuchSchema bounds the tool descriptions in one turn, in characters.
 *
 * About 2,300 tokens, which on this processor is four minutes of reading the
 * first time and nothing on the turns after it. Roughly a third of what
 * offering everything would cost, and enough for some forty tools.
 */
const HowMuchSchema = 9000

/*
 * always are the tools every turn is given, whatever it is about.
 *
 * Only tools that look at something. Nothing here changes anything, and that
 * is not caution about the gate — the gate would stop them anyway — it is
 * about what a small model does when it can see them.
 *
 * "Say hello in four words" made the model here propose creating
 * /home/Petar/greetings.txt, and the turn ended in an approval prompt instead
 * of an answer: not a wrong answer but no answer, arrived at by way of a
 * confirmation nobody asked for. Two days ago, asked to say hello in five
 * words, it reached for write_document and stopped for approval again.
 *
 * A model with thirty tools and a greeting in front of it will find something
 * to do with them. The prompt asks it not to; the list it is shown is the
 * part that can be relied on. So writing, editing and running are offered
 * when a message asks for them — see the cues in relevance.go — and reading
 * is always there, because reading cannot go wrong.
 */
var always = map[string]bool{
	"ask_first":       true,
	"what_can_you_do": true,
	"what_you_know":   true,
	"read_file":       true,
	"list_directory":  true,
	"search_files":    true,
}

/*
 * Choosing is what one turn's list is built from.
 *
 * Every field may be nil, and a Choosing with all of them nil offers
 * everything that fits the budget — which is the honest behaviour for a
 * caller that knows nothing about the machine.
 */
type Choosing struct {
	// Have reports whether something a tool needs is on this machine.
	Have func(id string) bool

	// Needs is what a tool cannot work without.
	Needs func(tool string) []string

	// Cued is a tool that asks to be offered only when it is mentioned.
	Cued func(tool string) []string
}

/*
 * remembered is what each conversation has already been shown.
 *
 * Small, and never cleaned: a conversation's set is a few dozen short names,
 * and a brain that has held a thousand conversations in one run has bigger
 * things to account for than a map of them.
 */
type remembered struct {
	mu   sync.Mutex
	sets map[int64]map[string]bool
}

func (o *remembered) keep(conversation int64, names []string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.sets == nil {
		o.sets = map[int64]map[string]bool{}
	}

	set, seen := o.sets[conversation]
	if !seen {
		set = map[string]bool{}
		o.sets[conversation] = set
	}

	for _, name := range names {
		set[name] = true
	}
}

func (o *remembered) already(conversation int64) map[string]bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.sets[conversation]
}

/*
 * choose is the list for this turn: what is always offered, what was offered
 * earlier in this conversation, and what this message plausibly needs — in
 * that order of claim on the budget.
 */
func (c Choosing) choose(specs []llm.ToolSpec, message string, already map[string]bool) []llm.ToolSpec {
	text := strings.ToLower(message)

	// A website in the question means the answer is out there, so the tools
	// that reach it are wanted whatever else is dropped.
	aboutSomewhere := mentionsSomewhere.MatchString(message)

	type ranked struct {
		spec  llm.ToolSpec
		claim int
	}

	var keep []ranked

	for _, spec := range specs {
		/*
		 * Something whose requirement is not on this machine is not offered.
		 *
		 * Not a refusal: the inventory in the prompt says Godot is not
		 * installed and what it would take, so the assistant can answer about
		 * it — which it could never do while the only signal was a tool that
		 * failed when called. What is saved is the budget, and a model's
		 * attention, on a tool that could only have failed.
		 */
		if !c.here(spec.Name) {
			continue
		}

		switch {
		case always[spec.Name]:
			keep = append(keep, ranked{spec, 3})

		case already[spec.Name]:
			// Offered earlier in this conversation. Kept whether or not this
			// message mentions it, because taking it away now would throw
			// away everything the model has already read.
			keep = append(keep, ranked{spec, 2})

		case c.cuedFor(spec.Name) != nil:
			// A tool that asked to be offered only when it is mentioned —
			// how a skill gets into the list without crowding it.
			if mentions(text, c.cuedFor(spec.Name)) {
				keep = append(keep, ranked{spec, 2})
			}

		case housekeeping[spec.Name] != nil:
			/*
			 * A housekeeping tool has to be asked for. Except when the
			 * question names a website, in which case looking at the screen
			 * is not merely unhelpful but actively wrong — it is what the
			 * model reached for last time, and it spent seven minutes on it.
			 */
			if !aboutSomewhere && mentions(text, housekeeping[spec.Name]) {
				keep = append(keep, ranked{spec, 2})
			}

		case mentions(text, []string{strings.ReplaceAll(spec.Name, "_", " "), spec.Name}):
			// Named outright, which is how a task's step asks for one.
			keep = append(keep, ranked{spec, 2})

		default:
			keep = append(keep, ranked{spec, 1})
		}
	}

	// The budget, spent on the strongest claims first and, within a claim, in
	// the order the registry gave them — which is alphabetical, so the same
	// question twice gives the same list.
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].claim > keep[j].claim })

	out := make([]llm.ToolSpec, 0, len(keep))
	spent := 0

	for _, each := range keep {
		cost := costOf(each.spec)

		// Always-offered tools are not subject to the budget: a turn without
		// read_file is not a cheaper turn, it is a broken one.
		if each.claim < 3 && spent+cost > HowMuchSchema {
			continue
		}

		out = append(out, each.spec)
		spent += cost
	}

	// Back into the registry's own order, so that two turns offering the same
	// tools send the same bytes.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// here reports whether everything a tool needs is on this machine.
func (c Choosing) here(tool string) bool {
	if c.Needs == nil || c.Have == nil {
		return true
	}

	for _, needed := range c.Needs(tool) {
		if !c.Have(needed) {
			return false
		}
	}

	return true
}

func (c Choosing) cuedFor(tool string) []string {
	if c.Cued == nil {
		return nil
	}

	own := c.Cued(tool)
	if len(own) == 0 {
		return nil
	}

	return own
}

/*
 * costOf is what a tool costs to describe, in characters.
 *
 * The name, the description and the schema, which is what is actually sent.
 * Characters rather than tokens because counting tokens means knowing the
 * model's own vocabulary, and the ratio is stable enough that a budget in
 * characters is a budget in tokens with a different number on it.
 */
func costOf(spec llm.ToolSpec) int {
	return len(spec.Name) + len(spec.Description) + len(spec.Parameters)
}
