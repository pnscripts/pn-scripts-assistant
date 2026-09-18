/*
 * Package capability is what a role needs to work, safely and well.
 *
 * A job says what somebody is for. A template says what they may touch. What
 * neither says is everything else a real hire needs on the first morning: the
 * engine installed and in a version that works, the documentation for that
 * version rather than a newer one, the tools that operate it and the ones that
 * must not be touched, what finished looks like and what proves it — and, for
 * an electrician or a structural engineer, the line past which an AI must stop
 * and a qualified person take over.
 *
 * A package says all of that, declared rather than coded. The hiring code
 * does not know what Godot is or what an electrician may not do: it reads a
 * package and applies it. So adding a profession's needs is writing a file,
 * never changing the program, and there is no `if job == electrician` for
 * anybody to forget to update.
 *
 * Some rules are not the package's to relax, and are checked in code when a
 * package is read: advisory work carries no tool that operates anything, and
 * work that needs a qualified professional is advisory, says so, and names who.
 * A package that breaks one is refused whole, not half applied.
 */
package capability

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/risk"
)

// FolderName is where its owner's own packages live, inside the brain's
// folder, beside agents and templates.
const FolderName = "packages"

/*
 * Work is what a package's work does to the world, which is the first thing
 * anybody deciding whether to approve it needs to know.
 */
type Work string

const (
	// Advisory is research, planning, writing and analysis. What comes out
	// is a document; nothing is operated.
	Advisory Work = "advisory"

	// Project changes files: code, assets, a project folder.
	Project Work = "project"

	// Operating runs software or drives hardware.
	Operating Work = "operating"
)

// Package is one declared set of needs.
type Package struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Title   string `json:"title"`
	Summary string `json:"summary"`

	Work Work `json:"work"`

	// Jobs are the occupations this serves, by taxonomy id.
	Jobs []string `json:"jobs"`

	// Cues are words in a request that point at this package: "godot",
	// "tetris", "novel". Lower case.
	Cues []string `json:"cues,omitempty"`

	// Engine names an engine adapter, for game packages.
	Engine string `json:"engine,omitempty"`

	Role Role `json:"role"`

	// Skills are standing know-how, a line each, added to the brief.
	Skills []string `json:"skills,omitempty"`

	// Knowledge is where the authoritative answers are, for this version.
	Knowledge []Source `json:"knowledge,omitempty"`

	Requires     []Requirement `json:"requires,omitempty"`
	Integrations []Integration `json:"integrations,omitempty"`

	Workspace *Workspace `json:"workspace,omitempty"`

	// Evidence is what has to exist before work under this package is called
	// finished. See the Evidence constants.
	Evidence []string `json:"evidence,omitempty"`

	Risk string `json:"risk,omitempty"`

	// What the work leans on beyond this machine.
	LicensedSoftware  bool `json:"licensed_software,omitempty"`
	ExternalService   bool `json:"external_service,omitempty"`
	NeedsProfessional bool `json:"needs_professional,omitempty"`

	// Limits are what somebody working under this may not do or claim, a
	// line each. Escalation is who a person must bring in, and when.
	Limits     []string `json:"limits,omitempty"`
	Escalation string   `json:"escalation,omitempty"`

	// BuiltIn is set on what shipped; File on one read from the brain's
	// folder, which says where to edit it.
	BuiltIn bool   `json:"built_in,omitempty"`
	File    string `json:"file,omitempty"`
}

// Role is the agent a package would hire, before it has a name.
type Role struct {
	Title string   `json:"title"`
	For   string   `json:"for"`
	Uses  string   `json:"uses,omitempty"`
	Needs []string `json:"needs,omitempty"`
	Brief string   `json:"brief,omitempty"`

	// Tools is exactly what it may use. Never is what it may not, whatever
	// anything else later adds.
	Tools []string `json:"tools"`
	Never []string `json:"never,omitempty"`
}

// Source is somewhere authoritative. {version} in the address is replaced
// with the version actually installed, so the answer is about that engine.
type Source struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Versioned bool   `json:"versioned,omitempty"`
}

// Requirement is one recipe, the versions that will do, and why.
type Requirement struct {
	ID       string `json:"id"`
	Versions string `json:"versions,omitempty"`
	Why      string `json:"why"`
	Optional bool   `json:"optional,omitempty"`
}

// Integration is an MCP server from the approved list, and why.
type Integration struct {
	ID       string `json:"id"`
	Why      string `json:"why"`
	Optional bool   `json:"optional,omitempty"`
}

// Workspace is what a project under this package looks like.
type Workspace struct {
	// Template is the engine adapter's scaffold to start from.
	Template string `json:"template,omitempty"`

	// Outputs are folders, relative to the project, where builds may go.
	Outputs []string `json:"outputs,omitempty"`

	// Conventions are the house rules of this kind of project, a line each.
	Conventions []string `json:"conventions,omitempty"`
}

// The kinds of evidence a package may require.
const (
	EvidenceFiles     = "files"  // what changed, by path
	EvidenceCheck     = "check"  // a validation or headless check that passed
	EvidenceBuild     = "build"  // a build that succeeded
	EvidenceTest      = "test"   // tests that ran and passed
	EvidenceExport    = "export" // a packaged artifact, by path
	EvidenceSmoke     = "smoke"  // it started and ran without errors
	EvidenceShot      = "screenshot"
	EvidenceDocument  = "document"  // a written document, by path
	EvidenceSources   = "sources"   // where each claim came from
	EvidenceQuestions = "questions" // what to put to a qualified person
	EvidenceVersions  = "versions"  // which software, at which version
)

var evidenceKinds = map[string]bool{
	EvidenceFiles: true, EvidenceCheck: true, EvidenceBuild: true, EvidenceTest: true,
	EvidenceExport: true, EvidenceSmoke: true, EvidenceShot: true, EvidenceDocument: true,
	EvidenceSources: true, EvidenceQuestions: true, EvidenceVersions: true,
}

/*
 * Operates are the tools that act on the world beyond writing a file: they
 * run programs, drive the screen, reach a device or a person, or install.
 *
 * Advisory work may not carry any of them, and that is checked when a package
 * is read rather than trusted to whoever wrote it. A tool added at run time —
 * an MCP server's — is treated as operating unless it says otherwise, so the
 * rule cannot be walked round by naming something new.
 */
var Operates = map[string]bool{
	"run_command": true, "set_device": true, "click": true, "type_text": true,
	"scroll": true, "open_app": true, "send_email": true, "install_a_part": true,
	"install_a_model": true, "remove_a_part": true, "install_requirement": true,
	"godot_build": true, "game_check": true, "game_build": true, "decide_waiting": true,
	"start_project": true, "confirm_hire": true,
}

// operating reports whether a tool acts on the world beyond a file.
func operating(tool string) bool {
	return Operates[tool] || strings.HasPrefix(tool, "mcp_")
}

// Known is what a package's names are checked against. Any may be nil, and a
// nil one checks nothing — a package read before the registry exists is
// checked again once it does.
type Known struct {
	Tool        func(string) bool
	Recipe      func(string) bool
	Integration func(string) bool
	Job         func(string) bool
	Engine      func(string) bool

	// Oversight is whether a job keeps a qualified person in the loop — a
	// doctor's, an electrician's. A package serving one is advisory and names
	// that person, whatever the package itself says.
	Oversight func(string) bool
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

var validVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

/*
 * Check is every rule a package must meet, and every name it uses resolved.
 *
 * All the problems at once rather than the first, because somebody editing a
 * package by hand fixes them together.
 */
func Check(p Package, known Known) []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if !validID.MatchString(p.ID) {
		add("the id %q should be dotted lower-case words, like software.game.godot", p.ID)
	}

	if !validVersion.MatchString(p.Version) {
		add("the version %q should be three numbers, like 1.0.0", p.Version)
	}

	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Summary) == "" {
		add("a package needs a title and a summary")
	}

	switch p.Work {
	case Advisory, Project, Operating:
	default:
		add("work %q is not advisory, project or operating", p.Work)
	}

	if len(p.Jobs) == 0 {
		add("a package serves at least one job")
	}

	for _, job := range p.Jobs {
		if known.Job != nil && !known.Job(job) {
			add("the job %q is not in the catalogue", job)
		}
	}

	if strings.TrimSpace(p.Role.Title) == "" || strings.TrimSpace(p.Role.For) == "" {
		add("the role needs a title and a line saying what it is for")
	}

	/*
	 * A role always names its tools. An empty list means "everything"
	 * wherever agents are written, and a package must never be a way to hire
	 * somebody with no limit at all.
	 */
	if p.Role.Tools == nil {
		add("the role must list its tools — an empty list would mean all of them")
	}

	/*
	 * What it may use must exist; what it may not need not. Forbidding a tool
	 * this machine happens not to have — mail that was never set up — is
	 * still a limit worth keeping for the day it is.
	 */
	for _, tool := range p.Role.Tools {
		if known.Tool != nil && !known.Tool(tool) {
			add("there is no tool called %q", tool)
		}
	}

	for _, tool := range p.Role.Tools {
		if listed(p.Role.Never, tool) {
			add("%q is both allowed and forbidden", tool)
		}

		if tool == "decide_waiting" {
			add("decide_waiting is the owner's own and is never a role's")
		}

		if p.Work == Advisory && operating(tool) {
			add("advisory work may not carry %q, which acts on the world", tool)
		}
	}

	/*
	 * The floor under what a package may say about itself. It was the package
	 * that decided whether its work needed a professional, and an owner's
	 * copy of the electrician's package saying "operating, no professional"
	 * loaded without a murmur. Whether a job keeps a person in the loop is the
	 * catalogue's to say, and a package serving such a job is held to it.
	 */
	if known.Oversight != nil {
		for _, job := range p.Jobs {
			if known.Oversight(job) && (p.Work != Advisory || !p.NeedsProfessional) {
				add("%s keeps a qualified person in the loop, so work for it is advisory and needs that professional", job)

				break
			}
		}
	}

	if p.NeedsProfessional {
		if p.Work != Advisory {
			add("work that needs a qualified professional is advisory here — the professional does the rest")
		}

		if strings.TrimSpace(p.Escalation) == "" {
			add("work that needs a qualified professional must say who, and when")
		}

		if len(p.Limits) == 0 {
			add("work that needs a qualified professional must say what may not be claimed")
		}
	}

	if p.Risk != "" && risk.Parse(p.Risk) == risk.Low && p.Risk != string(risk.Low) {
		add("risk %q is not low, medium, high or critical", p.Risk)
	}

	for _, r := range p.Requires {
		if known.Recipe != nil && !known.Recipe(r.ID) {
			add("there is no recipe called %q", r.ID)
		}

		if _, err := provision.ParseRule(r.Versions); err != nil {
			add("%s: %v", r.ID, err)
		}

		if strings.TrimSpace(r.Why) == "" {
			add("%s is required without saying why", r.ID)
		}
	}

	for _, i := range p.Integrations {
		if known.Integration != nil && !known.Integration(i.ID) {
			add("there is no approved integration called %q", i.ID)
		}
	}

	if p.Engine != "" && known.Engine != nil && !known.Engine(p.Engine) {
		add("there is no engine adapter called %q", p.Engine)
	}

	for _, e := range p.Evidence {
		if !evidenceKinds[e] {
			add("%q is not a kind of evidence", e)
		}
	}

	for _, s := range p.Knowledge {
		if !strings.HasPrefix(s.URL, "https://") {
			add("the source %q should be an https address", s.Title)
		}
	}

	return problems
}

func listed(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}

	return false
}

//go:embed builtin/*.json
var shipped embed.FS

// BuiltIn is what ships, read from the files embedded in the program.
func BuiltIn() ([]Package, error) {
	entries, err := shipped.ReadDir("builtin")
	if err != nil {
		return nil, err
	}

	var out []Package

	for _, e := range entries {
		raw, err := shipped.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}

		p, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}

		p.BuiltIn = true
		out = append(out, p)
	}

	return out, nil
}

// Parse reads one package, refusing fields it does not know: a key
// misspelled in a package is a limit silently not applied.
func Parse(raw []byte) (Package, error) {
	var p Package

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&p); err != nil {
		return Package{}, err
	}

	for i := range p.Cues {
		p.Cues[i] = strings.ToLower(strings.TrimSpace(p.Cues[i]))
	}

	return p, nil
}

// Set is every package in use, the shipped ones and its owner's.
type Set struct {
	byID map[string]Package
	// Refused is what was read and could not be used, and why — shown, so a
	// file somebody wrote does not simply fail to appear.
	Refused map[string]string
}

/*
 * Load is what ships plus what is in the brain's packages folder, checked.
 *
 * A file naming a shipped package replaces it, the rule agents and templates
 * already follow. A package that fails its checks is left out whole and its
 * reasons kept, rather than half applied — half an electrician's limits is
 * worse than none, because it looks like all of them.
 */
func Load(root string, known Known) *Set {
	s := &Set{byID: map[string]Package{}, Refused: map[string]string{}}

	shipped, err := BuiltIn()
	if err != nil {
		s.Refused["(built in)"] = err.Error()
	}

	for _, p := range shipped {
		s.add(p, known)
	}

	if root == "" {
		return s
	}

	entries, _ := os.ReadDir(filepath.Join(root, FolderName))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(root, FolderName, e.Name())

		raw, err := os.ReadFile(path)
		if err != nil {
			s.Refused[e.Name()] = err.Error()

			continue
		}

		p, err := Parse(raw)
		if err != nil {
			s.Refused[e.Name()] = err.Error()

			continue
		}

		p.File = path

		if shipped, replaces := s.byID[p.ID]; replaces && shipped.File == "" {
			if why := weakens(shipped, p); why != "" {
				s.Refused[e.Name()] = "it would weaken the package that ships with this program: " + why

				continue
			}
		}

		s.add(p, known)
	}

	return s
}

/*
 * weakens is how an owner's copy of a shipped package would let its work do
 * more than the shipped one, or empty when it would not. A copy may narrow —
 * fewer tools, more limits — and add what only reads; it may not open up.
 */
func weakens(shipped, copy Package) string {
	var more []string

	if shipped.NeedsProfessional && !copy.NeedsProfessional {
		more = append(more, "it no longer needs a qualified professional")
	}

	if shipped.Work == Advisory && copy.Work != Advisory {
		more = append(more, "its work is no longer advisory")
	}

	if shipped.Escalation != "" && strings.TrimSpace(copy.Escalation) == "" {
		more = append(more, "it no longer says who takes over")
	}

	for _, tool := range copy.Role.Tools {
		if operating(tool) && !listed(shipped.Role.Tools, tool) {
			more = append(more, "it may use "+tool)
		}
	}

	return strings.Join(more, "; ")
}

func (s *Set) add(p Package, known Known) {
	if problems := Check(p, known); len(problems) > 0 {
		said := make([]string, 0, len(problems))

		for _, problem := range problems {
			said = append(said, problem.Error())
		}

		name := p.ID
		if p.File != "" {
			name = filepath.Base(p.File)
		}

		s.Refused[name] = strings.Join(said, "; ")

		return
	}

	s.byID[p.ID] = p
}

// Get is one package.
func (s *Set) Get(id string) (Package, bool) {
	if s == nil {
		return Package{}, false
	}

	p, ok := s.byID[id]

	return p, ok
}

// All is every package, by id.
func (s *Set) All() []Package {
	if s == nil {
		return nil
	}

	out := make([]Package, 0, len(s.byID))

	for _, p := range s.byID {
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

/*
 * For is the packages that fit a job and a request, best first.
 *
 * A package serving the job counts; the request's words then decide between
 * them — a game developer asked for "a Tetris game in three.js" gets the
 * three.js package ahead of Godot's. With no job, the words alone decide, and
 * a request that names nothing any package answers to gets none: guessing an
 * engine for somebody is exactly the kind of decision that should be asked.
 */
func (s *Set) For(job, request string) []Package {
	words := wordsOf(request)

	type scored struct {
		p     Package
		score int
	}

	var found []scored

	for _, p := range s.All() {
		score := 0

		if job != "" && listed(p.Jobs, job) {
			score += 2
		}

		score += 3 * cueHits(p, words)

		if score > 0 {
			found = append(found, scored{p, score})
		}
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })

	out := make([]Package, 0, len(found))

	for _, f := range found {
		out = append(out, f.p)
	}

	return out
}

// Named is the packages a request names outright by a cue, best first. No
// job: this is "which engine did they say", not "which suits the job".
func (s *Set) Named(request string) []Package {
	words := wordsOf(request)

	var out []Package

	for _, p := range s.All() {
		if cueHits(p, words) > 0 {
			out = append(out, p)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return cueHits(out[i], words) > cueHits(out[j], words) })

	return out
}

func cueHits(p Package, words map[string]bool) int {
	hits := 0

	for _, cue := range p.Cues {
		if strings.Contains(cue, " ") {
			all := true

			for _, part := range strings.Fields(cue) {
				if !words[part] {
					all = false

					break
				}
			}

			if all {
				hits++
			}

			continue
		}

		if words[cue] {
			hits++
		}
	}

	return hits
}

func wordsOf(text string) map[string]bool {
	out := map[string]bool{}

	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '#' && r != '+'
	}) {
		out[strings.Trim(w, ".")] = true
	}

	return out
}

// Mandatory is the requirements that are not optional.
func (p Package) Mandatory() []Requirement {
	var out []Requirement

	for _, r := range p.Requires {
		if !r.Optional {
			out = append(out, r)
		}
	}

	return out
}

/*
 * Standing is what anybody working under this package is told, whatever the
 * step: its know-how, its limits, and who takes over where it must stop.
 *
 * Said on every step rather than once, because a model does not remember an
 * instruction from three steps ago — and the limit that matters most is the
 * one on the step where it is tempted.
 */
func (p Package) Standing() string {
	var b strings.Builder

	for _, s := range p.Skills {
		b.WriteString("- " + s + "\n")
	}

	if len(p.Limits) > 0 {
		b.WriteString("You must not:\n")

		for _, l := range p.Limits {
			b.WriteString("- " + l + "\n")
		}
	}

	if p.NeedsProfessional {
		b.WriteString("You are not a licensed professional and must never say or imply that " +
			"anything is certified, approved, compliant or safe. ")
	}

	if p.Escalation != "" {
		b.WriteString("Where a qualified person is needed: " + p.Escalation + "\n")
	}

	return strings.TrimSpace(b.String())
}

// Needs is the work's reliance on things beyond this machine, in words, for
// a proposal. Empty when it relies on none.
func (p Package) Needs() []string {
	var out []string

	if p.LicensedSoftware {
		out = append(out, "licensed software that its owner installs and accepts the terms of")
	}

	if p.ExternalService {
		out = append(out, "a service outside this machine, which privacy has to allow")
	}

	if p.NeedsProfessional {
		out = append(out, "a qualified professional for anything that is signed, inspected or built")
	}

	return out
}
