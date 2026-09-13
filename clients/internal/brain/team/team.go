/*
 * Package team is who does which part of a job.
 *
 * Every model is good at something different, and the difference is not
 * subtle: on this machine the coding model and the fastest model are minutes
 * apart per round, and the one that writes best prose is neither. A job with
 * four steps in it should not be four turns of the same compromise.
 *
 * So a task's steps are handed to named agents — a developer, a researcher, a
 * writer — each with its own model, its own tools and its own standing
 * instructions. The conductor is the team leader: it plans the work, decides
 * who does what, and checks it afterwards.
 *
 * Two things stay singular however many agents there are, and they are the
 * two that matter. Every model call goes through Router.Provider, which is the
 * one place privacy is enforced. And every action goes through the same
 * permits book, from the same loop. An agent's tool list can only ever be
 * narrower than what the assistant may already do — it removes, it never adds
 * — so a new agent can never be a new way round the gate.
 */
package team

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

// FolderName is where a roster somebody has changed lives, inside the brain's
// own folder, so it travels with the brain and can be edited by hand.
const FolderName = "agents"

var validName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)

// Agent is one member of the team.
type Agent struct {
	// Name is what the planner writes in a step and what the interface shows.
	Name string `json:"name"`

	// Title is the name a person reads. "Developer", not "developer".
	Title string `json:"title"`

	/*
	 * For is what this one is good at, in one line.
	 *
	 * It goes to the planner, which picks who does each step, and its words
	 * are also the fallback when the planner picks nobody. So it should read
	 * like a job description rather than a list of tools: the planner is
	 * choosing between people, not between capabilities.
	 */
	For string `json:"for"`

	/*
	 * Uses is the size of model this one wants, by role rather than by name.
	 *
	 * By role because the names differ on every machine. A machine with one
	 * model installed collapses all of these to it and nothing breaks, which
	 * is the same fallback the model roles already have.
	 */
	Uses string `json:"uses"`

	// Provider pins this agent to one service. Empty means the task's own,
	// which is the ordinary case. Whatever is named here is still asked for
	// through the router, so privacy is enforced exactly as before.
	Provider string `json:"provider,omitempty"`

	/*
	 * Tools is the only tools this one may use. Empty means all of them.
	 *
	 * A narrowing, never a widening. Two reasons, and both are real: how well
	 * a model chooses among tools falls off sharply with its size, so a
	 * researcher offered thirty-one options chooses worse than one offered
	 * five — and a writer that cannot reach run_command cannot be talked into
	 * running a command, whatever it is asked.
	 */
	Tools []string `json:"tools,omitempty"`

	// Brief is standing instructions for this one, added to the step's own.
	Brief string `json:"brief,omitempty"`

	/*
	 * Never is tools this one may not use, whatever else it may.
	 *
	 * Separate from Tools because it answers a different question, and
	 * survives a different kind of change. Tools is "what is this one for",
	 * and adding a capability widens it. Never is "what is not this one's to
	 * do", and nothing widens it — which is why a seat that must not send
	 * email says so here rather than by carefully leaving one name out of a
	 * list somebody will later add to.
	 */
	Never []string `json:"never,omitempty"`

	/*
	 * Position is the seat this one holds in the organisation, and Job is
	 * what that seat is — an occupation id like software.backend_engineer.
	 *
	 * Both may be empty, and an agent with neither is exactly what every
	 * agent was before there was an organisation: a name, a description and
	 * a tool list. That is why a roster file written last month still loads.
	 *
	 * Job is normally read from the position rather than written here. It is
	 * settable directly for the case the organisation has not caught up with
	 * — somebody hired to do a thing before there is a seat for it.
	 */
	Position string `json:"position,omitempty"`
	Job      string `json:"job,omitempty"`

	Seniority string `json:"seniority,omitempty"`

	/*
	 * Can is what this one is good at beyond whatever its job implies.
	 *
	 * The distinction matters: the job is shared by everybody who holds it,
	 * and this is the part that is true of this one in particular. A backend
	 * engineer who also knows the accounts is not a different job.
	 */
	Can []string `json:"can,omitempty"`

	/*
	 * Persona is how this one comes across, as against what it does.
	 *
	 * Kept apart from Brief because they are read by different things and at
	 * different times: the brief is instructions for the work, and this is
	 * manner. Two agents doing the same job differently is most of what makes
	 * a roster feel like people rather than one program wearing hats.
	 */
	Persona string `json:"persona,omitempty"`

	/*
	 * Prefers is model names, in order, overriding the size role.
	 *
	 * For the case somebody knows exactly which model they want this one to
	 * think with — and for a hosted service, where a size role means nothing
	 * because the roles resolve to what is installed on this machine.
	 */
	Prefers []string `json:"prefers,omitempty"`

	// Needs is what this one's work requires of a model, as against what
	// would be preferred. See policy.go.
	Needs Needs `json:"needs,omitempty"`

	// State is whether this one works here at the moment.
	State string `json:"state,omitempty"`

	// BuiltIn marks one that shipped with the program rather than being
	// written by its owner. Shown, so the two can be told apart.
	BuiltIn bool `json:"built_in,omitempty"`
}

/*
 * Whether an agent works here at the moment.
 *
 * Suspended and retired are kept rather than deleted, because "who used to do
 * this and what did they get done" is a real question and the work they did
 * is still recorded against their name. A file is still the way to remove one
 * for good.
 *
 * Temporary is the one that earns its place: an agent hired for one job and
 * dissolved when it ends. Without it, a long task that needed a specialist
 * would leave the roster a little more cluttered every time — and a roster
 * that grows on its own is one nobody trusts.
 */
const (
	Active    = "active"
	Temporary = "temporary"
	Suspended = "suspended"
	Retired   = "retired"
)

// Working reports whether this one may be given work. Written as a question
// about the agent rather than a comparison at each call site, because there
// are about to be several.
func (a Agent) Working() bool {
	return a.State == "" || a.State == Active || a.State == Temporary
}

// The model roles an agent may ask for.
const (
	UsesWork   = "work"
	UsesQuick  = "quick"
	UsesBest   = "best"
	UsesReason = "reason"
	UsesTalk   = "talk"
)

/*
 * BuiltIn is the roster the program ships with.
 *
 * Five, and every one of them has a genuinely different tool list and a
 * genuinely different model. That discipline matters more than the length of
 * the list: two agents that differ only in name make the planner's job harder
 * for nothing, and a roster full of them would route badly and look busy.
 *
 * Every one of them now holds a seat in the organisation, which is where its
 * job — and so what it is expected to know — comes from. The tool lists stay
 * written out even so: they were arrived at by watching small models choose
 * badly, and a list derived from a job definition is a different list. What
 * the seats add is who these people are, not what they may touch.
 *
 * The designer is here now because the tools are: it can make a picture and
 * put pictures into a film, which is a job rather than a label. There is still
 * no marketer and no seo, and that is the same discipline rather than an
 * oversight — both would be the researcher with a different name and one extra
 * tool, and two agents that differ only in name make the planner's job harder
 * for nothing. Either is one file away for somebody who wants it.
 */
func BuiltIn() []Agent {
	return []Agent{
		{
			Name: "assistant", Title: "Assistant", BuiltIn: true,
			Position: "assistant",
			For: "anything that does not clearly belong to somebody else, and anything " +
				"touching this machine, its settings, its memory or its mail",
			Uses: UsesWork,
		},
		{
			Name: "developer", Title: "Developer", BuiltIn: true,
			Position: "lead_developer",
			For: "reading, writing and changing code, running commands, and working " +
				"out why something does not build or run",
			Uses: UsesWork,
			Tools: []string{
				"read_file", "write_file", "edit_file", "search_files",
				"list_directory", "run_command", "read_document", "list_drives",
			},
			Brief: "Look at the code before changing it. Change the least that will do. " +
				"If a command would change something outside the project, say so first.",
		},
		{
			Name: "game_developer", Title: "Game developer", BuiltIn: true,
			Position: "game_developer",
			For:      "Godot projects: scenes, GDScript, exporting, and checking a build runs",
			Uses:     UsesWork,
			Tools: []string{
				"godot_status", "godot_docs", "godot_build",
				"read_file", "write_file", "edit_file", "search_files", "list_directory",
			},
			Brief: "Look a class up in the reference rather than remembering it. " +
				"Godot's API changes between versions and a method that does not exist " +
				"fails at run time, not at build time.",
		},
		{
			Name: "researcher", Title: "Researcher", BuiltIn: true,
			Position: "researcher",
			For: "finding something out — from the web, from documents, or from what is " +
				"already on this machine",
			Uses: UsesQuick,
			Tools: []string{
				// read_a_page as well as fetch_url: a great many sites now
				// send an empty shell and a script tag, and the researcher is
				// exactly who needs the one that runs them.
				"web_search", "fetch_url", "read_a_page", "read_document", "read_file",
				"search_files", "list_directory", "what_you_know",
			},
			Brief: "Say where each thing came from. A fact with no source is a guess " +
				"that has been written down neatly. Use fetch_url first and read_a_page " +
				"only when it comes back empty — the second is a whole browser and costs " +
				"most of a minute.",
		},
		{
			Name: "designer", Title: "Designer", BuiltIn: true,
			Position: "designer",
			For: "anything to be looked at rather than read — a picture, a logo, an " +
				"illustration, a film made out of pictures, or how this program looks",
			Uses: UsesBest,
			Tools: []string{
				"make_a_picture", "make_a_video", "set_appearance",
				"read_file", "read_document", "list_directory",
			},
			Brief: "Describe what the picture should be of in full — the subject, the " +
				"style, the colours, what is around it. A short description makes a " +
				"generic picture, and making it again costs minutes.",
		},
		{
			Name: "writer", Title: "Writer", BuiltIn: true,
			Position: "writer",
			For: "writing something to be read by a person — a summary, a document, a " +
				"letter, notes",
			Uses: UsesBest,
			Tools: []string{
				// write_file as well as write_document: notes and a draft in
				// markdown are writing, and a writer that could only produce a
				// PDF would reach for a tool it does not have and stop.
				"write_document", "write_file", "read_document", "read_file",
				"search_files", "list_directory",
			},
			Brief: "Write the thing itself, not a description of it. Plain words, and " +
				"no more of them than the point needs.",
		},
	}
}

/*
 * Roster is the team as it stands: what shipped, plus whatever its owner has
 * changed or added.
 *
 * A file naming an existing agent replaces it, so somebody who wants the
 * developer to use a different model edits one line rather than rebuilding the
 * roster. A file with an unknown name adds one.
 */
func Roster(root string) []Agent {
	byName := map[string]Agent{}
	order := []string{}

	for _, a := range BuiltIn() {
		byName[a.Name] = a
		order = append(order, a.Name)
	}

	for _, a := range fromFolder(root) {
		if _, known := byName[a.Name]; !known {
			order = append(order, a.Name)
		}

		a.BuiltIn = false
		byName[a.Name] = a
	}

	out := make([]Agent, 0, len(order))

	for _, name := range order {
		out = append(out, byName[name])
	}

	return out
}

// Folder is where a changed roster lives.
func Folder(root string) string { return filepath.Join(root, FolderName) }

/*
 * fromFolder reads agents somebody wrote.
 *
 * A file that will not parse is skipped rather than failing the lot, for the
 * same reason skills are: these are hand-editable by design, so one of them
 * being mid-edit is an ordinary Tuesday and not a reason for the team to
 * disappear.
 */
func fromFolder(root string) []Agent {
	entries, err := os.ReadDir(Folder(root))
	if err != nil {
		return nil
	}

	if agents, ok := remembered(root, entries); ok {
		return agents
	}

	var out []Agent

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(Folder(root), entry.Name()))
		if err != nil {
			continue
		}

		agent := parse(string(raw), strings.TrimSuffix(entry.Name(), ".md"))

		if !validName.MatchString(agent.Name) || agent.For == "" {
			continue
		}

		out = append(out, agent)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	keep(root, entries, out)

	return out
}

/*
 * Reading the folder again, or not.
 *
 * The roster is asked for on every step of every task and again for every
 * plan, and each ask was a directory listing plus a read and a parse of every
 * file in it. At six that is invisible. At three hundred it is a syscall
 * storm running beside model inference on the same processor, which is the
 * one resource this machine has none of to spare.
 *
 * Two cheap guards rather than one clever one. Within a second the answer is
 * reused outright — no task changes its roster mid-step, and a second is
 * faster than anybody can save a file and ask a question. After that the
 * listing is fingerprinted, and only a change in it costs a re-read.
 *
 * The fingerprint is names, sizes and times from the listing that was going
 * to happen anyway. It cannot see an edit that changed no byte and kept the
 * same timestamp, which is a trade worth naming: that edit is a file touched
 * by a program rather than a person, and the alternative is reading every
 * file to find out whether it was worth reading.
 */
var roster struct {
	sync.Mutex

	root    string
	mark    string
	checked time.Time
	agents  []Agent
}

// HowLongARosterKeeps is how long the last answer is reused without even
// looking. Short enough that saving a file and asking a question feels
// immediate, long enough that a task's steps do not each pay for it.
const HowLongARosterKeeps = time.Second

func remembered(root string, entries []os.DirEntry) ([]Agent, bool) {
	roster.Lock()
	defer roster.Unlock()

	if roster.root != root || roster.agents == nil {
		return nil, false
	}

	if time.Since(roster.checked) < HowLongARosterKeeps {
		return roster.agents, true
	}

	if fingerprint(entries) != roster.mark {
		return nil, false
	}

	roster.checked = time.Now()

	return roster.agents, true
}

func keep(root string, entries []os.DirEntry, agents []Agent) {
	roster.Lock()
	defer roster.Unlock()

	roster.root = root
	roster.mark = fingerprint(entries)
	roster.checked = time.Now()
	roster.agents = agents
}

// Forget throws the remembered roster away, for a caller that has just
// changed one and wants the change to be the next thing anybody sees.
func Forget() {
	roster.Lock()
	defer roster.Unlock()

	roster.agents = nil
}

func fingerprint(entries []os.DirEntry) string {
	var b strings.Builder

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		b.WriteString(entry.Name())

		info, err := entry.Info()
		if err != nil {
			// A file that vanished between the listing and the question is a
			// change by definition, so say so rather than matching.
			b.WriteString("?")

			continue
		}

		fmt.Fprintf(&b, ":%d:%d\n", info.Size(), info.ModTime().UnixNano())
	}

	return b.String()
}

func parse(raw, filename string) Agent {
	agent := Agent{Name: filename, Uses: UsesWork}

	header, brief, found := strings.Cut(raw, "\n---")

	if !found {
		header = raw
	}

	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)

		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			if value != "" {
				agent.Name = value
			}
		case "title":
			agent.Title = value
		case "for":
			agent.For = value
		case "uses":
			agent.Uses = strings.ToLower(value)
		case "provider":
			agent.Provider = value
		case "tools":
			agent.Tools = splitList(value)
		case "never":
			agent.Never = splitList(value)
		case "position":
			agent.Position = strings.ToLower(value)
		case "job":
			agent.Job = strings.ToLower(value)
		case "seniority":
			agent.Seniority = strings.ToLower(value)
		case "can":
			agent.Can = splitList(value)
		case "persona":
			agent.Persona = value
		case "state":
			agent.State = strings.ToLower(value)
		case "prefers", "models":
			agent.Prefers = splitList(value)
		case "needs":
			agent.Needs = readNeeds(value)
		}
	}

	agent.Brief = strings.TrimSpace(strings.TrimLeft(brief, "-\n"))

	if agent.Title == "" {
		agent.Title = strings.ToUpper(agent.Name[:1]) + strings.ReplaceAll(agent.Name[1:], "_", " ")
	}

	return agent
}

func splitList(value string) []string {
	var out []string

	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

/*
 * Save writes an agent to the folder, so a change made in the program and a
 * change made in an editor are the same change.
 *
 * The whole agent is written, not a patch, because the file is the record: a
 * roster that lived half in a config file and half in the code that shipped
 * would be a roster nobody could read the truth of.
 */
func Save(root string, agent Agent) error {
	if !validName.MatchString(agent.Name) {
		return fmt.Errorf("an agent's name must be lower-case letters, digits or underscores")
	}

	if strings.TrimSpace(agent.For) == "" {
		return fmt.Errorf("an agent with no description cannot be chosen for anything")
	}

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		return fmt.Errorf("making the agents folder: %w", err)
	}

	var b strings.Builder

	b.WriteString("name: " + agent.Name + "\n")
	b.WriteString("title: " + agent.Title + "\n")
	b.WriteString("for: " + strings.TrimSpace(agent.For) + "\n")
	b.WriteString("uses: " + orWork(agent.Uses) + "\n")

	if agent.Provider != "" {
		b.WriteString("provider: " + agent.Provider + "\n")
	}

	for _, line := range [][2]string{
		{"position", agent.Position},
		{"job", agent.Job},
		{"seniority", agent.Seniority},
		{"state", agent.State},
		{"can", strings.Join(agent.Can, ", ")},
		{"prefers", strings.Join(agent.Prefers, ", ")},
		{"needs", agent.Needs.Written()},
		{"tools", strings.Join(agent.Tools, ", ")},
		{"never", strings.Join(agent.Never, ", ")},
		{"persona", strings.TrimSpace(agent.Persona)},
	} {
		if line[1] != "" {
			b.WriteString(line[0] + ": " + line[1] + "\n")
		}
	}

	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(agent.Brief))
	b.WriteString("\n")

	// Written whole and moved into place, like everything else in this folder.
	path := filepath.Join(Folder(root), agent.Name+".md")

	if err := os.WriteFile(path+".new", []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing the agent: %w", err)
	}

	return os.Rename(path+".new", path)
}

/*
 * Reset puts a built-in agent back to what it shipped as.
 *
 * By deleting the file rather than rewriting it, so the answer to "what is the
 * developer now" is always one of two things — what shipped, or what is in the
 * folder — and never a third that merely resembles the first.
 */
func Reset(root, name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("that is not an agent's name")
	}

	err := os.Remove(filepath.Join(Folder(root), name+".md"))

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func orWork(uses string) string {
	switch uses {
	case UsesQuick, UsesBest, UsesReason, UsesTalk:
		return uses
	default:
		return UsesWork
	}
}

// Find returns the named agent, or false.
func Find(roster []Agent, name string) (Agent, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, a := range roster {
		if a.Name == name {
			return a, true
		}
	}

	return Agent{}, false
}

/*
 * For decides who does a step when the planner did not say, or named somebody
 * who does not exist.
 *
 * The same trust-but-validate shape as everything else the planner writes: its
 * choice is taken when it names a real person, and otherwise this works it out
 * from the step. A model inventing a "senior_architect" should cost nothing
 * more than the step being done by the generalist.
 */
func For(roster []Agent, named, kind, instruction string) Agent {
	if agent, ok := Find(roster, named); ok {
		return agent
	}

	text := strings.ToLower(instruction)

	// A step that writes something for a person to read goes to the writer,
	// whatever else the words look like.
	if kind == store.StepWrite {
		if agent, ok := Find(roster, "writer"); ok && !mentionsCode(text) {
			return agent
		}
	}

	best, bestScore := fallback(roster), 0

	for _, agent := range roster {
		if agent.Name == "assistant" {
			continue
		}

		if score := overlap(text, agent.For) + prior(kind, agent.Name); score > bestScore {
			best, bestScore = agent, score
		}
	}

	return best
}

/*
 * prior is what the shape of the step says, before its subject is read.
 *
 * The kind is what a step does; the job description is what it is about. When
 * they disagree the doing wins, and the reason is not a preference — it is
 * that the wrong choice cannot be carried out at all. "Search the web for what
 * changed in Godot" mentions Godot, so it scored to the game developer, who
 * has no web_search and no fetch_url and would have spent the step reaching
 * for tools it does not have.
 */
func prior(kind, name string) int {
	if kind == store.StepLook && name == "researcher" {
		return 2
	}

	return 0
}

func fallback(roster []Agent) Agent {
	if agent, ok := Find(roster, "assistant"); ok {
		return agent
	}

	if len(roster) > 0 {
		return roster[0]
	}

	return Agent{Name: "assistant", Title: "Assistant", Uses: UsesWork}
}

/*
 * overlap counts the distinctive words a step and a job description share.
 *
 * Words, and near enough words. It used to be a plain substring test, which
 * meant "write up what it does" found nothing in a writer whose description
 * says "writing" — the right person, missed on a suffix. A shortlist that
 * drops the obvious candidate over the difference between write and writing
 * is worse than no shortlist.
 *
 * So the comparison is on the first four letters, which is as much stemming as
 * this needs and all of it that can be explained in a sentence. It matches a
 * little too widely — commands and communication share four letters — and that
 * is the right way to be wrong here: the cost is one extra name on a list a
 * model then chooses from, where the other way round the cost is the right
 * person never being offered.
 */
func overlap(text, description string) int {
	said := map[string]bool{}

	for _, word := range strings.FieldsFunc(text, notALetter) {
		if len(word) >= 4 {
			said[word[:4]] = true
		}
	}

	score := 0

	for _, word := range strings.FieldsFunc(strings.ToLower(description), notALetter) {
		if len(word) < 4 || tooCommon[word] {
			continue
		}

		if said[word[:4]] || strings.Contains(text, word) {
			score++
		}
	}

	return score
}

func mentionsCode(text string) bool {
	for _, word := range []string{"code", "script", "function", "build", "compile", "test"} {
		if strings.Contains(text, word) {
			return true
		}
	}

	return false
}

func notALetter(r rune) bool {
	return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 128
}

var tooCommon = map[string]bool{
	"about": true, "anything": true, "been": true, "belong": true, "does": true,
	"else": true, "from": true, "into": true, "much": true, "not": true,
	"else's": true, "other": true, "read": true, "somebody": true, "something": true,
	"that": true, "their": true, "them": true, "they": true, "thing": true,
	"this": true, "what": true, "when": true, "which": true, "with": true,
	"working": true, "work": true, "already": true, "person": true,
}

/*
 * Model is which model this agent wants, given what is installed.
 *
 * A machine with one model installed answers every role with the same name,
 * which is correct: the roster is about who does what, and on a small machine
 * that is a question about tools and instructions rather than about models.
 */
func (a Agent) Model(sizes llm.Sizes) string {
	switch a.Uses {
	case UsesQuick:
		return firstOf(sizes.Quick, sizes.Work)
	case UsesBest:
		return firstOf(sizes.Best, sizes.Work)
	case UsesReason:
		return firstOf(sizes.Reason, sizes.Work)
	case UsesTalk:
		return firstOf(sizes.Talk, sizes.Work)
	default:
		return sizes.Work
	}
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}

/*
 * Allows reports whether this one may use a tool.
 *
 * The rule itself lives in the tools package, which is also where the loop
 * gets it, so this is a way of asking rather than a second answer. An empty
 * list means all of them, which is what the generalist has.
 */
func (a Agent) Allows(tool string) bool {
	return tools.Allowed(a.Tools, tool) && !tools.Listed(a.Never, tool)
}

// Describe is the roster as the planner is shown it: one line each, so it is
// choosing between people rather than reading a configuration file.
func Describe(roster []Agent) string {
	var b strings.Builder

	for _, a := range roster {
		fmt.Fprintf(&b, "- %s: %s\n", a.Name, a.For)
	}

	return b.String()
}
