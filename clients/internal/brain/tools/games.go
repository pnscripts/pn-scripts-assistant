package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/godot"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * Games, whatever they are made in.
 *
 * Four tools over the engine contract: which engines are here, the
 * documentation for the version actually in use, checking a project, and
 * building, exporting or running it. The same four whether the project is a
 * Godot game, a three.js page or a Unity project — the engine is read from the
 * project, so a developer does not need to know which tool goes with which
 * engine, and a small model has four names to choose between rather than
 * twenty.
 */

// Games is what the game tools share.
type Games struct {
	Engines *engines.Registry

	// Online is whether documentation may be looked up.
	Online func() bool

	// Places are the folders projects are looked for in.
	Places func() []string
}

// projectAt is the engine and project in a folder.
func (g Games) projectAt(dir string) (engines.Adapter, engines.Project, error) {
	if !filepath.IsAbs(dir) {
		return nil, engines.Project{}, fmt.Errorf("give the whole path to the project folder")
	}

	adapter, p, ok := g.Engines.Detect(dir)
	if !ok {
		return nil, engines.Project{}, fmt.Errorf("%s is not a project of any game engine this program knows", dir)
	}

	return adapter, p, nil
}

// GameEngines says which engines are here and which projects there are.
type GameEngines struct{ Games }

func (GameEngines) Name() string { return "game_engines" }

func (GameEngines) Description() string {
	return "Say which game engines are installed (Godot, three.js, Unity, Unreal and others), " +
		"which versions, and which game projects are in the folders this program can see. " +
		"Use before starting work on a game."
}

func (GameEngines) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (GameEngines) Risk() Risk                       { return Safe }
func (GameEngines) Summarize(json.RawMessage) string { return "Check which game engines are here" }

func (t GameEngines) Execute(context.Context, json.RawMessage) (string, error) {
	var b strings.Builder

	b.WriteString("Engines:\n")

	for _, a := range t.Engines.All() {
		engine, ok := a.Engine()

		line := "  " + a.Title() + ": "

		switch {
		case ok && engine.Version != "":
			line += "installed, " + engine.Version
		case ok:
			line += "available — " + engine.Where
		default:
			line += "not installed"
		}

		if a.Licensed() {
			line += " (licensed: its owner installs it and accepts its terms)"
		}

		b.WriteString(line + "\n")
	}

	if t.Places != nil {
		var found []engines.Project

		for _, place := range t.Places() {
			found = append(found, t.Engines.Find(place, 40)...)
		}

		if len(found) == 0 {
			b.WriteString("\nNo game projects in the folders this program can see.")
		} else {
			b.WriteString("\nProjects:\n")

			for i, p := range found {
				if i == 30 {
					fmt.Fprintf(&b, "  … and %d more\n", len(found)-i)

					break
				}

				fmt.Fprintf(&b, "  %s (%s%s) — %s\n", p.Name, p.Engine, targets(p), p.Dir)
			}
		}
	}

	return strings.TrimSpace(b.String()), nil
}

func targets(p engines.Project) string {
	if p.Targets == "" {
		return ""
	}

	return " " + p.Targets
}

// GameDocs looks something up in the documentation for the version in use.
type GameDocs struct{ Games }

func (GameDocs) Name() string { return "game_docs" }

func (GameDocs) Description() string {
	return "Look something up in the official documentation of the engine a project uses, for " +
		"exactly the version it uses: a Godot class, a three.js class, a Unity or Unreal API. " +
		"Use before writing code that calls into the engine, rather than remembering."
}

func (GameDocs) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project": {"type": "string", "description": "The project folder, so the engine and version are the project's."},
			"engine": {"type": "string", "description": "Or an engine by name — godot, threejs, unity, unreal — when there is no project yet."},
			"topic": {"type": "string", "description": "A class or API name, exactly: CharacterBody2D, PerspectiveCamera, MonoBehaviour."}
		},
		"additionalProperties": false
	}`)
}

func (GameDocs) Risk() Risk { return Safe }

type docsArgs struct {
	Project string `json:"project"`
	Engine  string `json:"engine"`
	Topic   string `json:"topic"`
}

func (GameDocs) Summarize(raw json.RawMessage) string {
	var a docsArgs

	json.Unmarshal(raw, &a)

	return "Look up " + firstNonEmpty(a.Topic, "the documentation") + " for " + firstNonEmpty(a.Project, a.Engine, "the engine")
}

func (t GameDocs) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a docsArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	var (
		adapter engines.Adapter
		version string
	)

	switch {
	case a.Project != "":
		found, p, err := t.projectAt(a.Project)
		if err != nil {
			return "", err
		}

		adapter, version = found, p.Targets
	case a.Engine != "":
		found, ok := t.Engines.Get(strings.ToLower(a.Engine))
		if !ok {
			return "", fmt.Errorf("there is no engine called %q", a.Engine)
		}

		adapter = found
	default:
		return "", fmt.Errorf("say which project, or which engine")
	}

	if installed, ok := adapter.Engine(); ok && installed.Version != "" && (version == "" || adapter.ID() == "godot") {
		// The installed engine is what will run the code.
		version = installed.Version
	}

	docs := adapter.Docs(version, a.Topic)

	var b strings.Builder

	fmt.Fprintf(&b, "%s documentation for version %s:\n", adapter.Title(), firstNonEmpty(version, "unknown"))

	for _, d := range docs {
		fmt.Fprintf(&b, "  %s — %s\n", d.Title, d.URL)
	}

	if a.Topic == "" || t.Online == nil || !t.Online() {
		if a.Topic != "" {
			b.WriteString("\nLooking it up needs the network, and looking things up is switched off " +
				"under Permissions.")
		}

		return strings.TrimSpace(b.String()), nil
	}

	progress.Detail("reading the " + adapter.Title() + " reference for " + a.Topic)

	switch adapter.ID() {
	case "godot":
		class, err := godot.Lookup(ctx, nil, a.Topic, godot.BranchFor(version))
		if err != nil {
			return "", err
		}

		b.WriteString("\n" + class.Summary(40))
	case "threejs":
		text, err := threeDocs(ctx, version, a.Topic)
		if err != nil {
			b.WriteString("\nCould not read it: " + err.Error())
		} else {
			b.WriteString("\n" + text)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

/*
 * threeDocs reads one class from the three.js documentation of one release.
 *
 * The release's own list of pages says where a class's page is; the page is
 * then read from the same tag. Both from github, through the same guarded
 * fetch everything else uses.
 */
func threeDocs(ctx context.Context, version, topic string) (string, error) {
	listURL := "https://raw.githubusercontent.com/mrdoob/three.js/r" + version + "/docs/list.json"

	listed, err := FetchURL{}.Execute(ctx, json.RawMessage(`{"url":"`+listURL+`"}`))
	if err != nil {
		return "", err
	}

	// The list maps titles to paths under docs/, in several languages; the
	// English API section is enough to find a class.
	at := strings.Index(listed, `"`+topic+`": "`)
	if at < 0 {
		return "", fmt.Errorf("r%s has no class called %s", version, topic)
	}

	rest := listed[at+len(topic)+5:]

	path, _, found := strings.Cut(rest, `"`)
	if !found {
		return "", fmt.Errorf("the list for r%s could not be read", version)
	}

	page := "https://raw.githubusercontent.com/mrdoob/three.js/r" + version + "/docs/" + path + ".html"

	text, err := FetchURL{}.Execute(ctx, json.RawMessage(`{"url":"`+page+`"}`))
	if err != nil {
		return "", err
	}

	return "From " + page + ":\n" + text, nil
}

// GameCheck checks a project: well formed, and it loads.
type GameCheck struct{ Games }

func (GameCheck) Name() string { return "game_check" }

func (GameCheck) Description() string {
	return "Check a game project, whatever engine it uses: its files are well formed and name " +
		"each other correctly, and it loads and runs its first moments headless. Reports each " +
		"problem with its file and line. Use after every change that could break loading."
}

func (GameCheck) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"project": {"type": "string", "description": "The project folder."}},
		"required": ["project"],
		"additionalProperties": false
	}`)
}

// Mutating: it runs code somebody wrote.
func (GameCheck) Risk() Risk { return Mutating }

func (GameCheck) Summarize(raw json.RawMessage) string {
	var a struct{ Project string }

	json.Unmarshal(raw, &a)

	return "Check " + a.Project + " (runs it headless, with no window)"
}

func (GameCheck) Touches(raw json.RawMessage) ([]string, []string) {
	var a struct{ Project string }

	json.Unmarshal(raw, &a)

	return nil, []string{a.Project}
}

func (t GameCheck) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	out, _, err := t.ExecuteShowing(ctx, raw)

	return out, err
}

func (t GameCheck) ExecuteShowing(ctx context.Context, raw json.RawMessage) (string, []store.Evidence, error) {
	var a struct {
		Project string `json:"project"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", nil, err
	}

	adapter, p, err := t.projectAt(a.Project)
	if err != nil {
		return "", nil, err
	}

	progress.Detail("checking " + p.Name)

	run := adapter.Check(ctx, p)

	return run.Summary(), RunEvidence(run, store.EvidenceCheck), nil
}

// GameBuild builds, exports or runs a project.
type GameBuild struct{ Games }

func (GameBuild) Name() string { return "game_build" }

func (GameBuild) Description() string {
	return "Build or export a game project into its output folder, or run it for a few seconds " +
		"(smoke) and take a picture of it. Reports exactly which commands ran, how they ended, " +
		"each error with its file and line, and what was made. Builds take minutes."
}

func (GameBuild) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project": {"type": "string", "description": "The project folder."},
			"what": {"type": "string", "enum": ["build", "smoke"], "description": "build makes the finished game; smoke runs it briefly and photographs it."},
			"into": {"type": "string", "description": "For build: the output folder inside the project. Defaults to the project's own."}
		},
		"required": ["project", "what"],
		"additionalProperties": false
	}`)
}

func (GameBuild) Risk() Risk { return Mutating }

type buildArgs struct {
	Project string `json:"project"`
	What    string `json:"what"`
	Into    string `json:"into"`
}

func (GameBuild) Weight(json.RawMessage) risk.Level { return risk.Medium }

func (t GameBuild) Summarize(raw json.RawMessage) string {
	var a buildArgs

	json.Unmarshal(raw, &a)

	if a.What == "smoke" {
		return "Run " + a.Project + " for a few seconds, headless, and photograph it"
	}

	return "Build " + a.Project + " into " + t.into(a)
}

// into is where a build goes: what was asked, or the project's first output
// folder, or the engine's usual one.
func (t GameBuild) into(a buildArgs) string {
	if a.Into != "" {
		if filepath.IsAbs(a.Into) {
			return a.Into
		}

		return filepath.Join(a.Project, a.Into)
	}

	if c, err := workspace.Load(a.Project); err == nil && len(c.Outputs) > 0 {
		return filepath.Join(a.Project, c.Outputs[0])
	}

	usual := map[string]string{"godot": "build", "threejs": "dist", "unity": "Builds", "unreal": "Packaged"}

	if adapter, _, ok := t.Engines.Detect(a.Project); ok && usual[adapter.ID()] != "" {
		return filepath.Join(a.Project, usual[adapter.ID()])
	}

	return filepath.Join(a.Project, "build")
}

func (t GameBuild) Touches(raw json.RawMessage) ([]string, []string) {
	var a buildArgs

	json.Unmarshal(raw, &a)

	if a.What == "smoke" {
		return []string{filepath.Join(a.Project, workspace.Evidence)}, []string{a.Project}
	}

	return []string{t.into(a)}, []string{a.Project}
}

func (t GameBuild) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	out, _, err := t.ExecuteShowing(ctx, raw)

	return out, err
}

func (t GameBuild) ExecuteShowing(ctx context.Context, raw json.RawMessage) (string, []store.Evidence, error) {
	var a buildArgs

	if err := argsOf(raw, &a); err != nil {
		return "", nil, err
	}

	adapter, p, err := t.projectAt(a.Project)
	if err != nil {
		return "", nil, err
	}

	switch a.What {
	case "smoke":
		progress.Detail("running " + p.Name + " for a few seconds")

		run := adapter.Smoke(ctx, p, filepath.Join(a.Project, workspace.Evidence))

		return run.Summary(), RunEvidence(run, store.EvidenceSmoke), nil

	case "build", "export":
		into := t.into(a)

		/*
		 * Builds go where the project says builds go, and nowhere else.
		 *
		 * A folder of the project's settings when it has them: an export
		 * aimed at the source folder would overwrite what it was built from.
		 */
		if c, err := workspace.Load(a.Project); err == nil {
			if !workspace.ScopeOf(a.Project, c).Output(into) {
				return "", nil, fmt.Errorf("%s is not one of this project's output folders (%s)",
					into, strings.Join(c.Outputs, ", "))
			}
		}

		progress.Detail("building " + p.Name)

		run := adapter.Build(ctx, p, into)

		return run.Summary(), RunEvidence(run, store.EvidenceBuild), nil
	}

	return "", nil, fmt.Errorf("build or smoke?")
}

/*
 * RunEvidence is an engine's run as evidence rows: every command exactly, the
 * outcome of the kind asked for, what it made, the picture, and the version.
 */
func RunEvidence(run engines.Run, kind string) []store.Evidence {
	var out []store.Evidence

	for _, argv := range run.Commands {
		out = append(out, store.Evidence{Kind: store.EvidenceCommand, Subject: quotedArgv(argv),
			Detail: fmt.Sprintf("exit status %d, %s, in %s", run.Exit, firstNonEmpty(run.Took, "time not measured"), run.Dir),
			OK:     run.Problem == "" && run.Exit == 0})
	}

	detail := fmt.Sprintf("%d errors", run.Errors())

	for i, d := range run.Diagnostics {
		if i == 8 {
			break
		}

		detail += "\n" + d.String()
	}

	if run.Problem != "" {
		detail = "could not run: " + run.Problem
	}

	if run.What == "export" {
		kind = store.EvidenceExport
	}

	out = append(out, store.Evidence{Kind: kind, Subject: run.Engine + " " + run.What, Detail: detail, OK: run.OK})

	for _, a := range run.Artifacts {
		artifact := store.EvidenceBuild
		if run.What == "export" {
			artifact = store.EvidenceExport
		}

		out = append(out, store.Evidence{Kind: artifact, Subject: a, Detail: "made by " + run.What, OK: run.OK})
	}

	if run.Screenshot != "" {
		// A picture of a run that failed, or of nothing drawn, proves nothing.
		out = append(out, store.Evidence{Kind: store.EvidenceShot, Subject: run.Screenshot, OK: run.OK})
	}

	for _, n := range run.Notes {
		if strings.HasPrefix(n, "no picture") {
			out = append(out, store.Evidence{Kind: store.EvidenceWarning, Subject: n})
		}
	}

	if run.Version != "" {
		out = append(out, store.Evidence{Kind: store.EvidenceVersion, Subject: run.Engine, Detail: run.Version, OK: true})
	}

	return out
}
