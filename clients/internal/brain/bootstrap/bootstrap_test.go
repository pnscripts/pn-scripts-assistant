package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/engines"
	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/team"
	"pn-scripts-assistant/internal/stage"
)

// inputs is a brain's worth of everything, in a home folder of the test's own.
func inputs(t *testing.T) (Inputs, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	root := filepath.Join(home, "brain")
	os.MkdirAll(root, 0o700)

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	if _, err := occupations.Ensure(db); err != nil {
		t.Fatal(err)
	}

	team.Forget()

	packages := capability.Load("", capability.Known{})

	return Inputs{
		Engines:  engines.NewRegistry(engines.Godot{}, engines.ThreeJS(stage.Three, stage.ThreeRevision, nil)),
		Packages: packages,
		Recipes:  provision.Standard(),
		Hiring: team.Inputs{Root: root, Roster: team.Roster(root), Chart: org.BuiltIn(), Catalogue: db,
			Templates: team.Templates(root), Packages: packages, Recipes: provision.Standard()},
		Own: []string{root},
	}, home
}

// With no folder, the only answer is the question — nothing is guessed.
func TestNoFolderMeansAskWhere(t *testing.T) {
	in, _ := inputs(t)

	p := Propose(in, Request{Sentence: "Make me a Tetris game"})

	if p.Question != WhereQuestion || p.Dir != "" || len(p.Steps) != 0 {
		t.Errorf("proposed without a folder: %+v", p)
	}

	if p.Name != "Tetris" {
		t.Errorf("named %q", p.Name)
	}
}

// An engine nobody named is asked for, with what each would take.
func TestAnEngineNobodyNamedIsAskedFor(t *testing.T) {
	in, home := inputs(t)

	p := Propose(in, Request{Sentence: "Make me a Tetris game", Dir: filepath.Join(home, "games", "tetris")})

	if !strings.Contains(p.Question, "Which engine") || len(p.Options) < 4 {
		t.Fatalf("the engine was not asked for: %q %+v", p.Question, p.Options)
	}

	for _, o := range p.Options {
		if o.Package == "software.game.threejs" && !strings.Contains(o.Says, "ready now") {
			t.Errorf("three.js needs nothing installed, and was described as %q", o.Says)
		}

		if o.Package == "software.game.unity" && !strings.Contains(o.Says, "license it yourself") {
			t.Errorf("Unity was described as %q", o.Says)
		}
	}
}

func TestATetrisGameInThreeJS(t *testing.T) {
	in, home := inputs(t)

	dir := filepath.Join(home, "games", "tetris")

	p := Propose(in, Request{Sentence: "Make me a Tetris game", Dir: dir, Package: "software.game.threejs"})

	if !p.Ready() || !p.New || p.Kind != "game" || p.Engine != "threejs" {
		t.Fatalf("not ready: %s", p.Text())
	}

	for _, want := range []string{"index.html", "src/main.js", "vendor/three.module.js"} {
		if !contains(p.Files, want) {
			t.Errorf("a new three.js project would not have %s: %v", want, p.Files)
		}
	}

	if p.Hire == nil || p.Hire.Permanence != team.TaskOnly || p.Agent == "" {
		t.Fatalf("nobody works under three.js, and no hire for this project was proposed: %+v", p.Hire)
	}

	// Hired as the package names the role, not as a sentence about it.
	if p.Hire.Agent.Title != "Web game developer" || len(p.Hire.Packages) != 1 {
		t.Errorf("the hire is %q under %v", p.Hire.Agent.Title, p.Hire.Packages)
	}

	for _, s := range p.Steps {
		if s.Assignee != p.Agent {
			t.Errorf("a step is not the hire's: %+v", s)
		}
	}

	if len(p.Steps) != 4 || !contains(p.Done, "a picture of it running") {
		t.Errorf("steps %d, done %v", len(p.Steps), p.Done)
	}

	if len(p.Installs) != 0 {
		t.Errorf("three.js needs nothing installed, and %v would be", p.Installs)
	}

	text := p.Text()

	for _, want := range []string{"write only inside " + dir, "dist", "nothing existing is overwritten", "Plan:"} {
		if !strings.Contains(text, want) {
			t.Errorf("the proposal does not say %q:\n%s", want, text)
		}
	}
}

// A named engine that is missing is installed as part of saying yes, and the
// proposal says what that is.
func TestAGodotGameProposesInstallingGodot(t *testing.T) {
	in, home := inputs(t)

	if provision.Standard().Check("godot", "").Present {
		t.Skip("godot is installed on this machine")
	}

	p := Propose(in, Request{Sentence: "make me a tetris game in godot", Dir: filepath.Join(home, "tetris")})

	if p.Question != "" || p.Engine != "godot" {
		t.Fatalf("godot was named and not chosen: %s", p.Text())
	}

	if !contains(p.Installs, "godot") {
		t.Errorf("a missing Godot is not proposed for installing: %v, blockers %v", p.Installs, p.Blockers)
	}

	if !strings.Contains(p.Text(), "will be installed first") {
		t.Errorf("the proposal does not say what would be installed:\n%s", p.Text())
	}
}

func TestAFolderFullOfOtherThingsGetsAFolderOfItsOwn(t *testing.T) {
	in, home := inputs(t)

	projects := filepath.Join(home, "Projects")
	os.MkdirAll(filepath.Join(projects, "invoices"), 0o755)
	os.WriteFile(filepath.Join(projects, "notes.txt"), []byte("x"), 0o644)

	p := Propose(in, Request{Sentence: "Make me a Tetris game", Dir: projects, Package: "software.game.threejs"})

	if p.Dir != filepath.Join(projects, "tetris") || !p.New {
		t.Errorf("the project would go in %s", p.Dir)
	}
}

func TestAnExistingProjectIsWorkedOnAsItIs(t *testing.T) {
	in, home := inputs(t)

	dir := filepath.Join(home, "blocks")
	files, _ := engines.ThreeJS(stage.Three, stage.ThreeRevision, nil).Scaffold(engines.Scaffold{Name: "Blocks"})

	for _, f := range files {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(f.Path)), 0o755)
		os.WriteFile(filepath.Join(dir, f.Path), f.Content, 0o644)
	}

	p := Propose(in, Request{Sentence: "add a high score to it", Dir: dir})

	if p.Question != "" || p.Engine != "threejs" || p.New || len(p.Files) != 0 {
		t.Errorf("an existing three.js project: %s", p.Text())
	}
}

func TestNoProjectInTheBrainsOwnFolder(t *testing.T) {
	in, _ := inputs(t)

	p := Propose(in, Request{Sentence: "make me a game", Dir: filepath.Join(in.Own[0], "game")})

	if !strings.Contains(p.Question, "can't use") || !strings.Contains(p.Question, WhereQuestion) {
		t.Errorf("a project was proposed inside the brain: %q", p.Question)
	}
}

func TestElectricalWorkIsAPlanForAProfessional(t *testing.T) {
	in, home := inputs(t)

	p := Propose(in, Request{Sentence: "Help me plan the electrical work for this room", Dir: filepath.Join(home, "kitchen")})

	if p.Question != "" || p.Package == nil || p.Package.ID != "electrical.planning" || p.Kind != "plan" {
		t.Fatalf("electrical work: %s", p.Text())
	}

	if len(p.Professional) == 0 || !contains(p.Done, "the questions for a qualified person") {
		t.Errorf("who takes over, or the questions, are missing: %s", p.Text())
	}

	if p.Hire == nil {
		t.Fatal("no electrical planner was proposed")
	}

	for _, tool := range p.Hire.Tools {
		if tool == "set_device" || tool == "run_command" {
			t.Errorf("the electrical planner may use %s", tool)
		}
	}
}

func TestNamesComeFromTheRequest(t *testing.T) {
	for sentence, want := range map[string]string{
		"Make me a Tetris game":                         "Tetris",
		"Hey Assistant, build a snake game in three.js": "Snake",
		"write a novel about the sea":                   "Novel about the sea",
		"make me something":                             "Something",
		"a Tetris game":                                 "Tetris",
		"":                                              "New project",
		"Create a Three.js game where you catch falling stars": "Catch falling stars",
		"Create a Unity game":                       "Unity game",
		"Create a small Godot game called Starfall": "Starfall",
		"Make an Unreal game":                       "Unreal game",
		"make a small 2D platformer in Godot":       "Platformer",
	} {
		if got := nameOf(sentence); got != want {
			t.Errorf("%q named %q, want %q", sentence, got, want)
		}
	}
}

func contains(list []string, want string) bool {
	for _, one := range list {
		if one == want {
			return true
		}
	}

	return false
}

/*
 * An engine that is installed and not licensed is not ready, and says so:
 * the audit found Unity offered as "ready now — everything it needs is here"
 * while it refused to run unattended.
 */
func TestAnUnlicensedEngineIsNotOfferedAsReady(t *testing.T) {
	in, _ := inputs(t)

	unity := provision.Recipe{ID: "unity", Title: "Unity editor", Kind: provision.Engine,
		Find:     func() (string, string, bool) { return "/opt/unity/Editor/Unity", "6000.5.4f1", true },
		Licensed: func(string) (string, string) { return provision.LicenceInactive, "no licence for batch mode" },
		Manual:   "sign in to Unity Hub"}

	in.Recipes = provision.NewBook(unity)
	in.Engines = engines.NewRegistry(engines.Unity{Find: unity.Find})

	unityPackage, ok := in.Packages.Get("software.game.unity")
	if !ok {
		t.Fatal("no Unity package")
	}

	if ready := readiness(in, unityPackage); strings.Contains(ready, "ready now") || !strings.Contains(ready, "not licensed") {
		t.Errorf("an unlicensed Unity reads as: %s", ready)
	}

	p := Propose(in, Request{Sentence: "Create a Unity game", Dir: filepath.Join(t.TempDir(), "game")})

	if p.Ready() || !strings.Contains(strings.Join(p.Blockers, " "), "not licensed") {
		t.Errorf("a Unity project could start without a licence: blockers %v", p.Blockers)
	}

	if strings.Contains(p.Text(), "— here") {
		t.Errorf("the proposal says the unlicensed editor is simply here:\n%s", p.Text())
	}
}
