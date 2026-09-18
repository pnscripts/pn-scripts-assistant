package team

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/capability"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
)

// proposing is a brain with the catalogue, the shipped packages and recipes.
func proposing(t *testing.T) (Inputs, *store.DB) {
	t.Helper()

	root, db := withCatalogue(t)

	return Inputs{
		Root: root, Roster: Roster(root), Chart: org.BuiltIn(), Catalogue: db,
		Templates: Templates(root),
		Packages:  capability.Load("", capability.Known{}),
		Recipes:   provision.Standard(),
	}, db
}

// A proposal writes nothing. Everything it says, it says before any file
// exists.
func TestAProposalWritesNothing(t *testing.T) {
	in, _ := proposing(t)

	// The game developer that ships is already here.
	here, err := Propose(in, Wish{Sentence: "Hey Assistant, hire a game-development expert"})
	if err != nil || here.Existing == nil || here.Existing.Name != "game_developer" {
		t.Fatalf("the game developer already here was not found: %+v %v", here.Existing, err)
	}

	// An organisation without one is proposed one.
	in.Roster = nil

	p, err := Propose(in, Wish{Sentence: "Hey Assistant, hire a game-development expert"})
	if err != nil {
		t.Fatal(err)
	}

	if p.Job.ID != "software.game_developer" {
		t.Fatalf("a game-development expert is %s", p.Job.ID)
	}

	if entries, _ := os.ReadDir(Folder(in.Root)); len(entries) != 0 {
		t.Errorf("proposing wrote %d files", len(entries))
	}

	// No engine named: every game package, and the engine is each project's.
	if len(p.Packages) != 4 {
		t.Errorf("packages %v", p.Packages)
	}

	if p.Permanence != "" || !strings.Contains(p.Text(), "permanent, or for one task only?") {
		t.Errorf("permanence was assumed rather than asked: %q", p.Permanence)
	}

	for _, want := range []string{"Capability packages", "Needs on this machine", "Godot", "It can do itself", "Needs you"} {
		if !strings.Contains(p.Text(), want) {
			t.Errorf("the proposal does not say %q:\n%s", want, p.Text())
		}
	}
}

func TestEachExampleFindsItsPackage(t *testing.T) {
	in, _ := proposing(t)

	for sentence, want := range map[string]string{
		"Hire an electrician to help me plan this installation": "electrical.planning",
		"Hire a book writer to draft a novel":                   "writing.book",
		"Hire a construction engineer to review this plan":      "construction.engineering",
		"Hire a backend developer to build an API":              "software.backend",
		"Hire a Unity developer":                                "software.game.unity",
		"Hire a three.js game developer":                        "software.game.threejs",
	} {
		p, err := Propose(in, Wish{Sentence: sentence})
		if err != nil {
			t.Errorf("%q: %v", sentence, err)

			continue
		}

		if len(p.Packages) != 1 || p.Packages[0].ID != want {
			t.Errorf("%q came under %v (job %s), want %s", sentence, p.Packages, p.Job.ID, want)
		}
	}
}

/*
 * An electrician is advisory, and the proposal says so in every way that
 * matters: what it may not touch, what it may not claim, and who takes over.
 */
func TestAnElectricianIsAdvisoryAndSaysWhoTakesOver(t *testing.T) {
	in, _ := proposing(t)

	p, err := Propose(in, Wish{Sentence: "Hire an electrician to help me plan this installation"})
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"set_device", "run_command", "send_email"} {
		for _, tool := range p.Tools {
			if tool == forbidden {
				t.Errorf("an electrical planner may use %s", forbidden)
			}
		}
	}

	if p.Risk != "critical" || len(p.Professional) == 0 || p.Goal != "help me plan this installation" {
		t.Errorf("risk %s, professional %v, goal %q", p.Risk, p.Professional, p.Goal)
	}

	text := p.Text()

	for _, want := range []string{"Needs a qualified professional", "licensed electrician", "Must not"} {
		if !strings.Contains(text, want) {
			t.Errorf("the proposal does not say %q", want)
		}
	}
}

// Accepted, the file is what was proposed: its packages, its tools, its seat.
func TestAcceptingWritesWhatWasProposed(t *testing.T) {
	in, _ := proposing(t)

	p, err := Propose(in, Wish{Sentence: "hire a three.js game developer permanently"})
	if err != nil {
		t.Fatal(err)
	}

	if p.Permanence != Permanent {
		t.Fatalf("permanently was read as %q", p.Permanence)
	}

	hired, err := Accept(in.Root, p, 0)
	if err != nil {
		t.Fatal(err)
	}

	back, ok := Find(Roster(in.Root), hired.Agent.Name)
	if !ok {
		t.Fatal("the hire is not on the roster")
	}

	if strings.Join(back.Packages, ",") != "software.game.threejs" {
		t.Errorf("packages %v", back.Packages)
	}

	if strings.Join(back.Tools, ",") != strings.Join(p.Tools, ",") || len(back.Tools) == 0 {
		t.Errorf("the file's tools %v are not the proposal's %v", back.Tools, p.Tools)
	}

	if back.Position == "" {
		t.Error("a permanent hire was not seated")
	}

	// And asking again finds them rather than proposing a copy.
	again, err := Propose(Inputs{Root: in.Root, Roster: Roster(in.Root), Chart: org.Chart(in.Root),
		Catalogue: in.Catalogue, Packages: in.Packages}, Wish{Sentence: "hire a three.js game developer"})
	if err != nil || again.Existing == nil || again.Existing.Name != back.Name {
		t.Errorf("a second three.js developer was proposed: %+v %v", again.Existing, err)
	}

	// And the Godot developer that shipped is still the answer for Godot.
	godot, _ := Propose(Inputs{Root: in.Root, Roster: Roster(in.Root), Chart: org.Chart(in.Root),
		Catalogue: in.Catalogue, Packages: in.Packages}, Wish{Sentence: "hire a Godot expert"})
	if godot.Existing == nil || godot.Existing.Name != "game_developer" {
		t.Errorf("a Godot expert was proposed beside the one that ships: %+v", godot.Existing)
	}
}

func TestNothingIsWrittenUntilPermanenceIsDecided(t *testing.T) {
	in, _ := proposing(t)

	p, _ := Propose(in, Wish{Sentence: "hire a book writer"})

	if _, err := Accept(in.Root, p, 0); err == nil {
		t.Error("a hire was made with nobody having said for how long")
	}

	p.Permanence = TaskOnly

	hired, err := Accept(in.Root, p, 42)
	if err != nil {
		t.Fatal(err)
	}

	if hired.Agent.State != Temporary || hired.Agent.HiredFor != 42 || hired.Agent.Position != "" {
		t.Errorf("a task-only hire is %+v", hired.Agent)
	}
}

/*
 * However a hire is arrived at, its tools are a written list. With nothing to
 * derive them from, that list is none — never the empty list that means all.
 */
func TestAHireIsNeverGivenEverything(t *testing.T) {
	root, db := withCatalogue(t)

	hired, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, nil,
		Wish{Sentence: "someone who specialises in welding"})
	if err != nil {
		t.Fatal(err)
	}

	back, _ := Find(Roster(root), hired.Agent.Name)

	if len(back.Tools) == 0 {
		t.Fatal("a hire was written with no tool list, which means every tool")
	}

	fit := Settle(back, nil, db, nil)

	if only, with := fit.Only(); with || len(only) != 0 || !fit.Narrowed {
		t.Errorf("a hire with nothing to derive tools from may use %v (offered: %v)", only, with)
	}
}

func TestSplittingAWish(t *testing.T) {
	for sentence, want := range map[string][2]string{
		"hire an electrician to help me plan this installation": {"hire an electrician", "help me plan this installation"},
		"someone who specialises in Laravel security":           {"someone who specialises in Laravel security", ""},
		"someone to write my novel":                             {"someone to write my novel", "write my novel"},
	} {
		role, goal := SplitWish(sentence)

		if role != want[0] || goal != want[1] {
			t.Errorf("%q split as %q / %q", sentence, role, goal)
		}
	}
}

func TestPermanenceInEitherLanguage(t *testing.T) {
	for text, want := range map[string]string{
		"permanent please":          Permanent,
		"just for this task":        TaskOnly,
		"само за тази задача":       TaskOnly,
		"за постоянно":              Permanent,
		"a game developer, please.": "",
	} {
		if got := Answer(text); got != want {
			t.Errorf("%q read as %q, want %q", text, got, want)
		}
	}
}

// A job that keeps a person in the loop is never handed anything that acts on
// the world by way of its capabilities.
func TestAJobWithOversightAdvisesWithoutAPackage(t *testing.T) {
	root, db := withCatalogue(t)

	p, err := Propose(Inputs{Root: root, Chart: org.BuiltIn(), Catalogue: db, Toolbox: fakeBox{}},
		Wish{Sentence: "hire a plumber"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range p.Agent.Tools {
		if capability.Operates[tool] {
			t.Errorf("a plumber may use %s", tool)
		}
	}
}

// fakeBox hands out a command line to anybody who troubleshoots, the way the
// real mapping does.
type fakeBox struct{}

func (fakeBox) ForCapabilities(caps []string) []string {
	return []string{"read_file", "run_command", "what_this_machine_needs"}
}

// A word that only says which package is not a speciality: a book writer is
// the book package's writer, not a "book writer specialist".
func TestAPackagesOwnWordsAreNotASpeciality(t *testing.T) {
	in, _ := proposing(t)

	p, err := Propose(in, Wish{Sentence: "hire a book writer to draft a novel about the sea"})
	if err != nil {
		t.Fatal(err)
	}

	if p.Agent.Title != "Book writer" || strings.Contains(p.Agent.For, "Especially") {
		t.Errorf("titled %q, for %q", p.Agent.Title, p.Agent.For)
	}

	// Asking for more than the job and its package still says so.
	p, _ = Propose(in, Wish{Sentence: "someone who specialises in Laravel security"})

	if !strings.Contains(p.Agent.Title, "Laravel security") {
		t.Errorf("a real speciality was lost: %q", p.Agent.Title)
	}
}

// A task's own hire goes by itself only when it reaches no further than the
// task already could; otherwise it is a proposal.
func TestATaskHireThatWidensIsOnlyProposed(t *testing.T) {
	inside := Proposal{Permanence: TaskOnly, Agent: Agent{Tools: []string{"read_file", "write_file"}}}
	if why := Widens(inside); why != "" {
		t.Errorf("an ordinary task hire widens: %s", why)
	}

	for _, p := range []Proposal{
		{Permanence: Permanent},
		{Permanence: TaskOnly, Agent: Agent{Provider: "anthropic"}},
		{Permanence: TaskOnly, Agent: Agent{Tools: []string{"mcp_fetch_fetch"}}},
	} {
		if Widens(p) == "" {
			t.Errorf("a hire reaching further went unasked: %+v", p.Agent)
		}
	}
}
