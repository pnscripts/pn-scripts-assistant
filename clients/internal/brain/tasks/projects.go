package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * Work on a project: confined to it, told about it, and finished only on
 * evidence.
 *
 * A task that belongs to a project reads the project's own settings at every
 * step — which folders are its, where builds go, which integrations it may
 * use — so an edit to them takes effect on the next step rather than the next
 * task. Every step is told what the project is and what was learned working on
 * it, and the know-how and limits of the packages it works under.
 *
 * And it is finished when the evidence its packages ask for exists, not when
 * its steps say they are done: a game is finished when there is a check that
 * passed and a run that did not fail, recorded as they happened, and a plan
 * for an electrician when the questions for the electrician are written down.
 */

// confine is a project task's folders and integrations, for one step.
func (c *Conductor) confine(task *store.Task, brief *agent.Brief) error {
	if task.Project == "" {
		return nil
	}

	cfg, err := workspace.Load(task.Project)
	if err != nil {
		return fmt.Errorf("the project's settings could not be read, so no work is done on it: %w", err)
	}

	/*
	 * Settings that widened while the task ran stop it.
	 *
	 * Its owner may well have edited them — they are a file in the project,
	 * meant to be edited — but so could the work, by any route the checks on
	 * a single call did not foresee. A setting that narrows what the task may
	 * do is simply obeyed from the next step. One that widens it — another
	 * folder, another integration, another command — is not something a task
	 * started under the old settings was approved for.
	 */
	if at, ok := c.settingsAtStart(task); ok {
		if why := workspace.Widened(at, cfg); why != "" {
			return fmt.Errorf("the project's settings were widened while this task was running (%s); "+
				"start it again to work under them", why)
		}
	} else if c.DB != nil {
		raw, _ := json.Marshal(cfg)

		c.DB.Record(store.Evidence{TaskID: task.ID, Kind: store.EvidenceSettings,
			Subject: workspace.Path(task.Project), Detail: string(raw), OK: true})
	}

	brief.Within = workspace.ScopeOf(task.Project, cfg)

	allowed := append([]string{}, cfg.Integrations...)
	brief.Integrations = &allowed

	return nil
}

// settingsAtStart is the project's settings as the task first read them.
func (c *Conductor) settingsAtStart(task *store.Task) (workspace.Config, bool) {
	if c.DB == nil {
		return workspace.Config{}, false
	}

	rows, err := c.DB.EvidenceFor(task.ID)
	if err != nil {
		return workspace.Config{}, false
	}

	for _, e := range rows {
		if e.Kind != store.EvidenceSettings {
			continue
		}

		var cfg workspace.Config

		if json.Unmarshal([]byte(e.Detail), &cfg) == nil {
			return cfg, true
		}
	}

	return workspace.Config{}, false
}

// packagesOf is the packages a task works under.
func (c *Conductor) packagesOf(task *store.Task) []capability.Package {
	if c.Packages == nil || task.Packages == "" {
		return nil
	}

	set := c.Packages()

	var out []capability.Package

	for _, id := range strings.Split(task.Packages, ",") {
		if p, ok := set.Get(strings.TrimSpace(id)); ok {
			out = append(out, p)
		}
	}

	return out
}

// aboutTheProject is what every step of a project task is told.
func (c *Conductor) aboutTheProject(task *store.Task) string {
	var b strings.Builder

	if task.Project != "" {
		cfg, err := workspace.Load(task.Project)
		if err == nil {
			fmt.Fprintf(&b, "The project is at %s: a %s project", task.Project, cfg.Kind)

			if cfg.Engine != "" {
				fmt.Fprintf(&b, " made with %s %s", cfg.Engine, cfg.EngineVersion)
			}

			b.WriteString(". Work only inside it")

			if len(cfg.Outputs) > 0 {
				b.WriteString("; builds go only to " + strings.Join(cfg.Outputs, ", "))
			}

			b.WriteString(". Give whole paths.\n")

			if len(cfg.Conventions) > 0 {
				b.WriteString("Its conventions: " + strings.Join(cfg.Conventions, "; ") + ".\n")
			}

			for purpose, argv := range cfg.Commands {
				fmt.Fprintf(&b, "Its %s command: %s (run it with dir set to the project).\n", purpose, strings.Join(argv, " "))
			}
		}

		if skills := workspace.Skills(task.Project, 3); len(skills) > 0 {
			b.WriteString("This project's own know-how:\n- " + strings.Join(skills, "\n- ") + "\n")
		}

		if learned := workspace.Memory(task.Project, 5); len(learned) > 0 {
			b.WriteString("What was learned working on it before:\n- " + strings.Join(learned, "\n- ") + "\n")
		}
	}

	for _, p := range c.packagesOf(task) {
		if standing := p.Standing(); standing != "" {
			b.WriteString("\nWorking under " + p.Title + ":\n" + standing + "\n")
		}
	}

	return strings.TrimSpace(b.String())
}

// recordEvidence keeps what a turn's calls can prove they did.
func (c *Conductor) recordEvidence(task *store.Task, step *store.TaskStep, res agent.Result) {
	for _, s := range res.Steps {
		for _, e := range s.Evidence {
			e.TaskID, e.StepID = task.ID, step.ID

			if err := c.DB.Record(e); err != nil {
				c.Log.Warn("could not keep evidence", "task", task.ID, "error", err)
			}
		}
	}
}

// Proved keeps the evidence of an action that ran once its owner approved it,
// against the step that was waiting for it.
func (c *Conductor) Proved(invocation store.Invocation, evidence []store.Evidence) {
	step, err := c.DB.StepWaitingOn(invocation.ID)
	if err != nil || step == nil {
		return
	}

	rows := append([]store.Evidence{{Kind: store.EvidenceApproval, Subject: invocation.Summary,
		Detail: "approved by its owner", OK: true}}, evidence...)

	for _, e := range rows {
		e.TaskID, e.StepID = step.TaskID, step.ID

		c.DB.Record(e)
	}
}

// documentExtensions are files that count as a written document.
var documentExtensions = map[string]bool{".md": true, ".txt": true, ".docx": true, ".pdf": true, ".odt": true, ".rtf": true}

/*
 * missingEvidence is what the task's packages ask for and nothing recorded
 * shows, in words.
 *
 * Only the latest word on each thing counts. A check that passed, then an
 * edit, then a check that failed, is a failing project — an audit found it
 * judged finished on the strength of the first. And a check proves the files
 * as they were when it ran, so one from before the last change to them proves
 * nothing about the files there are now.
 */
func (c *Conductor) missingEvidence(task *store.Task) []string {
	wanted := map[string]bool{}
	order := []string{}

	for _, p := range c.packagesOf(task) {
		for _, e := range p.Evidence {
			if !wanted[e] {
				wanted[e] = true
				order = append(order, e)
			}
		}
	}

	if len(order) == 0 {
		return nil
	}

	rows, err := c.evidenceOf(task)
	if err != nil {
		return []string{"its evidence could not be read"}
	}

	var missing []string

	for _, kind := range order {
		if why := standing(kind, rows); why != "" {
			missing = append(missing, why)
		}
	}

	return missing
}

var evidenceWords = map[string]string{
	capability.EvidenceFiles: "files written", capability.EvidenceCheck: "a check that passed",
	capability.EvidenceBuild: "a build that succeeded", capability.EvidenceTest: "tests that passed",
	capability.EvidenceExport: "an export", capability.EvidenceSmoke: "a run that did not fail",
	capability.EvidenceShot: "a picture of it running", capability.EvidenceDocument: "a written document",
	capability.EvidenceSources: "sources for what it says", capability.EvidenceQuestions: "questions for a qualified person",
	capability.EvidenceVersions: "the versions it was made with",
}

// verifies is evidence about the files as they were: stale once they change.
var verifies = map[string]bool{
	capability.EvidenceCheck: true, capability.EvidenceBuild: true, capability.EvidenceTest: true,
	capability.EvidenceExport: true, capability.EvidenceSmoke: true, capability.EvidenceShot: true,
}

/*
 * standing is why one kind of evidence does not stand, in words, or empty
 * when it does: there is none, the latest one failed, or the files changed
 * after it.
 */
func standing(kind string, rows []store.Evidence) string {
	latest, lastChange := -1, -1

	for i, e := range rows {
		if (e.Kind == store.EvidenceFile || e.Kind == store.EvidenceDocument) && e.OK {
			lastChange = i
		}

		if matches(kind, e) {
			latest = i
		}
	}

	words := evidenceWords[kind]

	switch {
	case latest < 0:
		return words
	case !rows[latest].OK:
		return words + " (the latest one failed: " + firstLine(rows[latest].Detail) + ")"
	case verifies[kind] && lastChange > latest:
		return words + " (the files changed after the latest one)"
	}

	return ""
}

// matches is whether a row is evidence of one kind, passed or not.
func matches(kind string, e store.Evidence) bool {
	switch kind {
	case capability.EvidenceFiles:
		return e.Kind == store.EvidenceFile
	case capability.EvidenceCheck:
		return e.Kind == store.EvidenceCheck
	case capability.EvidenceBuild:
		return e.Kind == store.EvidenceBuild || e.Kind == store.EvidenceExport
	case capability.EvidenceTest:
		return e.Kind == store.EvidenceTest || (e.Kind == store.EvidenceCommand && looksLikeTests(e.Subject))
	case capability.EvidenceExport:
		return e.Kind == store.EvidenceExport
	case capability.EvidenceSmoke:
		return e.Kind == store.EvidenceSmoke
	case capability.EvidenceShot:
		return e.Kind == store.EvidenceShot
	case capability.EvidenceDocument:
		return e.Kind == store.EvidenceDocument || (e.Kind == store.EvidenceFile && documentExtensions[strings.ToLower(filepath.Ext(e.Subject))])
	case capability.EvidenceSources:
		return e.Kind == store.EvidenceSource
	case capability.EvidenceQuestions:
		return (e.Kind == store.EvidenceDocument || e.Kind == store.EvidenceFile) && e.OK && holdsQuestions(e.Subject)
	case capability.EvidenceVersions:
		return e.Kind == store.EvidenceVersion
	}

	return false
}

func looksLikeTests(command string) bool {
	for _, word := range []string{" test", "pytest", "vitest", "jest", "phpunit", "cargo test", "go test"} {
		if strings.Contains(" "+command, word) {
			return true
		}
	}

	return false
}

/*
 * holdsQuestions is a written file that asks things: somewhere it says it is
 * about questions, and it asks at least three of them, each on a line of its
 * own. The word and one question mark were once enough, and any document that
 * mentioned "a question?" once passed as the list a qualified person needs.
 */
func holdsQuestions(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 4<<20 {
		return false
	}

	text := strings.ToLower(string(raw))

	if !strings.Contains(text, "question") {
		return false
	}

	asked := 0

	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); len(line) > 12 && strings.HasSuffix(line, "?") {
			asked++
		}
	}

	return asked >= 3
}

/*
 * judged is a project task's end, weighed on its evidence.
 *
 * Steps all done and the evidence missing is not finished: it stops as
 * blocked, saying what nobody can yet show — which is the honest state of a
 * game that was "written" and never once run.
 */
func (c *Conductor) judged(task *store.Task, state, because string) (string, string) {
	// Any task working under a package is judged on its evidence, project or
	// not: a plan written for an electrician is held to its questions for the
	// electrician as much as a game is to its check.
	// A task handed one step of a project is not the project: its parent is
	// judged on the whole, with what the child did in its record.
	if (task.Project == "" && task.Packages == "") || state != store.TaskDone || task.ParentTaskID != 0 {
		return state, because
	}

	if missing := c.missingEvidence(task); len(missing) > 0 {
		return store.TaskBlocked, "its steps are done, but nothing yet shows " + strings.Join(missing, ", ")
	}

	return state, because
}

/*
 * evidenceOf is a task's evidence with its children's, in the order it was
 * recorded. A file a specialist changed for one of its steps is a change to
 * the project all the same: left out, a check passed before it would still
 * count as the latest word on files it never saw.
 */
func (c *Conductor) evidenceOf(task *store.Task) ([]store.Evidence, error) {
	rows, err := c.DB.EvidenceFor(task.ID)
	if err != nil {
		return nil, err
	}

	var walk func(id int64, depth int)

	walk = func(id int64, depth int) {
		if depth > MostDepth {
			return
		}

		children, err := c.DB.Children(id)
		if err != nil {
			return
		}

		for _, child := range children {
			if more, err := c.DB.EvidenceFor(child.ID); err == nil {
				rows = append(rows, more...)
			}

			walk(child.ID, depth+1)
		}
	}

	walk(task.ID, 1)

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })

	return rows, nil
}

// evidenceReport is the evidence, for the task's account.
func (c *Conductor) evidenceReport(task *store.Task) string {
	rows, err := c.DB.EvidenceFor(task.ID)
	if err != nil || len(rows) == 0 {
		return ""
	}

	var (
		lines []string
		files int
	)

	for _, e := range rows {
		switch e.Kind {
		case store.EvidenceFile:
			if e.OK {
				files++
			}
		case store.EvidenceCommand:
			lines = append(lines, fmt.Sprintf("• ran %s — %s", e.Subject, firstLine(e.Detail)))
		case store.EvidenceCheck, store.EvidenceBuild, store.EvidenceExport, store.EvidenceSmoke, store.EvidenceTest:
			state := "passed"
			if !e.OK {
				state = "failed"
			}

			lines = append(lines, fmt.Sprintf("• %s %s: %s", e.Kind, state, firstLine(e.Detail)))
		case store.EvidenceShot:
			lines = append(lines, "• picture: "+e.Subject)
		case store.EvidenceVersion:
			lines = append(lines, fmt.Sprintf("• %s %s", e.Subject, e.Detail))
		case store.EvidenceWarning:
			lines = append(lines, "• unresolved: "+e.Subject)
		}
	}

	if files > 0 {
		lines = append([]string{fmt.Sprintf("• %d files written", files)}, lines...)
	}

	if len(lines) > 25 {
		lines = append(lines[:25], fmt.Sprintf("• … and %d more", len(lines)-25))
	}

	var professional []string

	for _, p := range c.packagesOf(task) {
		if p.Escalation != "" {
			professional = append(professional, "• "+p.Escalation)
		}
	}

	out := "Evidence:\n" + strings.Join(lines, "\n")

	if len(professional) > 0 {
		out += "\n\nStill needs a qualified professional:\n" + strings.Join(professional, "\n")
	}

	return out
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")

	return trimTo(line, 160)
}

// rememberProject writes what came of a task into the project's own memory.
func (c *Conductor) rememberProject(task *store.Task, state, because string) {
	if task.Project == "" {
		return
	}

	line := task.Name + ": " + state

	if because != "" {
		line += " — " + because
	}

	if err := workspace.Remember(task.Project, line); err != nil {
		c.Log.Warn("could not write the project's memory", "project", task.Project, "error", err)
	}
}
