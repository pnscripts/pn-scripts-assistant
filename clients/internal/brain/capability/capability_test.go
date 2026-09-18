package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/provision"
)

// known resolves jobs against the shipped catalogue and requirements against
// the shipped recipes. Tools are checked where the registry exists — see the
// brain package, which holds the real one.
func known() Known {
	jobs, _ := occupations.Seed()
	ids := map[string]bool{}

	for _, j := range jobs {
		ids[j.ID] = true
	}

	book := provision.Standard()

	return Known{
		Job:    func(id string) bool { return ids[id] },
		Recipe: book.Known,
	}
}

func TestEveryShippedPackageMeetsTheRules(t *testing.T) {
	shipped, err := BuiltIn()
	if err != nil {
		t.Fatal(err)
	}

	if len(shipped) < 11 {
		t.Fatalf("%d packages shipped", len(shipped))
	}

	for _, p := range shipped {
		for _, problem := range Check(p, known()) {
			t.Errorf("%s: %v", p.ID, problem)
		}
	}

	for _, want := range []string{
		"software.backend", "software.frontend", "software.game.godot", "software.game.threejs",
		"software.game.unity", "software.game.unreal", "writing.book", "construction.engineering",
		"electrical.planning", "research.market", "design.graphics",
	} {
		if _, ok := Load("", known()).Get(want); !ok {
			t.Errorf("%s did not load", want)
		}
	}
}

func valid() Package {
	return Package{
		ID: "test.thing", Version: "1.0.0", Title: "Thing", Summary: "A thing.",
		Work: Project, Jobs: []string{"software.software_engineer"},
		Role: Role{Title: "Thing maker", For: "making things", Tools: []string{"read_file"}},
	}
}

// Advisory work may not carry anything that operates on the world, however
// the package is written — and a tool from an MCP server counts as operating.
func TestAdvisoryWorkCannotOperate(t *testing.T) {
	for _, tool := range []string{"run_command", "set_device", "send_email", "mcp_home_switch"} {
		p := valid()
		p.Work = Advisory
		p.Role.Tools = []string{"read_file", tool}

		if len(Check(p, Known{})) == 0 {
			t.Errorf("advisory work was allowed %s", tool)
		}
	}
}

func TestWorkThatNeedsAProfessionalSaysWhoAndStaysAdvisory(t *testing.T) {
	p := valid()
	p.NeedsProfessional = true

	problems := Check(p, Known{})

	if len(problems) < 3 {
		t.Errorf("project work needing a professional, with no escalation or limits, passed with %v", problems)
	}

	p.Work = Advisory
	p.Escalation = "A licensed person signs it."
	p.Limits = []string{"Sign anything"}

	if problems := Check(p, Known{}); len(problems) != 0 {
		t.Errorf("a complete advisory package was refused: %v", problems)
	}
}

// A role with no tool list would be a hire with no limit.
func TestARoleAlwaysListsItsTools(t *testing.T) {
	p := valid()
	p.Role.Tools = nil

	if len(Check(p, Known{})) == 0 {
		t.Error("a role with no tool list passed")
	}

	p.Role.Tools = []string{"decide_waiting"}

	if len(Check(p, Known{})) == 0 {
		t.Error("a role was given the owner's own decisions")
	}
}

func TestAMisspelledKeyIsRefusedNotIgnored(t *testing.T) {
	if _, err := Parse([]byte(`{"id":"a.b","limtis":["x"]}`)); err == nil {
		t.Error("a misspelled limits key was silently ignored")
	}
}

func TestHerOwnReplacesWhatShippedAndABrokenFileIsReported(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, FolderName)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	own := `{"id":"writing.book","version":"2.0.0","title":"My book way","summary":"Mine.",
		"work":"advisory","jobs":["media.writer"],
		"role":{"title":"Writer","for":"my books","tools":["write_document"]}}`

	broken := `{"id":"writing.poems","version":"1.0.0","title":"Poems","summary":"Poems.",
		"work":"advisory","jobs":["media.writer"],
		"role":{"title":"Poet","for":"poems","tools":["run_command"]}}`

	os.WriteFile(filepath.Join(dir, "book.json"), []byte(own), 0o600)
	os.WriteFile(filepath.Join(dir, "poems.json"), []byte(broken), 0o600)

	set := Load(root, known())

	got, ok := set.Get("writing.book")
	if !ok || got.Version != "2.0.0" || got.BuiltIn {
		t.Errorf("her own package did not replace the shipped one: %+v", got)
	}

	if _, ok := set.Get("writing.poems"); ok {
		t.Error("a package that breaks the rules was loaded")
	}

	if why := set.Refused["poems.json"]; !strings.Contains(why, "run_command") {
		t.Errorf("the refusal did not say why: %q", why)
	}
}

func TestTheRequestChoosesBetweenPackagesForOneJob(t *testing.T) {
	set := Load("", known())

	found := set.For("software.game_developer", "make me a tetris game in three.js")

	if len(found) == 0 || found[0].ID != "software.game.threejs" {
		t.Fatalf("three.js was named and not chosen: %v", ids(found))
	}

	// Nothing named: every engine for the job comes back, and the choice is
	// the owner's to make.
	found = set.For("software.game_developer", "make me a tetris game")

	if len(found) < 4 {
		t.Errorf("only %v offered for a game with no engine named", ids(found))
	}

	if named := set.Named("make me a tetris game"); len(named) != 0 {
		t.Errorf("an engine was taken as named when none was: %v", ids(named))
	}

	if named := set.Named("help me plan the electrical work for this room"); len(named) == 0 ||
		named[0].ID != "electrical.planning" {
		t.Errorf("electrical work matched %v", ids(named))
	}
}

func TestStandingInstructionsCarryTheLimitsAndWhoTakesOver(t *testing.T) {
	p, _ := Load("", known()).Get("electrical.planning")

	told := p.Standing()

	for _, want := range []string{"You must not", "safe", "licensed electrician", "certified"} {
		if !strings.Contains(told, want) {
			t.Errorf("the standing instructions do not say %q:\n%s", want, told)
		}
	}

	if len(p.Needs()) == 0 {
		t.Error("a package needing a professional says it needs nothing")
	}
}

func ids(list []Package) []string {
	out := make([]string, 0, len(list))

	for _, p := range list {
		out = append(out, p.ID)
	}

	return out
}

// An owner's copy of a shipped package may narrow it, never open it up; and
// work for a job that keeps a professional in the loop stays advisory
// whatever a package says about itself.
func TestACopyMayNotWeakenWhatShipped(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, FolderName)
	os.MkdirAll(dir, 0o700)

	shipped, _ := BuiltIn()

	for _, p := range shipped {
		if p.ID != "electrical.planning" {
			continue
		}

		p.Work, p.NeedsProfessional, p.Escalation = Operating, false, ""
		p.Role.Tools = append(p.Role.Tools, "set_device", "run_command")

		raw, _ := json.Marshal(p)
		os.WriteFile(filepath.Join(dir, "electrical.planning.json"), raw, 0o600)
	}

	k := known()
	k.Oversight = func(job string) bool { return job == "trades.electrician" }

	set := Load(root, k)

	got, _ := set.Get("electrical.planning")
	if got.Work != Advisory || !got.NeedsProfessional || !got.BuiltIn {
		t.Errorf("an owner's copy took the electrician's limits away: %+v", got)
	}

	why := set.Refused["electrical.planning.json"]
	for _, want := range []string{"professional", "advisory", "set_device", "run_command"} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not say %q: %s", want, why)
		}
	}

	// A new package for the same job fares no better.
	own := `{"id":"electrical.wiring","version":"1.0.0","title":"Wiring","summary":"Wires things.",
		"work":"operating","jobs":["trades.electrician"],
		"role":{"title":"Wirer","for":"wiring","tools":["read_file"]}}`
	os.WriteFile(filepath.Join(dir, "wiring.json"), []byte(own), 0o600)

	if _, ok := Load(root, k).Get("electrical.wiring"); ok {
		t.Error("an operating package for an electrician's job was loaded")
	}
}
