/*
 * Package bootstrap turns "make me a Tetris game" into a proposal somebody can
 * say yes to.
 *
 * In the order a careful person would go, and stopping at the first thing
 * that needs its owner:
 *
 *   where        never guessed. With no folder, the only answer is a question.
 *   look         the folder is inspected before anything is proposed for it.
 *   what with    the engine or kind of work, from what was said, what the
 *                folder already is, or — when neither says — a question with
 *                the options and what each would cost.
 *   who          somebody already here who works that way, or a hire for
 *                this job only, proposed with everything else.
 *   what it      what the machine needs, what is missing, what can be
 *   takes        installed from here and what cannot.
 *   the plan     the steps, who does them, and what finished means — in the
 *                evidence that will have to exist, not in a sentence.
 *
 * Nothing here writes, installs or hires. The brain keeps the proposal and
 * carries out exactly it once its owner has approved it.
 */
package bootstrap

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/brain/workspace"
)

// WhereQuestion is asked, in these words, whenever no folder was given.
const WhereQuestion = "Where should I create or work on this project?"

// Inputs is what a proposal is worked out from.
type Inputs struct {
	Engines  *engines.Registry
	Packages *capability.Set
	Recipes  *provision.Book

	// Hiring is how a hire would be proposed, when nobody here fits.
	Hiring team.Inputs

	// Own are this program's own folders, which no project goes near.
	Own []string

	/*
	 * Choose picks among the packages that could do the work when the
	 * request names none — the engine for a game — and says why; nil, or an
	 * answer with Ask, puts the question to the owner instead.
	 */
	Choose func(request string, candidates []capability.Package) Chosen

	// Stack is how the work would be done — who writes it, with which
	// model, and what falls back — in lines, for the proposal.
	Stack func(dir string, pkg capability.Package) []string

	// Time is how long the work is given, in minutes, and why that long.
	Time func(dir string, pkg capability.Package) (int, string)
}

// Chosen is a package chosen for the owner, and why.
type Chosen struct {
	Package capability.Package
	Reasons []string
	Ask     string
}

// Request is what was asked, and what has been answered so far.
type Request struct {
	Sentence string `json:"sentence"`
	Dir      string `json:"dir,omitempty"`

	// Package is a capability package chosen by its owner, answering the
	// question of which engine or kind of work.
	Package string `json:"package,omitempty"`
}

// Proposal is a project worked out and not yet started.
type Proposal struct {
	ID      int64   `json:"id,omitempty"`
	Request Request `json:"request"`

	// Question is what must be answered before anything else is decided,
	// and Options the answers, when there are a few obvious ones.
	Question string   `json:"question,omitempty"`
	Options  []Option `json:"options,omitempty"`

	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
	Dir  string `json:"dir,omitempty"`

	// New is a folder to be made into a project; otherwise an existing
	// project is worked on as it is.
	New    bool             `json:"new"`
	Report workspace.Report `json:"report"`

	Package       *team.PackageView `json:"package,omitempty"`
	Engine        string            `json:"engine,omitempty"`
	EngineVersion string            `json:"engine_version,omitempty"`

	// Files are what a new project starts with, by path; nothing that exists
	// is among them.
	Files  []string         `json:"files,omitempty"`
	Config workspace.Config `json:"config"`

	// Agent is who does the work: somebody here, or Hire, proposed for this
	// project only.
	Agent string         `json:"agent,omitempty"`
	Hire  *team.Proposal `json:"hire,omitempty"`

	Requires []team.Needed `json:"requires,omitempty"`

	// Installs are the recipes installed first, as part of what is approved.
	Installs []string `json:"installs,omitempty"`

	// Chosen is why its package — its engine, for a game — was chosen, when
	// it was chosen rather than named.
	Chosen []string `json:"chosen,omitempty"`

	// Stack is how the work will be done: who writes it, with what, and
	// what falls back, as the orchestrator decided it.
	Stack []string `json:"stack,omitempty"`

	// Minutes is how long the work is given before it stops as unfinished,
	// and MinutesWhy why that long; approved with the rest.
	Minutes    int    `json:"minutes,omitempty"`
	MinutesWhy string `json:"minutes_why,omitempty"`

	// Blockers are what stops it starting at all, until its owner acts.
	Blockers []string `json:"blockers,omitempty"`

	Steps []store.TaskStep `json:"steps,omitempty"`

	// Done is what finished means, as the evidence that must exist.
	Done []string `json:"done,omitempty"`

	Limits       []string `json:"limits,omitempty"`
	Professional []string `json:"professional,omitempty"`
}

// Option is one answer to a question, and what choosing it would mean.
type Option struct {
	Package string `json:"package"`
	Title   string `json:"title"`
	Says    string `json:"says"`
}

// Ready is a proposal with nothing left to ask and nothing in the way.
func (p Proposal) Ready() bool { return p.Question == "" && len(p.Blockers) == 0 }

/*
 * Kinds of work, by the words that name them. Only for choosing between
 * packages when the request names none outright: "a Tetris game" is a game,
 * and which engine is then the question.
 */
var kinds = []struct {
	kind  string
	job   string
	words []string
}{
	{"game", "software.game_developer", []string{"game", "games", "tetris", "platformer", "puzzle", "arcade",
		"shooter", "snake", "pong", "breakout", "asteroids", "rpg", "roguelike", "игра", "игрa"}},
	{"book", "media.writer", []string{"book", "novel", "novella", "memoir", "manuscript", "книга", "роман"}},
	{"api", "software.backend_engineer", []string{"api", "backend", "server", "endpoint", "microservice"}},
	{"web", "software.frontend_engineer", []string{"website", "frontend", "landing", "webpage", "page"}},
	{"plan", "trades.electrician", []string{"electrical", "wiring", "circuits", "socket", "sockets"}},
	{"plan", "engineering.civil_engineer", []string{"construction", "structural", "extension", "renovation"}},
}

func kindOf(sentence string) (kind, job string) {
	words := map[string]bool{}

	for _, w := range strings.FieldsFunc(strings.ToLower(sentence), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		words[w] = true
	}

	for _, k := range kinds {
		for _, w := range k.words {
			if words[w] {
				return k.kind, k.job
			}
		}
	}

	return "", ""
}

// kindOfPackage is what a package's work makes.
func kindOfPackage(p capability.Package) string {
	switch {
	case p.Engine != "":
		return "game"
	case p.ID == "writing.book":
		return "book"
	case p.ID == "software.backend":
		return "api"
	case p.ID == "software.frontend":
		return "web"
	case p.Work == capability.Advisory:
		return "plan"
	}

	return "other"
}

/*
 * Propose works a request out as far as it can, and stops at the first
 * question.
 */
func Propose(in Inputs, r Request) Proposal {
	p := Proposal{Request: r, Name: nameOf(r.Sentence)}

	if strings.TrimSpace(r.Dir) == "" {
		p.Question = WhereQuestion

		return p
	}

	dir := expand(r.Dir)

	report := workspace.Inspect(dir, in.Engines, in.Own...)

	/*
	 * A folder that holds other things and is not a project is a place to
	 * put one, not the project: the new project goes into a folder of its
	 * own inside it, named for what it is.
	 */
	if report.Refused == "" && report.Exists && !report.Empty && report.Engine == "" &&
		len(report.Stacks) == 0 && report.Config == nil {
		dir = filepath.Join(dir, safeFolder(p.Name))
		report = workspace.Inspect(dir, in.Engines, in.Own...)
	}

	p.Dir, p.Report = dir, report

	if report.Refused != "" {
		p.Question = "I can't use " + dir + ": " + report.Refused + ". " + WhereQuestion

		return p
	}

	pkg, question, options, why := choosePackage(in, r, report)
	if question != "" {
		p.Question, p.Options = question, options

		return p
	}

	p.Chosen = why

	p.New = !report.Exists || report.Empty
	p.Kind = kindOfPackage(pkg)
	p.Package = &team.PackageView{ID: pkg.ID, Version: pkg.Version, Title: pkg.Title, Work: string(pkg.Work),
		Needs: pkg.Needs(), Evidence: pkg.Evidence}
	p.Engine = pkg.Engine

	if in.Stack != nil && p.Dir != "" {
		p.Stack = in.Stack(p.Dir, pkg)
	}

	if in.Time != nil && p.Dir != "" {
		p.Minutes, p.MinutesWhy = in.Time(p.Dir, pkg)
	}
	p.Limits = pkg.Limits

	if pkg.Escalation != "" {
		p.Professional = append(p.Professional, pkg.Escalation)
	}

	p.requirements(in, pkg)
	p.scaffold(in, pkg)
	p.staff(in, pkg)
	p.plan(pkg)

	return p
}

func expand(dir string) string {
	dir = strings.TrimSpace(dir)

	if strings.HasPrefix(dir, "~/") || dir == "~" {
		if home := homeDir(); home != "" {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		}
	}

	return filepath.Clean(dir)
}

/*
 * choosePackage is what the work is done under.
 *
 * What the owner chose, when they have; the engine the folder already is;
 * what the request names outright; and otherwise a question — "a Tetris game"
 * does not say Godot or three.js, and choosing for somebody is exactly the
 * guess this flow exists not to make.
 */
func choosePackage(in Inputs, r Request, report workspace.Report) (capability.Package, string, []Option, []string) {
	if r.Package != "" {
		if p, ok := in.Packages.Get(r.Package); ok {
			return p, "", nil, []string{"you chose it"}
		}
	}

	if report.Engine != "" {
		for _, p := range in.Packages.All() {
			if p.Engine == report.Engine {
				return p, "", nil, []string{"the project is already a " + p.Engine + " project"}
			}
		}
	}

	if report.Config != nil {
		for _, id := range report.Config.Packages {
			if p, ok := in.Packages.Get(id); ok {
				return p, "", nil, []string{"the project's settings say so"}
			}
		}
	}

	if named := in.Packages.Named(r.Sentence); len(named) > 0 {
		return named[0], "", nil, []string{"you asked for it by name"}
	}

	kind, job := kindOf(r.Sentence)

	candidates := []capability.Package{}

	for _, p := range in.Packages.All() {
		if listed(p.Jobs, job) && (kind != "game" || p.Engine != "") {
			candidates = append(candidates, p)
		}
	}

	switch len(candidates) {
	case 0:
		return capability.Package{}, "What kind of project is it — a game, a book, a web page, an API, a plan? " +
			"Say which, and what it should be built with if you know.", nil, nil
	case 1:
		return candidates[0], "", nil, []string{"it is the one way here to do this kind of work"}
	}

	/*
	 * Chosen, not asked, when it can be: which engine is a question the
	 * machine and the request answer between them — what is installed and
	 * licensed, what can be checked and photographed, whether it has to run
	 * in a browser — and its owner is asked only when they do not.
	 */
	var ask string

	if in.Choose != nil {
		chosen := in.Choose(r.Sentence, candidates)
		if chosen.Ask == "" && chosen.Package.ID != "" {
			return chosen.Package, "", nil, chosen.Reasons
		}

		ask = chosen.Ask
	}

	options := make([]Option, 0, len(candidates))

	for _, c := range candidates {
		options = append(options, Option{Package: c.ID, Title: c.Title, Says: readiness(in, c)})
	}

	question := "Which should it be built with?"
	if kind == "game" {
		question = "Which engine should it use?"
	}

	if ask != "" {
		question = ask + "\n" + question
	}

	return capability.Package{}, question, options, nil
}

// readiness says what choosing a package would take on this machine.
func readiness(in Inputs, p capability.Package) string {
	var missing []string

	for _, r := range p.Mandatory() {
		s := in.Recipes.Check(r.ID, r.Versions)

		if s.Ready() {
			continue
		}

		switch {
		case s.Present && s.Compatible && s.LicenceState == provision.LicenceInactive:
			missing = append(missing, s.Title+" is installed but not licensed to work unattended — "+
				"its owner signs in to "+hubFor(s.ID))
		case s.Present && s.Compatible && s.LicenceState != "":
			missing = append(missing, s.Title+" is installed; whether its licence lets it work unattended could not be told")
		case s.Blocked:
			missing = append(missing, s.Title+" is "+s.Problem)
		case s.Installable:
			missing = append(missing, fmt.Sprintf("%s would be installed (%s, %s)", s.Title, orSize(s.Size), s.Licence))
		default:
			missing = append(missing, s.Title+" is not here, and you would install and license it yourself")
		}
	}

	if len(missing) == 0 {
		switch p.Engine {
		case "threejs":
			return "ready now — it runs in a browser, and nothing needs installing"
		default:
			return "ready now — everything it needs is here"
		}
	}

	return strings.Join(missing, "; ")
}

// hubFor is where the owner of a licensed engine signs in.
func hubFor(recipe string) string {
	if recipe == "unity" {
		return "Unity Hub"
	}

	return "the engine's own launcher"
}

func orSize(size string) string {
	if size == "" {
		return "size not known"
	}

	return size
}

func listed(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}

	return false
}

// requirements is what the package needs, as it stands here now, and what
// would be installed as part of saying yes.
func (p *Proposal) requirements(in Inputs, pkg capability.Package) {
	for _, r := range pkg.Requires {
		s := in.Recipes.Check(r.ID, r.Versions)

		p.Requires = append(p.Requires, team.Needed{Package: pkg.ID, Why: r.Why, Optional: r.Optional, Status: s})

		if r.Optional || s.Ready() {
			continue
		}

		switch {
		case s.Present && s.Compatible && s.LicenceState != "":
			// Installed and not usable: a licence is its owner's to sort
			// out, and saying "update it" would send them the wrong way.
			p.Blockers = append(p.Blockers, fmt.Sprintf("%s is %s", s.Title, s.Problem))
		case s.Present:
			p.Blockers = append(p.Blockers, fmt.Sprintf("%s is here but %s — update it first", s.Title, s.Problem))
		case s.Blocked:
			// Here and unusable. Installing a second copy over it would not
			// fix the permission and might not be allowed either.
			p.Blockers = append(p.Blockers, fmt.Sprintf("%s: %s", s.Title, s.Problem))
		case s.Installable:
			p.Installs = append(p.Installs, r.ID)
		default:
			p.Blockers = append(p.Blockers, s.Title+" is needed and cannot be installed from here: "+s.Manual)
		}
	}
}

// scaffold is what a new project starts with, and its settings.
func (p *Proposal) scaffold(in Inputs, pkg capability.Package) {
	c := workspace.Config{
		Version: workspace.Version, Name: p.Name, Kind: p.Kind, Engine: pkg.Engine,
		Packages: []string{pkg.ID}, Roots: []string{"."},
	}

	if pkg.Workspace != nil {
		c.Outputs = pkg.Workspace.Outputs
		c.Conventions = pkg.Workspace.Conventions
	}

	if p.Report.Commands != nil {
		c.Commands = p.Report.Commands
	}

	if existing := p.Report.Config; existing != nil {
		c = *existing
	}

	if adapter, ok := in.Engines.Get(pkg.Engine); ok {
		if engine, found := adapter.Engine(); found {
			p.EngineVersion = engine.Version
			c.EngineVersion = engine.Version
		}
	}

	p.Config = c

	if !p.New {
		return
	}

	var files []engines.File

	switch {
	case pkg.Engine != "":
		if adapter, ok := in.Engines.Get(pkg.Engine); ok {
			made, err := adapter.Scaffold(engines.Scaffold{Name: p.Name, Engine: p.EngineVersion})
			if err != nil {
				p.Blockers = append(p.Blockers, "a new "+adapter.Title()+" project cannot be made here: "+err.Error())
			}

			files = made
		}
	case pkg.Workspace != nil && pkg.Workspace.Template != "":
		files, _ = workspace.Template(pkg.Workspace.Template, p.Name)
	case pkg.Work == capability.Advisory:
		files, _ = workspace.Template("plan", p.Name)
	}

	for _, f := range files {
		p.Files = append(p.Files, f.Path)
	}
}

/*
 * staff is who does the work: somebody here who works under the package, or
 * a hire for this project alone — proposed here, made only if the project is
 * approved, and let go when its work ends.
 */
func (p *Proposal) staff(in Inputs, pkg capability.Package) {
	for _, a := range in.Hiring.Roster {
		if a.Working() && listed(a.Packages, pkg.ID) {
			p.Agent = a.Name

			return
		}
	}

	if in.Hiring.Catalogue == nil {
		return
	}

	hire, err := team.Propose(in.Hiring, team.Wish{Sentence: "a " + strings.ToLower(pkg.Role.Title), Package: pkg.ID})
	if err != nil {
		p.Blockers = append(p.Blockers, "nobody could be found or hired to do it: "+err.Error())

		return
	}

	if hire.Existing != nil {
		p.Agent = hire.Existing.Name

		return
	}

	hire.Permanence = team.TaskOnly
	p.Hire = &hire
	p.Agent = hire.Agent.Name
}

// plan is the steps and what finished means.
func (p *Proposal) plan(pkg capability.Package) {
	dir := p.Dir
	what := strings.TrimSpace(p.Request.Sentence)

	step := func(instruction, doneWhen, kind string, changes bool) store.TaskStep {
		return store.TaskStep{Instruction: instruction, DoneWhen: doneWhen, Kind: kind, Changes: changes, Assignee: p.Agent}
	}

	output := "its output folder"
	if len(p.Config.Outputs) > 0 {
		output = filepath.Join(dir, p.Config.Outputs[0])
	}

	switch p.Kind {
	case "game":
		/*
		 * Writing is somebody's work; checking, running and building are this
		 * program's, done with the engine itself and not asked of a model —
		 * each has an action, and a check that fails goes back to whoever
		 * writes the game with what it found, until it passes or rounds run out.
		 */
		acting := func(instruction, doneWhen, kind, action string, changes bool) store.TaskStep {
			s := step(instruction, doneWhen, kind, changes)
			s.Action = action

			return s
		}

		check := acting(fmt.Sprintf("Check the project at %s with game_check and fix every error it reports, "+
			"until it reports none.", dir), "game_check reports no errors", store.StepCheck, "engine:check", true)
		smoke := acting(fmt.Sprintf("Run the game at %s for a few seconds (game_build, what: smoke), photograph it, "+
			"and fix whatever goes wrong while it runs.", dir), "the smoke run passes", store.StepCheck, "engine:smoke", true)
		build := acting(fmt.Sprintf("Build the game at %s with game_build (what: build) into %s.", dir, output),
			"a build is in "+output, store.StepDo, "engine:build", true)

		p.Steps = []store.TaskStep{
			step(fmt.Sprintf("In the project at %s, write the game that was asked for: %s. Keep the "+
				"rules apart from the drawing, and make it playable and visible from the first frame.",
				dir, what), "the game's files are written in the project", store.StepDo, true),
			check,
		}

		switch {
		case p.Engine == "unity" || p.Engine == "unreal":
			// A player is run once it has been built.
			p.Steps = append(p.Steps, build, smoke)
		case p.Engine != "godot" || templatesReady(p.Requires):
			p.Steps = append(p.Steps, smoke, build)
		default:
			p.Steps = append(p.Steps, smoke)
		}

	case "book":
		p.Steps = []store.TaskStep{
			step(fmt.Sprintf("In %s, write the outline in notes/outline.md and the people in it in "+
				"notes/characters.md, for: %s.", dir, what), "the outline and the characters are written", store.StepWrite, true),
			step(fmt.Sprintf("Draft the first chapter in %s following the outline.",
				filepath.Join(dir, "manuscript", "01.md")), "chapter one is drafted", store.StepWrite, true),
		}

	case "api", "web":
		test := "the project's own tests"

		if argv := p.Config.Commands["test"]; len(argv) > 0 {
			test = strings.Join(argv, " ")
		}

		p.Steps = []store.TaskStep{
			step(fmt.Sprintf("Look at the project at %s with inspect_project and read its conventions "+
				"before changing anything.", dir), "the project's conventions are known", store.StepLook, false),
			step(fmt.Sprintf("In %s, build what was asked for: %s.", dir, what), "the code is written", store.StepDo, true),
			step(fmt.Sprintf("Run %s in %s with run_command and fix what fails.", test, dir),
				"the tests pass", store.StepCheck, true),
		}

	default:
		p.Steps = []store.TaskStep{
			step(fmt.Sprintf("Read what is in %s — drawings, documents, notes — and find out what the work "+
				"depends on: %s.", dir, what), "what the work depends on is known", store.StepLook, false),
			step(fmt.Sprintf("Write the plan and a checklist in %s.", filepath.Join(dir, "plans", "plan.md")),
				"the plan is written", store.StepWrite, true),
			step(fmt.Sprintf("Write the questions a qualified professional must answer in %s.",
				filepath.Join(dir, "plans", "questions.md")), "the questions are written", store.StepWrite, true),
		}
	}

	words := map[string]string{
		capability.EvidenceFiles: "the files that were written, by path", capability.EvidenceCheck: "a check that passed",
		capability.EvidenceBuild: "a build that succeeded", capability.EvidenceTest: "tests that passed",
		capability.EvidenceExport: "an exported game, by path", capability.EvidenceSmoke: "a run that did not fail",
		capability.EvidenceShot: "a picture of it running", capability.EvidenceDocument: "a written document, by path",
		capability.EvidenceSources: "a source for every claim", capability.EvidenceQuestions: "the questions for a qualified person",
		capability.EvidenceVersions: "the software and versions it was made with",
	}

	for _, e := range pkg.Evidence {
		p.Done = append(p.Done, words[e])
	}

	p.Config.Acceptance = append(p.Config.Acceptance, p.Done...)
}

func templatesReady(needs []team.Needed) bool {
	for _, n := range needs {
		if n.Status.ID == "godot-templates" {
			return n.Status.Ready()
		}
	}

	return false
}

func orName(engine string) string {
	if engine == "" {
		return "library"
	}

	return engine
}

var leadIn = regexp.MustCompile(`(?i)^(?:(?:hey|hi|ok|okay)\s+\w+[,!]?\s+)?(?:please\s+|can you\s+|could you\s+)*` +
	`(?:(?:make|create|build|write|start|set up|develop|draft|plan)\s+(?:me\s+|us\s+)?)?(?:a\s+|an\s+|the\s+|my\s+)?`)

var trailing = regexp.MustCompile(`(?i)\s+(?:game|project|app|application|website|in\s+.+|with\s+.+|using\s+.+|for\s+.+)$`)

/*
 * nameOf is what the project is called, from what was asked: "make me a
 * Tetris game" is Tetris. Words, not a guess at a brand — and "New project"
 * when there is nothing to take a name from.
 */
func nameOf(sentence string) string {
	sentence = strings.TrimSpace(sentence)

	// Named outright: "… called Starfall".
	if m := calledName.FindStringSubmatch(sentence); m != nil {
		return capitalised(strings.Trim(m[1], ".!?\"' "))
	}

	name := strings.TrimSpace(leadIn.ReplaceAllString(sentence, ""))

	// What it is about, when it says: "a game where you catch falling stars".
	about := ""
	if m := aboutClause.FindStringSubmatchIndex(name); m != nil {
		about = strings.TrimSpace(name[m[2]:m[3]])
		name = strings.TrimSpace(name[:m[0]])
	}

	for {
		shorter := strings.TrimSpace(trailing.ReplaceAllString(name, ""))
		if shorter == name || shorter == "" {
			break
		}

		name = shorter
	}

	// The engine is not its name: "a Unity game" is a game made with Unity,
	// and a project called Unity is a class called Unity, which the engine
	// already has. Nor are the words for what size or kind it is.
	engine := ""
	if m := engineWord.FindStringSubmatch(name); m != nil {
		engine = m[1]
	}

	var kept []string

	for _, w := range strings.Fields(engineWord.ReplaceAllString(name, " ")) {
		if !generic[strings.ToLower(strings.Trim(w, ".,"))] {
			kept = append(kept, w)
		}
	}

	name = strings.Trim(strings.Join(kept, " "), ".!? ")

	switch {
	case name != "" && len([]rune(name)) <= 40:
		return capitalised(name)
	case about != "" && len([]rune(about)) <= 40:
		return capitalised(about)
	case engine != "":
		return capitalised(engine + " game")
	}

	return "New project"
}

var (
	calledName  = regexp.MustCompile(`(?i)\b(?:called|named|titled)\s+["']?([^,.;!?"']{1,40})`)
	aboutClause = regexp.MustCompile(`(?i)\s+(?:where\s+(?:you|the player|players|a player)\s+|in which\s+(?:you\s+)?|that\s+lets you\s+)(.+)$`)
	engineWord  = regexp.MustCompile(`(?i)\b(three\.?js|godot|unity|unreal(?: engine)?|phaser|babylon(?:\.?js)?|playcanvas|bevy|defold|gamemaker)\b`)
	generic     = map[string]bool{"small": true, "simple": true, "little": true, "new": true, "quick": true,
		"2d": true, "3d": true, "browser": true, "web": true, "game": true, "project": true, "engine": true}
)

func capitalised(name string) string {
	r := []rune(strings.TrimSpace(name))
	if len(r) == 0 {
		return "New project"
	}

	r[0] = unicode.ToUpper(r[0])

	return string(r)
}

// safeFolder is a name as a folder: lower case, dashes.
func safeFolder(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}

	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}

	return "new-project"
}
