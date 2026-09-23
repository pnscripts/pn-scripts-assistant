package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Proposing a hire before making one.
 *
 * Hiring used to be one step: a sentence in, a file written. That is right for
 * the conductor, whose hires are temporary, narrow and reported — its owner
 * chose that — and wrong for somebody saying "hire a game developer" out loud,
 * who should see who would turn up, what they could touch, what the machine
 * would need and what it would cost before anything is written.
 *
 * So hiring is two steps now, and the second never re-decides the first.
 * Propose works everything out and writes nothing. Accept writes exactly what
 * was proposed — the agent, its tools, its seat — and nothing that was not.
 * Hire, for the conductor, is the two back to back, as it always was.
 */

// How long a hire lasts.
const (
	Permanent = "permanent"
	TaskOnly  = "task"
)

// NoTools is the tool list that means none. An empty list means all of them
// wherever agents are written, so "none" has to be said.
const NoTools = "none"

// Inputs is everything a proposal is worked out from. Only the first five
// are needed; the rest make it fuller, and a proposal without them says less
// rather than guessing.
type Inputs struct {
	Root      string
	Roster    []Agent
	Chart     []org.Unit
	Catalogue Catalogue
	Toolbox   Toolbox

	Templates []Template
	Packages  *capability.Set
	Recipes   *provision.Book

	// Integration answers what is known of an approved integration.
	Integration func(id string) IntegrationState

	// Sizes and Provider decide which model the agent would think with.
	Sizes    llm.Sizes
	Provider string
}

// IntegrationState is what the proposal needs to say about one integration.
type IntegrationState struct {
	Title    string `json:"title"`
	Known    bool   `json:"known"`
	Approved bool   `json:"approved"`
	Remote   bool   `json:"remote"`
}

// Proposal is a hire worked out and not yet made.
type Proposal struct {
	ID      int64  `json:"id,omitempty"`
	Request string `json:"request"`

	// Permanence is permanent, task, or empty for "ask". Goal is the one job
	// a task-only hire is for.
	Permanence string `json:"permanence,omitempty"`
	Goal       string `json:"goal,omitempty"`

	// Existing is somebody already here who does this, in which case there is
	// nothing to approve.
	Existing *Agent `json:"existing,omitempty"`
	Why      string `json:"why"`

	Agent Agent   `json:"agent"`
	Job   JobView `json:"job"`

	// Seat is where a permanent hire would sit, and SeatIn the unit a new
	// seat would be added to — empty when it is a vacant seat already there.
	Seat   string `json:"seat,omitempty"`
	SeatIn string `json:"seat_in,omitempty"`

	// SeatUnit is that unit as a person reads its name.
	SeatUnit string `json:"seat_unit,omitempty"`

	// Can is what the request named beyond the job.
	Can []string `json:"can,omitempty"`

	Packages         []PackageView       `json:"packages,omitempty"`
	Responsibilities []string            `json:"responsibilities,omitempty"`
	Limits           []string            `json:"limits,omitempty"`
	Tools            []string            `json:"tools"`
	Never            []string            `json:"never,omitempty"`
	Model            ModelView           `json:"model"`
	Skills           []string            `json:"skills,omitempty"`
	Requires         []Needed            `json:"requires,omitempty"`
	Integrations     []WantedIntegration `json:"integrations,omitempty"`

	// Itself is what it can do on its own; You what needs its owner; and
	// Professional what needs a qualified person, whatever anybody approves.
	Itself       []string `json:"itself,omitempty"`
	You          []string `json:"you,omitempty"`
	Professional []string `json:"professional,omitempty"`

	Risk      string `json:"risk"`
	Oversight bool   `json:"oversight,omitempty"`
}

// JobView is the job as a proposal shows it.
type JobView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// PackageView is one capability package as a proposal shows it.
type PackageView struct {
	ID       string   `json:"id"`
	Version  string   `json:"version"`
	Title    string   `json:"title"`
	Work     string   `json:"work"`
	Needs    []string `json:"needs,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// ModelView is which model the agent would think with, and where.
type ModelView struct {
	Uses     string `json:"uses"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider"`
}

// Needed is one requirement, as it stands on this machine now.
type Needed struct {
	Package  string           `json:"package"`
	Why      string           `json:"why"`
	Optional bool             `json:"optional,omitempty"`
	Status   provision.Status `json:"status"`
}

// WantedIntegration is one integration a package would use.
type WantedIntegration struct {
	Package  string           `json:"package"`
	ID       string           `json:"id"`
	Why      string           `json:"why"`
	Optional bool             `json:"optional,omitempty"`
	State    IntegrationState `json:"state"`
}

// Answer reads "permanent", "just for this task" and their like, in either
// language. Empty when the words say neither.
func Answer(text string) string {
	t := " " + strings.ToLower(text) + " "

	for _, cue := range []string{"task only", "task-only", "only for this", "just for this",
		"for this task", "for this job", "for one task", "for the task", "temporary", "temporarily",
		"one-off", "one off", "just this once", "само за", "временно", "за тази задача"} {
		if strings.Contains(t, cue) {
			return TaskOnly
		}
	}

	for _, cue := range []string{"permanent", "permanently", "for good", "keep them", "keep him",
		"keep her", "stay on", "full time", "full-time", "постоянно", "за постоянно"} {
		if strings.Contains(t, cue) {
			return Permanent
		}
	}

	return ""
}

/*
 * SplitWish is who is wanted and what for.
 *
 * "Hire an electrician to help me plan this installation" is a job and a
 * piece of work, and searching the catalogue with both found planners: every
 * word of the work counted as much as the one naming the trade. So the job
 * is looked for in the part before "to", and the rest is kept as the goal —
 * which is exactly the one task a task-only hire is for.
 *
 * Only "to", and only when something is left on each side that says a job:
 * "someone who specialises in Laravel security" has no work in it, and
 * "someone to write my novel" names no job before the "to", so the whole
 * sentence is searched as it always was.
 */
func SplitWish(sentence string) (role, goal string) {
	lower := strings.ToLower(sentence)

	for _, joint := range []string{" to ", " so that ", " да "} {
		at := strings.Index(lower, joint)
		if at <= 0 {
			continue
		}

		before, after := strings.TrimSpace(sentence[:at]), strings.TrimSpace(sentence[at+len(joint):])

		if len(subjectOf(before)) == 0 || after == "" {
			return sentence, strings.TrimSpace(after)
		}

		return before, strings.TrimRight(after, ".!? ")
	}

	return sentence, ""
}

/*
 * Propose works out a hire and writes nothing.
 *
 * The same resolution Hire always made — the job from the catalogue, somebody
 * already here, a template — and then everything a person needs to decide:
 * which packages the work falls under, what they would need on this machine
 * and in what state it is now, what the hire could do alone and what would
 * need its owner or a professional.
 */
func Propose(in Inputs, wish Wish) (Proposal, error) {
	role, goal := SplitWish(wish.Sentence)
	words := subjectOf(role)

	var (
		job          *store.Occupation
		capabilities []string
		err          error
	)

	if wish.Job != "" {
		// Named outright: the job is the one picked, and its title is the
		// words everything else is worked out from.
		if in.Catalogue == nil {
			return Proposal{}, fmt.Errorf("there is no catalogue of jobs to hire from")
		}

		if job, err = in.Catalogue.Occupation(wish.Job); err != nil || job == nil {
			return Proposal{}, fmt.Errorf("there is no job %q in the catalogue", wish.Job)
		}

		if len(words) == 0 {
			words = subjectOf(job.Title)
		}

		if strings.TrimSpace(wish.Sentence) == "" {
			wish.Sentence = job.Title
		}
	}

	if len(words) == 0 {
		return Proposal{}, fmt.Errorf("say what the person should be good at")
	}

	if job == nil {
		capabilities = capabilitiesIn(in.Catalogue, words)

		job, err = jobFor(in.Catalogue, words, capabilities)
		if err != nil {
			return Proposal{}, err
		}
	}

	if job == nil {
		return Proposal{}, fmt.Errorf("nothing in the catalogue of jobs matches %q, so nobody was hired",
			strings.Join(words, " "))
	}

	p := Proposal{
		Request:    strings.TrimSpace(wish.Sentence),
		Goal:       goal,
		Permanence: Answer(wish.Sentence),
		Job:        JobView{ID: job.ID, Title: job.Title, Description: job.Description},
		Oversight:  job.Oversight,
	}

	if wish.ForTask != 0 {
		p.Permanence = TaskOnly
	}

	/*
	 * A package named outright says which job, when the words alone did not.
	 *
	 * "A Unity developer" scored a C# developer, which is true of the
	 * language and wrong about the work: Unity is a game engine, and its
	 * package says the job is the game developer's. Only among the jobs the
	 * named package serves, and only one the words also point at.
	 */
	chosen, forced := capability.Package{}, false

	if in.Packages != nil && wish.Package != "" {
		chosen, forced = in.Packages.Get(wish.Package)

		if forced && !listed(chosen.Jobs, job.ID) {
			if better := jobAmong(in.Catalogue, words, chosen.Jobs); better != nil {
				job = better
				p.Job = JobView{ID: job.ID, Title: job.Title, Description: job.Description}
				p.Oversight = job.Oversight
			}
		}
	}

	if in.Packages != nil && !forced {
		if pointed := in.Packages.Named(wish.Sentence); len(pointed) > 0 && !listed(pointed[0].Jobs, job.ID) {
			if better := jobAmong(in.Catalogue, words, pointed[0].Jobs); better != nil {
				job = better
				p.Job = JobView{ID: job.ID, Title: job.Title, Description: job.Description}
				p.Oversight = job.Oversight
			}
		}
	}

	named := namedPackages(in.Packages, job.ID, wish.Sentence)

	if forced {
		named = []capability.Package{chosen}
	}

	if existing, why, ok := alreadyHere(in, job, words, capabilities, named); ok && !wish.New {
		p.Existing = &existing
		p.Why = why

		return p, nil
	}

	packages := packagesFor(in.Packages, job.ID, named)

	agent := Agent{Uses: UsesWork}
	fromTemplate := false

	if t, ok := TemplateFor(in.Templates, job.ID); ok {
		agent = FromTemplate(t, "")
		fromTemplate = true
	}

	agent.Job = job.ID
	agent.State = Active

	already := map[string]bool{}

	for _, need := range job.Needs {
		already[need.ID] = true
	}

	for _, c := range capabilities {
		if !already[c] {
			agent.Can = append(agent.Can, c)
		}
	}

	agent.Title = titleFor(words, job)
	agent.For = strings.TrimSpace(job.Description)

	if agent.For == "" {
		agent.For = strings.ToLower(job.Title)
	}

	/*
	 * One package decides who this is; several leave the job to say it.
	 *
	 * A Godot developer is a package's role, written for the work. A game
	 * developer who could work in any of four engines is the job, told that
	 * the engine's own instructions arrive with each project — four briefs in
	 * one would be most of a prompt about engines the work is not using.
	 */
	switch len(packages) {
	case 0:
	case 1:
		role := packages[0].Role

		if len(beyond(uncovered(words, job), packages[0])) == 0 || forced {
			agent.Title = role.Title
		}

		agent.For = role.For
		agent.Brief = strings.TrimSpace(role.Brief)
		agent.Uses = orElseUses(role.Uses, agent.Uses)
		agent.Needs = readNeeds(strings.Join(role.Needs, ","))
	default:
		names := make([]string, 0, len(packages))

		for _, pkg := range packages {
			names = append(names, pkg.Title)
		}

		agent.Brief = strings.TrimSpace(agent.Brief + "\nWork in whichever of these the project " +
			"uses — " + strings.Join(names, "; ") + ". That one's own instructions come with the project.")
	}

	for _, pkg := range packages {
		agent.Packages = append(agent.Packages, pkg.ID)
		agent.Never = mergeNames(agent.Never, pkg.Role.Never)
	}

	if extra := beyond(uncovered(words, job), packages...); !forced && (len(extra) > 0 || len(agent.Can) > 0) {
		especially := append(append([]string{}, extra...), agent.Can...)

		agent.For = strings.TrimSuffix(agent.For, ".") + ". Especially: " +
			strings.ReplaceAll(strings.Join(especially, ", "), "_", " ") + "."
	}

	agent.Tools = toolsFor(in, agent, packages, fromTemplate, wish.Within)

	/*
	 * A job that keeps a person in the loop — an electrician, a surgeon, a
	 * structural engineer — gets nothing that acts on the world by way of
	 * what its capabilities happen to map to: "troubleshooting" is a command
	 * line for a developer and a live circuit for an electrician. Its package,
	 * when there is one, says what it may use; without one, it advises.
	 */
	if job.Oversight && len(packages) == 0 && !fromTemplate {
		agent.Tools = withoutOperating(agent.Tools)
	}
	agent.Name = unusedName(in.Roster, nameFor(words, job))

	if forced {
		agent.Name = unusedName(in.Roster, nameFor(subjectOf(chosen.Role.Title), job))
	}

	p.Agent = agent
	p.Can = agent.Can
	p.Tools = shownTools(agent.Tools)
	p.Never = agent.Never
	p.Responsibilities = orLines(job.Does, job.Description)

	through := agent.Through(in.Provider)
	p.Model = ModelView{Uses: agent.Uses, Name: agent.ModelOn(through, in.Sizes), Provider: providerName(through)}

	if p.Permanence != TaskOnly {
		p.Seat, p.SeatIn = seatPlan(in.Chart, in.Roster, agent, job)

		if unit, ok := org.Find(in.Chart, p.SeatIn); ok {
			p.SeatUnit = unit.Title
		}
	}

	level := risk.Parse(job.Risk)

	for _, pkg := range packages {
		p.Packages = append(p.Packages, PackageView{
			ID: pkg.ID, Version: pkg.Version, Title: pkg.Title, Work: string(pkg.Work),
			Needs: pkg.Needs(), Evidence: pkg.Evidence,
		})

		p.Skills = append(p.Skills, pkg.Skills...)
		level = risk.Max(level, risk.Parse(pkg.Risk))

		for _, limit := range pkg.Limits {
			p.Limits = append(p.Limits, limit)
		}

		for _, r := range pkg.Requires {
			n := Needed{Package: pkg.ID, Why: r.Why, Optional: r.Optional}

			if in.Recipes != nil {
				n.Status = in.Recipes.Check(r.ID, r.Versions)
			} else {
				n.Status = provision.Status{ID: r.ID, Title: r.ID, Wanted: r.Versions}
			}

			p.Requires = append(p.Requires, n)
		}

		for _, i := range pkg.Integrations {
			w := WantedIntegration{Package: pkg.ID, ID: i.ID, Why: i.Why, Optional: i.Optional}

			if in.Integration != nil {
				w.State = in.Integration(i.ID)
			}

			p.Integrations = append(p.Integrations, w)
		}

		if pkg.Escalation != "" {
			p.Professional = append(p.Professional, pkg.Escalation)
		}
	}

	if len(agent.Never) > 0 {
		p.Limits = append(p.Limits, "Use "+strings.Join(agent.Never, ", ")+" — never, whatever else is allowed")
	}

	if job.Oversight {
		p.Limits = append(p.Limits, "Work without a person in the loop — this job keeps one whatever the settings say")
	}

	/*
	 * Never an AI in a professional's place.
	 *
	 * "Hire a doctor" once proposed somebody called Physician, responsible
	 * for "never decide alone", and not a word about who does decide. A job
	 * that keeps a person in the loop, with no package saying who that person
	 * is, is hired as what it can honestly be — somebody who informs and
	 * prepares — and the professional it cannot replace is named.
	 */
	if job.Oversight && len(p.Professional) == 0 {
		title := strings.ToLower(job.Title)

		p.Agent.Title = strings.TrimSuffix(p.Agent.Title, " (advisory)") + " (advisory)"
		p.Agent.Brief = strings.TrimSpace(p.Agent.Brief + "\nYou are an AI that knows about the work of a " + title +
			". You are not a " + title + " and never say or imply that you are. You inform, explain and prepare " +
			"questions; for any diagnosis, decision, sign-off or work in person, say plainly that a qualified " +
			title + " is needed.")
		p.Professional = append(p.Professional, "A qualified "+title+" — for any diagnosis, decision, sign-off "+
			"or work in person. This agent informs and prepares; it is not one.")
		p.Limits = append(p.Limits, "Present itself as a "+title+", or give what only a "+title+" may give")
	}

	/*
	 * A developer with a command line and no way to read or write a file
	 * works entirely through the command line, which is the one tool that
	 * can reach anywhere. Whoever may change files or run commands may also
	 * read them — reading was never the risk.
	 */
	p.Agent.Tools = coherent(p.Agent.Tools)

	// And never past what whoever asked could use.
	if wish.Within != nil && !(len(p.Agent.Tools) == 1 && p.Agent.Tools[0] == NoTools) {
		if p.Agent.Tools = overlapOf(p.Agent.Tools, *wish.Within); len(p.Agent.Tools) == 0 {
			p.Agent.Tools = []string{NoTools}
		}
	}

	agent.Tools = p.Agent.Tools
	p.Tools = shownTools(agent.Tools)

	p.Risk = string(level)
	p.Itself = canItself(agent.Tools)
	p.You = needsYou(p, packages)

	p.Why = fmt.Sprintf("nobody here holds the job %s", strings.ToLower(job.Title))

	return p, nil
}

/*
 * alreadyHere is somebody on the organisation who already does this.
 *
 * Never the generalist, which does anything and so matches everything; and a
 * wish for somebody to lead is answered only by somebody in that job, since
 * knowing security does not make an engineer the head of it.
 */
func alreadyHere(in Inputs, job *store.Occupation, words, capabilities []string, named []capability.Package) (Agent, string, bool) {
	for _, a := range in.Roster {
		if !a.Working() || a.Name == "assistant" {
			continue
		}

		// Asked for by the package — "a Unity developer" — only somebody who
		// works under it is already here. Holding the job is not enough: a
		// Godot developer is not the Unity developer that was asked for.
		if !worksUnder(a, named) {
			continue
		}

		fit := Settle(a, in.Chart, in.Catalogue, in.Toolbox)

		holds := fit.Job != nil && fit.Job.ID == job.ID
		knows := len(capabilities) > 0 && knowsAll(fit.Can, capabilities) && !leadingWish(words)

		// Knowing the same things is not being the same kind of worker: a
		// developer who knows testing is not the penetration tester asked for.
		if head := roleNoun(words); knows && head != "" && (fit.Job == nil || !calledA(*fit.Job, head)) {
			knows = false
		}

		if knows || holds {
			return a, fmt.Sprintf("the %s already does this, so nobody new needs hiring",
				strings.ToLower(a.Title)), true
		}
	}

	return Agent{}, "", false
}

/*
 * packagesFor is the packages a hire works under.
 *
 * Those the request named outright, when it named any that serve the job —
 * "a Godot expert" is Godot's. Otherwise every package that serves the job:
 * a game developer asked for with no engine can work in any of them, and
 * which one is a question for each project rather than for the hire.
 */
func packagesFor(set *capability.Set, job string, named []capability.Package) []capability.Package {
	if set == nil {
		return nil
	}

	if len(named) > 0 {
		return named
	}

	var serving []capability.Package

	for _, p := range set.All() {
		if listed(p.Jobs, job) {
			serving = append(serving, p)
		}
	}

	return serving
}

// jobAmong is the job from a short list the words say most about, or nil
// when they say nothing about any of them.
func jobAmong(cat Catalogue, words, ids []string) *store.Occupation {
	var (
		best      *store.Occupation
		bestScore float64
	)

	for _, id := range ids {
		job, err := cat.Occupation(id)
		if err != nil || job == nil {
			continue
		}

		score := phraseIn(words, *job)

		for _, w := range words {
			score += float64(pointsFor(*job, stem(w)))
		}

		if score > bestScore {
			best, bestScore = job, score
		}
	}

	return best
}

/*
 * beyond is the words a wish uses that neither the job nor its packages
 * already answer to. "A book writer" is a writer working under the book
 * package — "book" says which package, not a speciality worth putting in the
 * title — where "a Laravel security specialist" asks for more than any job.
 */
func beyond(words []string, packages ...capability.Package) []string {
	var out []string

	for _, w := range words {
		covered := false

		for _, p := range packages {
			for _, cue := range append(append([]string{}, p.Cues...), strings.Fields(strings.ToLower(p.Role.Title))...) {
				if cue == w || stem(cue) == stem(w) {
					covered = true
				}
			}
		}

		if !covered {
			out = append(out, w)
		}
	}

	return out
}

// namedPackages is the packages a request names outright that serve the job.
func namedPackages(set *capability.Set, job, sentence string) []capability.Package {
	if set == nil {
		return nil
	}

	var named []capability.Package

	for _, p := range set.Named(sentence) {
		if listed(p.Jobs, job) {
			named = append(named, p)
		}
	}

	return named
}

// worksUnder is whether an agent works under every one of these packages.
func worksUnder(a Agent, packages []capability.Package) bool {
	for _, p := range packages {
		if !listed(a.Packages, p.ID) {
			return false
		}
	}

	return true
}

func listed(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}

	return false
}

/*
 * toolsFor is always a written list. Never "everything".
 *
 * The packages' roles when there are any, the template's otherwise, and what
 * the capabilities come to when there is neither — then cut to whatever the
 * asker could use, and the owner's own decisions taken out. When all of that
 * leaves nothing, the answer is none, said as none: an empty list in an agent
 * file means every tool, and a hire must never be a way to get that.
 */
func toolsFor(in Inputs, agent Agent, packages []capability.Package, fromTemplate bool, within *[]string) []string {
	var base []string

	known := true

	switch {
	case len(packages) > 0:
		for _, pkg := range packages {
			base = mergeNames(base, pkg.Role.Tools)
		}
	case fromTemplate && len(agent.Tools) > 0:
		base = append([]string{}, agent.Tools...)
	case in.Toolbox != nil:
		base = in.Toolbox.ForCapabilities(Settle(agent, in.Chart, in.Catalogue, nil).Can)
	default:
		known = false
	}

	if !known {
		if within == nil {
			return []string{NoTools}
		}

		base = append([]string{}, (*within)...)
	}

	if within != nil {
		base = overlapOf(base, *within)
	}

	out := []string{}

	for _, tool := range base {
		if tool == "decide_waiting" || tool == NoTools || listed(agent.Never, tool) {
			continue
		}

		out = append(out, tool)
	}

	if len(out) == 0 {
		return []string{NoTools}
	}

	return out
}

// withoutOperating is a tool list with everything that acts on the world
// taken out, and "none" when that leaves nothing.
func withoutOperating(tools []string) []string {
	out := []string{}

	for _, t := range tools {
		if !capability.Operates[t] && !strings.HasPrefix(t, "mcp_") && t != NoTools {
			out = append(out, t)
		}
	}

	if len(out) == 0 {
		return []string{NoTools}
	}

	return out
}

func shownTools(tools []string) []string {
	if len(tools) == 1 && tools[0] == NoTools {
		return []string{}
	}

	return tools
}

func orElseUses(uses, fallback string) string {
	switch uses {
	case UsesWork, UsesQuick, UsesBest, UsesReason, UsesTalk:
		return uses
	default:
		return fallback
	}
}

func orLines(lines []string, fallback string) []string {
	if len(lines) > 0 {
		return lines
	}

	if strings.TrimSpace(fallback) == "" {
		return nil
	}

	return []string{fallback}
}

func providerName(through string) string {
	if through == "" || through == llm.Local {
		return "this machine"
	}

	return through
}

/*
 * seatPlan is where a permanent hire would sit, decided without writing.
 *
 * A vacant seat for the same job first; otherwise a new seat in the unit
 * whose seats are nearest in kind; otherwise at the top. Accept adds the seat
 * to that unit as the chart stands then, rather than saving a copy of the
 * unit taken now — an edit somebody made in between is theirs to keep.
 */
func seatPlan(chart []org.Unit, roster []Agent, agent Agent, job *store.Occupation) (seat, in string) {
	filled := map[string]bool{}

	for _, a := range roster {
		if a.Working() && a.Position != "" {
			filled[a.Position] = true
		}
	}

	for _, s := range org.Seats(chart) {
		if s.Job == job.ID && !filled[s.Name] {
			return s.Name, ""
		}
	}

	category, _, _ := strings.Cut(job.ID, ".")

	for _, unit := range chart {
		for _, s := range unit.Seats {
			if c, _, _ := strings.Cut(s.Job, "."); c == category {
				return agent.Name, unit.Name
			}
		}
	}

	for _, unit := range org.Shape(chart) {
		if unit.Parent == "" {
			return agent.Name, unit.Name
		}
	}

	return "", ""
}

// canItself says in words what its tools let it do on its own.
func canItself(tools []string) []string {
	has := map[string]bool{}

	for _, t := range tools {
		has[t] = true
	}

	var out []string

	add := func(line string, any ...string) {
		for _, t := range any {
			if has[t] {
				out = append(out, line)

				return
			}
		}
	}

	add("Read files in the folders it is given", "read_file", "search_files", "list_directory", "read_document")
	add("Write and change files — every change is put to you unless you have allowed it", "write_file", "edit_file")
	add("Write documents", "write_document")
	add("Run the project's own commands, each shown to you exactly as it will run", "run_command")
	add("Check, run and build games headless, with the engine's own output kept", "game_check", "game_build", "godot_build")
	add("Look up the documentation for the version actually installed", "game_docs", "godot_docs")
	add("Look things up on the web, when your privacy setting allows it", "web_search", "fetch_url", "read_a_page")
	add("Make pictures", "make_a_picture")
	add("Look at a project before touching it", "inspect_project")

	if len(out) == 0 {
		out = append(out, "Think and advise — it has no tools that act")
	}

	return out
}

// needsYou is everything that waits for its owner.
func needsYou(p Proposal, packages []capability.Package) []string {
	var out []string

	for _, n := range p.Requires {
		if n.Optional || n.Status.Ready() {
			continue
		}

		s := n.Status

		switch {
		case s.Present && !s.Compatible:
			out = append(out, fmt.Sprintf("Update %s: %s", s.Title, s.Problem))
		case s.Blocked:
			out = append(out, s.Title+": "+s.Problem)
		case s.Installable:
			line := "Approve installing " + s.Title

			if s.Size != "" {
				line += " (" + s.Size + ")"
			}

			out = append(out, line+" — "+s.Source)
		default:
			out = append(out, s.Title+": "+s.Manual)
		}
	}

	for _, i := range p.Integrations {
		if !i.Optional && !i.State.Approved {
			out = append(out, "Approve the integration "+orElse(i.State.Title, i.ID)+" before it can be used")
		}
	}

	project := false

	for _, pkg := range packages {
		if pkg.Work != capability.Advisory {
			project = true
		}
	}

	if project {
		out = append(out, "Choose the folder it works in, for each project")
	}

	out = append(out, "Approve anything it changes, unless you have allowed that kind of change")

	return out
}

func orElse(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}

/*
 * Accept makes the hire that was proposed, and nothing else.
 *
 * forTask is the task a task-only hire belongs to; zero leaves it to whoever
 * starts that task to say. The name is checked again, because somebody may
 * have been hired under it since — a name is how a person is found, not what
 * they may do, so taking the next free one changes nothing that was approved.
 */
func Accept(root string, p Proposal, forTask int64) (Hired, error) {
	if p.Existing != nil {
		return Hired{Agent: *p.Existing, Existing: true, Job: p.Job.ID, Seat: p.Existing.Position, Why: p.Why}, nil
	}

	switch p.Permanence {
	case Permanent, TaskOnly:
	default:
		return Hired{}, fmt.Errorf("permanent or for one task? That has not been decided")
	}

	agent := p.Agent
	agent.Name = unusedName(Roster(root), agent.Name)

	hired := Hired{Job: p.Job.ID, Can: agent.Can}

	if p.Permanence == TaskOnly {
		agent.State = Temporary
		agent.HiredFor = forTask
		agent.Position = ""
		hired.Why = fmt.Sprintf("hired as a %s for this job only, from the job %s",
			strings.ToLower(agent.Title), strings.ToLower(p.Job.Title))
	} else {
		agent.State = Active
		agent.HiredFor = 0

		if p.Seat != "" {
			if err := takeSeat(root, p, agent); err != nil {
				return Hired{}, err
			}

			agent.Position = p.Seat
		}

		hired.Seat = agent.Position
		hired.Why = fmt.Sprintf("hired a %s from the job %s", strings.ToLower(agent.Title),
			strings.ToLower(p.Job.Title))
	}

	if err := Save(root, agent); err != nil {
		return Hired{}, err
	}

	Forget()

	hired.Agent = agent

	return hired, nil
}

// takeSeat adds a new seat to its unit as the chart stands now. A vacant seat
// already there needs nothing written.
func takeSeat(root string, p Proposal, agent Agent) error {
	if p.SeatIn == "" {
		return nil
	}

	chart := org.Chart(root)

	unit, ok := org.Find(chart, p.SeatIn)
	if !ok {
		return fmt.Errorf("the unit %q is no longer on the chart", p.SeatIn)
	}

	for _, s := range unit.Seats {
		if s.Name == p.Seat {
			return nil
		}
	}

	seat := org.Position{Name: p.Seat, Title: agent.Title, Job: p.Job.ID, Seniority: org.Senior}

	for _, other := range unit.Seats {
		if other.ReportsTo == "" {
			seat.ReportsTo = other.Name

			break
		}
	}

	unit.Seats = append(append([]org.Position{}, unit.Seats...), seat)

	return org.Save(root, unit)
}

// Hire answers a wish with somebody already here, or somebody new — the
// proposal and its acceptance, one after the other, for the conductor.
func Hire(root string, roster []Agent, chart []org.Unit, cat Catalogue, box Toolbox, templates []Template, wish Wish) (Hired, error) {
	return HireWith(Inputs{Root: root, Roster: roster, Chart: chart, Catalogue: cat, Toolbox: box,
		Templates: templates}, wish)
}

// HireWith is Hire with everything a proposal can use.
func HireWith(in Inputs, wish Wish) (Hired, error) {
	p, err := Propose(in, wish)
	if err != nil {
		return Hired{}, err
	}

	if p.Existing != nil {
		return Accept(in.Root, p, 0)
	}

	if wish.ForTask != 0 {
		p.Permanence = TaskOnly
	} else {
		p.Permanence = Permanent
	}

	// A task's own hire, made without asking, reaches no further than the
	// task could. See Widens.
	if wish.ForTask != 0 {
		if why := Widens(p); why != "" {
			return Hired{Proposal: &p}, &NeedsApproval{Why: why, Proposal: p}
		}
	}

	return Accept(in.Root, p, wish.ForTask)
}

/*
 * Widens is why a hire made by a task would change what the organisation can
 * reach, or empty when it would not.
 *
 * A task may take somebody on for itself without asking, as its owner chose —
 * a new role inside what whoever it works for could already do. Not when the
 * hire would reach further: a hosted service of its own, an integration, or
 * any permanent place. Those are proposals its owner decides on, and the step
 * goes to somebody already here meanwhile.
 */
func Widens(p Proposal) string {
	var more []string

	if p.Permanence == Permanent {
		more = append(more, "it would be permanent")
	}

	if p.Agent.Provider != "" && p.Agent.Provider != "ollama" {
		more = append(more, "it would think through "+p.Agent.Provider+", off this machine")
	}

	for _, tool := range p.Agent.Tools {
		if strings.HasPrefix(tool, "mcp_") {
			more = append(more, "it would use the integration tool "+tool)
		}
	}

	return strings.Join(more, "; ")
}

// NeedsApproval is a hire a task may not make by itself.
type NeedsApproval struct {
	Why      string
	Proposal Proposal
}

func (n *NeedsApproval) Error() string { return "this hire needs its owner's approval: " + n.Why }

// Text is the proposal written out for a person deciding on it.
func (p Proposal) Text() string {
	var b strings.Builder

	if p.Existing != nil {
		fmt.Fprintf(&b, "Nobody new is needed: %s (%s) already does this.", p.Existing.Title, p.Existing.Name)

		return b.String()
	}

	a := p.Agent

	fmt.Fprintf(&b, "Proposed hire: %s, called %s\n", a.Title, a.Name)
	fmt.Fprintf(&b, "Job: %s (%s)", p.Job.Title, p.Job.ID)

	if p.Seat != "" {
		where := "a vacant seat"
		if p.SeatIn != "" {
			where = "a new seat in " + orElse(p.SeatUnit, p.SeatIn)
		}

		fmt.Fprintf(&b, " — sitting in %s (%s)", where, p.Seat)
	}

	b.WriteString("\n")

	switch p.Permanence {
	case Permanent:
		b.WriteString("Permanent: saved in the organisation, available for later work.\n")
	case TaskOnly:
		b.WriteString("For one task only: recorded, and let go when the task ends.\n")
		if p.Goal != "" {
			b.WriteString("The task: " + p.Goal + "\n")
		}
	default:
		b.WriteString("Not decided yet: permanent, or for one task only?\n")
	}

	fmt.Fprintf(&b, "Risk: %s\n", p.Risk)

	section := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}

		b.WriteString("\n" + title + ":\n")

		for _, line := range lines {
			b.WriteString("  - " + line + "\n")
		}
	}

	section("Responsible for", p.Responsibilities)

	var packages []string

	for _, pkg := range p.Packages {
		line := fmt.Sprintf("%s %s — %s (%s)", pkg.ID, pkg.Version, pkg.Title, pkg.Work)

		if len(pkg.Needs) > 0 {
			line += "; relies on " + strings.Join(pkg.Needs, "; ")
		}

		packages = append(packages, line)
	}

	section("Capability packages", packages)

	model := p.Model.Uses + " model"
	if p.Model.Name != "" {
		model = p.Model.Name + " (the " + p.Model.Uses + " model)"
	}

	section("Thinks with", []string{model + ", on " + p.Model.Provider})

	if a.Brief != "" {
		section("Standing instructions", []string{strings.ReplaceAll(a.Brief, "\n", " ")})
	}

	section("Skills", p.Skills)

	tools := p.Tools
	if len(tools) == 0 {
		tools = []string{"none — its work is judgement"}
	}

	section("May use", []string{strings.Join(tools, ", ")})
	section("Must not", p.Limits)

	var needs []string

	for _, n := range p.Requires {
		needs = append(needs, neededLine(n))
	}

	section("Needs on this machine", needs)

	var integrations []string

	for _, i := range p.Integrations {
		state := "not on the approved list"

		switch {
		case i.State.Approved:
			state = "approved"
		case i.State.Known:
			state = "on the list, not approved — needs your approval"
		}

		where := "on this machine"
		if i.State.Remote {
			where = "outside this machine"
		}

		integrations = append(integrations, fmt.Sprintf("%s — %s; %s, %s", orElse(i.State.Title, i.ID), i.Why, where, state))
	}

	section("Integrations (MCP)", integrations)
	section("It can do itself", p.Itself)
	section("Needs you", p.You)
	section("Needs a qualified professional", p.Professional)

	return strings.TrimSpace(b.String())
}

func neededLine(n Needed) string {
	s := n.Status

	line := s.Title

	if s.Wanted != "" {
		line += " " + s.Wanted
	}

	switch {
	case s.Ready():
		line += fmt.Sprintf(" — here (%s)", orElse(s.Version, "version not said"))
	case s.Present:
		line += " — here, but " + s.Problem
	case s.Blocked:
		line += " — " + s.Problem
	default:
		line += " — missing"
	}

	if n.Optional {
		line += " [optional: " + n.Why + "]"
	} else {
		line += " [" + n.Why + "]"
	}

	if !s.Ready() {
		var terms []string

		for _, part := range []string{s.Size, s.Source, s.Licence, s.Cost} {
			if part != "" {
				terms = append(terms, part)
			}
		}

		if len(terms) > 0 {
			line += "; " + strings.Join(terms, "; ")
		}

		if !s.Installable && s.Manual != "" {
			line += "; cannot be installed from here: " + s.Manual
		}
	}

	return line
}

// DropHire removes an agent's file, for a task-only hire whose task could not
// be started.
func DropHire(root, name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("that is not an agent's name")
	}

	err := os.Remove(filepath.Join(Folder(root), name+".md"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	Forget()

	return nil
}

// UnusedName is name, or name with a number after it when somebody here is
// already called that.
func UnusedName(roster []Agent, name string) string { return unusedName(roster, name) }

// WithTools gives a proposal's hire exactly these tools, never an empty list:
// empty would mean all of them.
func WithTools(p *Proposal, tools []string) {
	if len(tools) == 0 {
		tools = []string{NoTools}
	}

	p.Agent.Tools = append([]string{}, tools...)
	p.Tools = shownTools(p.Agent.Tools)
}

// coherent is a tool list that can read what it may change: whoever may
// write files or run commands is given the tools that only read them.
func coherent(tools []string) []string {
	acts := false

	for _, t := range tools {
		switch t {
		case "write_file", "edit_file", "run_command", "write_document", "edit_document":
			acts = true
		}
	}

	if !acts {
		return tools
	}

	out := append([]string{}, tools...)

	for _, reading := range []string{"read_file", "list_directory", "search_files"} {
		if !listed(out, reading) {
			out = append(out, reading)
		}
	}

	if listed(out, "run_command") {
		for _, writing := range []string{"write_file", "edit_file"} {
			if !listed(out, writing) {
				out = append(out, writing)
			}
		}
	}

	return out
}
