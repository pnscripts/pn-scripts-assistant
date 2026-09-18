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
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Making games with Godot.
 *
 * Four capabilities and they are deliberately different sizes. Knowing whether
 * the engine is here, and what it can see, is a question — safe, instant, and
 * the one that has to be answered before any of the others make sense. Looking
 * a class up is the one that does the most work per line of code written.
 * Running and exporting change things and ask first.
 *
 * What is not here is "learn Godot". A model asked to write GDScript writes
 * GDScript whether or not it knows the engine, and what it produces is fluent
 * and half invented — method names that read exactly like real ones. Learning
 * a thousand classes would not fix that: a fact recalled by similarity is just
 * as confident when it is stale, and the reference changes every release.
 * Looking one up is authoritative, costs a second, and is checkable.
 */

// GodotStatus answers whether the engine is here and what it can see.
type GodotStatus struct {
	// Places is where the brain may look for projects — the folders somebody
	// has already agreed it can read, and nowhere else.
	Places func() []string

	// Online is whether privacy allows reaching out. An update check is a
	// small thing to leak and it is still a thing that says this machine
	// exists and runs Godot.
	Online func() bool
}

func (GodotStatus) Name() string { return "godot_status" }

func (GodotStatus) Description() string {
	return "Say whether the Godot game engine is installed, which version, " +
		"whether a newer one has been released, and which Godot projects are " +
		"on this machine. Use before anything else to do with Godot."
}

func (GodotStatus) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (GodotStatus) Risk() Risk { return Safe }

func (GodotStatus) Summarize(json.RawMessage) string { return "Check Godot" }

func (t GodotStatus) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	engine, installed := godot.Find()

	var b strings.Builder

	if !installed {
		/*
		 * Said as a fact with a next step, not as a failure.
		 *
		 * "Godot is not installed" is where most assistants stop, and it is
		 * the least useful half of the answer: the person asking almost
		 * always wants to know what to do about it.
		 */
		b.WriteString("Godot is not installed on this machine. " +
			"It is a single file — download the Linux build from godotengine.org, " +
			"make it executable, and put it in ~/.local/bin. I will find it there.")

		return b.String(), nil
	}

	fmt.Fprintf(&b, "Godot is installed: %s\n  %s", engine.Version, engine.Path)

	if t.Online != nil && t.Online() {
		if latest, err := godot.Latest(ctx, nil); err == nil {
			switch {
			case godot.Newer(engine.Version, latest.Version):
				fmt.Fprintf(&b, "\n\n%s has been released (%s). Yours is older.",
					latest.Version, latest.When)

			default:
				fmt.Fprintf(&b, "\n\nThat is the current release (%s).", latest.Version)
			}
		} else {
			fmt.Fprintf(&b, "\n\nI could not check for a newer one: %v", err)
		}
	} else {
		b.WriteString("\n\nI have not checked for a newer version — that needs " +
			"the network, and your privacy setting keeps this machine to itself.")
	}

	var roots []string

	if t.Places != nil {
		roots = t.Places()
	}

	var projects []godot.Project

	for _, root := range roots {
		found, err := godot.Projects(root)
		if err != nil {
			continue
		}

		projects = append(projects, found...)
	}

	if len(projects) == 0 {
		b.WriteString("\n\nThere are no Godot projects in the folders I can see.")

		return b.String(), nil
	}

	fmt.Fprintf(&b, "\n\n%d Godot project(s):", len(projects))

	for i, p := range projects {
		if i >= 20 {
			fmt.Fprintf(&b, "\n  … and %d more", len(projects)-i)

			break
		}

		fmt.Fprintf(&b, "\n  %s\n    %s", p.Name, p.Path)
	}

	return b.String(), nil
}

/*
 * GodotDocs looks a class up in the engine's own reference.
 *
 * The one that matters most for writing code that runs. Everything else here
 * is about the engine; this is about not inventing method names.
 */
type GodotDocs struct {
	Online func() bool
}

func (GodotDocs) Name() string { return "godot_docs" }

func (GodotDocs) Description() string {
	return "Look up a Godot class in the official class reference: its " +
		"properties, methods with exact signatures, and signals. Use this " +
		"before writing GDScript that calls into a class, and whenever you " +
		"are about to state that a method exists. Names are case-sensitive " +
		"and written like CharacterBody2D."
}

func (GodotDocs) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"class": {
				"type": "string",
				"description": "The class name, exactly: Node2D, CharacterBody2D, AnimationPlayer."
			},
			"method": {
				"type": "string",
				"description": "Optional. One method to look up, when the question is whether it exists and what it takes."
			}
		},
		"required": ["class"],
		"additionalProperties": false
	}`)
}

func (GodotDocs) Risk() Risk { return Safe }

func (GodotDocs) Summarize(args json.RawMessage) string {
	var a struct {
		Class  string `json:"class"`
		Method string `json:"method"`
	}

	json.Unmarshal(args, &a)

	if a.Method != "" {
		return fmt.Sprintf("Look up %s.%s in the Godot reference", a.Class, a.Method)
	}

	return "Look up " + a.Class + " in the Godot reference"
}

func (t GodotDocs) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Class  string `json:"class"`
		Method string `json:"method"`
	}

	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("could not read the arguments: %w", err)
	}

	if t.Online != nil && !t.Online() {
		return "", fmt.Errorf(
			"the class reference is on the internet and looking things up is " +
				"switched off. It is under Permissions — a separate switch from " +
				"privacy, because a class name says nothing about anybody. Or " +
				"open the reference in the Godot editor, which ships with it offline")
	}

	// The branch that matches the installed engine, so the answer is about the
	// engine that will run the code rather than whatever is newest.
	branch := "master"

	if engine, ok := godot.Find(); ok {
		branch = godot.BranchFor(engine.Version)
	}

	progress.Detail("reading the " + a.Class + " reference")

	class, err := godot.Lookup(ctx, nil, a.Class, branch)
	if err != nil {
		return "", err
	}

	if a.Method == "" {
		return class.Summary(40), nil
	}

	m, found := class.Find(a.Method)
	if !found {
		/*
		 * Said as plainly as possible, because this is the answer that stops
		 * a bug: the method does not exist, and everything written on the
		 * assumption that it does is wrong.
		 */
		return fmt.Sprintf("%s has no method called %q in Godot %s. It has: %s",
			class.Name, a.Method, branch, names(class)), nil
	}

	out := fmt.Sprintf("%s.%s", class.Name, m.Signature())

	if d := m.Describe; strings.TrimSpace(d) != "" {
		out += "\n" + strings.Join(strings.Fields(d), " ")
	}

	return out, nil
}

// names lists a class's methods, for when the one asked about is not there.
func names(c godot.Class) string {
	out := make([]string, 0, len(c.Methods))

	for i, m := range c.Methods {
		if i >= 40 {
			out = append(out, fmt.Sprintf("… and %d more", len(c.Methods)-i))

			break
		}

		out = append(out, m.Name)
	}

	if len(out) == 0 {
		return "no methods of its own"
	}

	return strings.Join(out, ", ")
}

// GodotBuild runs or exports a project.
type GodotBuild struct{}

func (GodotBuild) Name() string { return "godot_build" }

func (GodotBuild) Description() string {
	return "Run a Godot project to see whether it starts, or export a build of " +
		"it. Running checks it opens without errors; exporting writes a " +
		"finished game to a file. Both take minutes."
}

func (GodotBuild) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project": {
				"type": "string",
				"description": "Path to the project folder, the one holding project.godot."
			},
			"what": {
				"type": "string",
				"enum": ["check", "export"],
				"description": "check runs it headless and reports errors; export writes a build."
			},
			"preset": {
				"type": "string",
				"description": "For export: the export preset's name, as set in the project. Defaults to the first."
			},
			"into": {
				"type": "string",
				"description": "For export: where to write the build."
			}
		},
		"required": ["project", "what"],
		"additionalProperties": false
	}`)
}

// Mutating: it runs a program somebody wrote and can write a file.
func (GodotBuild) Risk() Risk { return Mutating }

func (GodotBuild) Summarize(args json.RawMessage) string {
	var a struct {
		Project string `json:"project"`
		What    string `json:"what"`
		Into    string `json:"into"`
	}

	json.Unmarshal(args, &a)

	if a.What == "export" {
		return fmt.Sprintf("Export %s to %s", filepath.Base(a.Project), a.Into)
	}

	return "Run " + filepath.Base(a.Project) + " to see whether it starts"
}

/*
 * Execute runs the Godot adapter, which is where running and exporting Godot
 * projects now lives — this tool is the name it has always had, kept for
 * everything that already knows it.
 */
func (t GodotBuild) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	out, _, err := t.ExecuteShowing(ctx, args)

	return out, err
}

func (GodotBuild) ExecuteShowing(ctx context.Context, args json.RawMessage) (string, []store.Evidence, error) {
	var a struct {
		Project string `json:"project"`
		What    string `json:"what"`
		Preset  string `json:"preset"`
		Into    string `json:"into"`
	}

	if err := json.Unmarshal(args, &a); err != nil {
		return "", nil, fmt.Errorf("could not read the arguments: %w", err)
	}

	adapter := engines.Godot{Preset: a.Preset}

	p, ok := adapter.Detect(a.Project)
	if !ok {
		return "", nil, fmt.Errorf("%s is not a Godot project — there is no project.godot in it", a.Project)
	}

	if _, installed := adapter.Engine(); !installed {
		return "", nil, fmt.Errorf("Godot is not installed on this machine")
	}

	if a.What == "export" {
		if strings.TrimSpace(a.Into) == "" {
			return "", nil, fmt.Errorf("where should the build go?")
		}

		folder := a.Into

		// A file named, as this tool always took: the folder is where it goes.
		if filepath.Ext(a.Into) != "" {
			folder, adapter.File = filepath.Dir(a.Into), a.Into
		}

		progress.Detail("exporting " + filepath.Base(a.Project))

		run := adapter.Build(ctx, p, folder)

		return run.Summary(), RunEvidence(run, store.EvidenceExport), nil
	}

	progress.Detail("running " + filepath.Base(a.Project))

	run := adapter.Check(ctx, p)

	return run.Summary(), RunEvidence(run, store.EvidenceCheck), nil
}
