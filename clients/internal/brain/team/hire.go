package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Hiring: from "I need someone who specialises in Laravel security" to a
 * person on the organisation who does.
 *
 * In code, from the taxonomy, and never by asking a model to invent somebody.
 * The specification's own sequence, in order:
 *
 *   find the job         what the sentence names, matched against the jobs
 *                        and the capabilities the catalogue already has
 *   or find the person   somebody already here who can do all of it is the
 *                        answer, and hiring a copy of them is refused — two
 *                        agents that differ in name make the planner's
 *                        choice harder for nothing
 *   take a template      what that job may touch, when one exists
 *   add what is special  the capabilities the sentence named beyond the job
 *   decide the tools     never more than whoever asked could use
 *   find or make a seat  a vacant one for that job, or a new one beside the
 *                        people who do similar work
 *   save it              a file in the agents folder, like every agent
 *
 * A temporary hire, for one task, gets no seat: it is dissolved when the task
 * ends, and a chart that grew a seat for every one would be a chart nobody
 * could read by the end of the month.
 */

// Catalogue is what hiring needs from the taxonomy.
type Catalogue interface {
	Occupations
	FindOccupations(text string, limit int) ([]store.Occupation, error)
	FindCapabilities(text string, limit int) ([]store.Capability, error)
	OccupationsNeeding(capabilityID string, limit int) ([]store.Occupation, error)
}

// Wish is somebody being asked for.
type Wish struct {
	// Sentence is what was said, or a step's instruction, or a job title a
	// planner wrote.
	Sentence string

	// ForTask makes this a temporary hire for that task. Zero is a permanent
	// one, which only its owner asks for.
	ForTask int64

	// Within is the most the hire may use, when whoever asked was held to
	// less than everything. Nil is no limit.
	Within *[]string
}

// Hired is what came of a wish.
type Hired struct {
	Agent Agent `json:"agent"`

	// Existing is true when somebody already here was the answer.
	Existing bool `json:"existing"`

	Job  string `json:"job"`
	Seat string `json:"seat,omitempty"`

	// Can is what the sentence named that the job did not already cover.
	Can []string `json:"can,omitempty"`

	// Why is the decision in a sentence, for the report and the interface.
	Why string `json:"why"`
}

// asking is the words that say somebody is wanted rather than what for.
var asking = map[string]bool{
	"i": true, "we": true, "need": true, "want": true, "someone": true, "somebody": true,
	"who": true, "that": true, "can": true, "a": true, "an": true, "the": true, "to": true,
	"create": true, "hire": true, "make": true, "add": true, "get": true, "find": true,
	"agent": true, "person": true, "specialist": true, "specialists": true, "expert": true,
	"specialises": true, "specializes": true, "specialised": true, "specialized": true,
	"specialising": true, "specializing": true, "in": true, "on": true, "at": true,
	"of": true, "for": true, "with": true, "and": true, "is": true, "good": true,
	"knows": true, "know": true, "please": true, "me": true, "us": true, "new": true,
	"do": true, "does": true, "doing": true,
	"някой": true, "специалист": true, "агент": true, "експерт": true, "който": true,
	"трябва": true, "ми": true, "ни": true, "създай": true, "наеми": true, "за": true,
	"в": true, "на": true, "с": true, "и": true, "търся": true,
}

// subjectOf is the words of a wish that say what the work is.
func subjectOf(sentence string) []string {
	var words []string

	for _, w := range strings.FieldsFunc(strings.ToLower(sentence), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	}) {
		if !asking[w] && len([]rune(w)) > 1 {
			words = append(words, w)
		}
	}

	return words
}

/*
 * capabilitiesIn is the capabilities a wish names, pairs of words first.
 *
 * "Laravel security" is two capabilities and "machine learning" is one, and
 * the only way to tell is to try the pair before its halves. A word counts
 * when it is a capability's name, id or alias outright, or the start of its
 * name — "security" finds "Security auditing", "postgres" finds PostgreSQL.
 */
func capabilitiesIn(cat Catalogue, words []string) []string {
	found := []string{}
	used := make([]bool, len(words))

	try := func(term string) (string, bool) {
		list, err := cat.FindCapabilities(term, 3)
		if err != nil {
			return "", false
		}

		id := strings.ReplaceAll(term, " ", "_")

		for _, c := range list {
			name := strings.ToLower(c.Name)

			if c.ID == id || name == term || strings.HasPrefix(name, term) {
				return c.ID, true
			}

			for _, alias := range c.Aliases {
				if strings.ToLower(alias) == term {
					return c.ID, true
				}
			}
		}

		return "", false
	}

	add := func(id string) {
		for _, have := range found {
			if have == id {
				return
			}
		}

		found = append(found, id)
	}

	for i := 0; i+1 < len(words); i++ {
		if id, ok := try(words[i] + " " + words[i+1]); ok {
			add(id)
			used[i], used[i+1] = true, true
		}
	}

	for i, w := range words {
		if used[i] || len([]rune(w)) < 3 {
			continue
		}

		if id, ok := try(w); ok {
			add(id)
		}
	}

	return found
}

/*
 * jobFor is the job a wish is best answered by.
 *
 * Scored word by word against what each job is, in order of how much a match
 * says. Three points when the job calls for a capability the word names —
 * "security" in application security, network security, security auditing —
 * because what a job needs is what it is. Two when the word is in its title,
 * one when it is in what else it is called.
 *
 * And a job that runs something rather than doing it loses three, unless the
 * wish asked for one. Asked for somebody in Laravel security, the chief
 * security officer matched as well as any engineer — the word is in the title
 * and the job audits security — and the first version of this hired one; the
 * second hired the security manager, who looks after a building. Somebody
 * asking for a specialist wants a specialist; somebody asking for a head of
 * security says "head".
 *
 * A job that matched nothing is not a weak answer; it is no answer, and
 * nobody is hired on the strength of it.
 */
func jobFor(cat Catalogue, words, capabilities []string) (*store.Occupation, error) {
	candidates := map[string]bool{}

	note := func(jobs []store.Occupation, err error) error {
		for _, job := range jobs {
			candidates[job.ID] = true
		}

		return err
	}

	for _, capability := range capabilities {
		if err := note(cat.OccupationsNeeding(capability, 40)); err != nil {
			return nil, err
		}
	}

	for _, w := range words {
		if err := note(cat.FindOccupations(w, 20)); err != nil {
			return nil, err
		}

		related, err := cat.FindCapabilities(w, 10)
		if err != nil {
			return nil, err
		}

		for _, c := range related {
			if strings.Contains(c.ID, w) || strings.Contains(strings.ToLower(c.Name), w) {
				if err := note(cat.OccupationsNeeding(c.ID, 20)); err != nil {
					return nil, err
				}
			}
		}
	}

	leading := false

	for _, w := range words {
		switch w {
		case "chief", "head", "director", "manager", "lead", "officer", "executive", "vp":
			leading = true
		}
	}

	var best *store.Occupation

	bestScore := 0

	for id := range candidates {
		job, err := cat.Occupation(id)
		if err != nil || job == nil {
			continue
		}

		score := scoreJob(*job, words)

		if !leading && leads(*job) {
			score -= 3
		}

		if score > bestScore || (score == bestScore && score > 0 && best != nil && job.ID < best.ID) {
			one := *job
			best, bestScore = &one, score
		}
	}

	return best, nil
}

// leads is whether a job is running something rather than doing it: filed as
// leadership, or with a title that says so wherever it is filed — the
// catalogue's security manager lives under safety.
func leads(job store.Occupation) bool {
	if strings.HasPrefix(job.ID, "leadership.") {
		return true
	}

	for _, w := range strings.Fields(strings.ToLower(job.Title)) {
		switch w {
		case "chief", "head", "director", "manager", "officer":
			return true
		}
	}

	return false
}

func scoreJob(job store.Occupation, words []string) int {
	title := strings.ToLower(job.Title)
	aliases := strings.ToLower(strings.Join(job.Aliases, " "))

	score := 0

	for _, w := range words {
		for _, need := range job.Needs {
			if strings.Contains(need.ID, w) {
				score += 3

				break
			}
		}

		switch {
		case strings.Contains(title, w):
			score += 2
		case strings.Contains(aliases, w):
			score++
		}
	}

	return score
}

// Hire answers a wish with somebody already here, or somebody new.
func Hire(root string, roster []Agent, chart []org.Unit, cat Catalogue, box Toolbox, templates []Template, wish Wish) (Hired, error) {
	words := subjectOf(wish.Sentence)

	if len(words) == 0 {
		return Hired{}, fmt.Errorf("say what the person should be good at")
	}

	capabilities := capabilitiesIn(cat, words)

	job, err := jobFor(cat, words, capabilities)
	if err != nil {
		return Hired{}, err
	}

	if job == nil {
		return Hired{}, fmt.Errorf("nothing in the catalogue of jobs matches %q, so nobody was hired",
			strings.Join(words, " "))
	}

	// Somebody here already, who knows everything that was named and holds the
	// job — or, when no capability was named, simply holds the job.
	for _, a := range roster {
		if !a.Working() {
			continue
		}

		fit := Settle(a, chart, cat, box)

		holds := fit.Job != nil && fit.Job.ID == job.ID

		if (len(capabilities) > 0 && knowsAll(fit.Can, capabilities)) || (len(capabilities) == 0 && holds) {
			return Hired{
				Agent: a, Existing: true, Job: job.ID, Seat: a.Position,
				Why: fmt.Sprintf("the %s already does this, so nobody new was hired",
					strings.ToLower(a.Title)),
			}, nil
		}
	}

	agent := Agent{Uses: UsesWork}

	if t, ok := TemplateFor(templates, job.ID); ok {
		agent = FromTemplate(t, "")
	}

	agent.Job = job.ID
	agent.State = Active

	// What the sentence named beyond what the job already knows.
	already := map[string]bool{}

	for _, need := range job.Needs {
		already[need.ID] = true
	}

	for _, capability := range capabilities {
		if !already[capability] && !coveredBy(capability, job.Needs) {
			agent.Can = append(agent.Can, capability)
		}
	}

	agent.Title = titleFor(words, capabilities, job)
	agent.Name = unusedName(roster, nameFor(words, job))
	agent.For = strings.TrimSpace(job.Description)

	if agent.For == "" {
		agent.For = strings.ToLower(job.Title)
	}

	if len(capabilities) > 0 {
		agent.For = strings.TrimSuffix(agent.For, ".") + ". Especially: " +
			strings.ReplaceAll(strings.Join(capabilities, ", "), "_", " ") + "."
	}

	/*
	 * Never more than whoever asked could use.
	 *
	 * Worked out to a written list — the template's, or what its capabilities
	 * resolve to — and then cut down, so the file says what this one may touch
	 * rather than depending only on a limit held somewhere else.
	 *
	 * Except when that leaves nothing. A file cannot say "no tools" — an empty
	 * list means all of them there, as it does everywhere agents are written
	 * — so the limit that says none is the one on the work it was hired for,
	 * which is enforced where that work runs. A temporary hire does no other.
	 */
	if wish.Within != nil {
		fit := Settle(agent, chart, cat, box)
		mine := append([]string{}, (*wish.Within)...)

		if fit.Narrowed {
			mine = overlapOf(fit.Tools, *wish.Within)
		}

		agent.Tools = mine
	}

	hired := Hired{Job: job.ID, Can: agent.Can}

	if wish.ForTask != 0 {
		agent.State = Temporary
		agent.HiredFor = wish.ForTask
		hired.Why = fmt.Sprintf("hired as a %s for this job only, from the job %s",
			strings.ToLower(agent.Title), strings.ToLower(job.Title))
	} else {
		seat, err := seatFor(root, chart, roster, agent, job)
		if err != nil {
			return Hired{}, err
		}

		agent.Position = seat
		hired.Seat = seat
		hired.Why = fmt.Sprintf("hired a %s from the job %s", strings.ToLower(agent.Title),
			strings.ToLower(job.Title))
	}

	if err := Save(root, agent); err != nil {
		return Hired{}, err
	}

	Forget()

	hired.Agent = agent

	return hired, nil
}

/*
 * coveredBy is whether a job already knows the thing a capability was matched
 * for, under another name.
 *
 * "Security" is matched to security auditing by its first word; hired as an
 * application security engineer, the person already knows security, and a
 * second capability saying so is noise in every shortlist they are scored in.
 */
func coveredBy(capability string, needs []store.Need) bool {
	parts := strings.Split(capability, "_")

	for _, need := range needs {
		for _, part := range strings.Split(need.ID, "_") {
			for _, mine := range parts {
				if len(mine) > 3 && part == mine {
					return true
				}
			}
		}
	}

	return false
}

/*
 * knowsAll is whether somebody already covers everything a wish named, by the
 * same rule a hire is given capabilities by.
 *
 * The same rule, or it asks twice and hires twice: the first version wanted
 * every named capability outright, while hiring had skipped "security
 * auditing" as covered by an application security engineer's own — so asking
 * for Laravel security a second time found nobody who knew security auditing
 * and hired the same specialist again.
 */
func knowsAll(can, wanted []string) bool {
	have := map[string]bool{}
	needs := make([]store.Need, 0, len(can))

	for _, one := range can {
		have[one] = true
		needs = append(needs, store.Need{ID: one})
	}

	for _, one := range wanted {
		if !have[one] && !coveredBy(one, needs) {
			return false
		}
	}

	return true
}

func overlapOf(a, b []string) []string {
	in := map[string]bool{}

	for _, one := range b {
		in[one] = true
	}

	out := []string{}

	for _, one := range a {
		if in[one] {
			out = append(out, one)
		}
	}

	return out
}

/*
 * titleFor is what a person reads: the job's own title when nothing more was
 * asked for, and "Laravel security specialist" when something was.
 */
func titleFor(words, capabilities []string, job *store.Occupation) string {
	if len(capabilities) == 0 {
		return job.Title
	}

	named := strings.Join(words, " ")

	return strings.ToUpper(named[:1]) + named[1:] + " specialist"
}

/*
 * nameFor is the identifier. Letters a file name and a planner can both use,
 * which rules out anything but a–z — so a wish in Bulgarian takes its name
 * from the job rather than from the words.
 */
func nameFor(words []string, job *store.Occupation) string {
	candidate := strings.Join(words, "_") + "_specialist"

	if !validName.MatchString(candidate) {
		_, tail, found := strings.Cut(job.ID, ".")
		if !found {
			tail = job.ID
		}

		candidate = tail
	}

	if len(candidate) > 36 {
		candidate = strings.TrimRight(candidate[:36], "_")
	}

	return candidate
}

// unusedName adds a number until nobody on the roster has the name.
func unusedName(roster []Agent, name string) string {
	taken := map[string]bool{}

	for _, a := range roster {
		taken[a.Name] = true
	}

	if !taken[name] {
		return name
	}

	for i := 2; ; i++ {
		if next := fmt.Sprintf("%s_%d", name, i); !taken[next] {
			return next
		}
	}
}

/*
 * seatFor is where a permanent hire sits.
 *
 * A vacant seat for the same job first — that is a place the organisation
 * already said it wanted somebody. Otherwise a new seat, in the unit whose
 * seats are the nearest kind of work, reporting to its most senior seat.
 * Otherwise at the top of the chart. And with no chart at all, no seat, which
 * is a perfectly good answer: an agent with no seat works exactly as every
 * agent did before there was an organisation.
 */
func seatFor(root string, chart []org.Unit, roster []Agent, agent Agent, job *store.Occupation) (string, error) {
	filled := map[string]bool{}

	for _, a := range roster {
		if a.Working() && a.Position != "" {
			filled[a.Position] = true
		}
	}

	for _, seat := range org.Seats(chart) {
		if seat.Job == job.ID && !filled[seat.Name] {
			return seat.Name, nil
		}
	}

	category, _, _ := strings.Cut(job.ID, ".")

	for _, unit := range chart {
		near := false

		for _, seat := range unit.Seats {
			if c, _, _ := strings.Cut(seat.Job, "."); c == category {
				near = true

				break
			}
		}

		if !near {
			continue
		}

		seat := org.Position{Name: agent.Name, Title: agent.Title, Job: job.ID, Seniority: org.Senior}

		for _, other := range unit.Seats {
			if other.ReportsTo == "" && other.Name != seat.Name {
				seat.ReportsTo = other.Name

				break
			}
		}

		unit.Seats = append(append([]org.Position{}, unit.Seats...), seat)

		if err := org.Save(root, unit); err != nil {
			return "", err
		}

		return seat.Name, nil
	}

	/*
	 * Nowhere nearer, so at the top: a seat in the unit nothing sits under,
	 * answering to nobody on the chart — which on a personal organisation
	 * means answering to its owner.
	 */
	for _, unit := range org.Shape(chart) {
		if unit.Parent != "" {
			continue
		}

		unit.Seats = append(append([]org.Position{}, unit.Seats...),
			org.Position{Name: agent.Name, Title: agent.Title, Job: job.ID, Seniority: org.Senior})

		if err := org.Save(root, unit); err != nil {
			return "", err
		}

		return agent.Name, nil
	}

	return "", nil
}

/*
 * Dissolve lets go of everybody hired for one task, and says who.
 *
 * Their files go; their work does not — every step they did is still recorded
 * against their name, which is where anybody would look for it. Suspending
 * them instead would leave the roster a little more cluttered after every task
 * that needed a specialist, and a roster that grows on its own is one nobody
 * trusts.
 */
func Dissolve(root string, roster []Agent, task int64) ([]Agent, error) {
	gone := []Agent{}

	for _, a := range roster {
		if a.State != Temporary || a.HiredFor != task || a.BuiltIn {
			continue
		}

		err := os.Remove(filepath.Join(Folder(root), a.Name+".md"))
		if err != nil && !os.IsNotExist(err) {
			return gone, err
		}

		gone = append(gone, a)
	}

	if len(gone) > 0 {
		Forget()
	}

	return gone, nil
}
