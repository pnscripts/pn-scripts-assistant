package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
)

// withCatalogue is a brain root and the real job catalogue, as a new brain
// has it the moment it starts.
func withCatalogue(t *testing.T) (string, *store.DB) {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	if _, err := occupations.Ensure(db); err != nil {
		t.Fatal(err)
	}

	Forget()

	return root, db
}

/*
 * The specification's own sentence, answered from the catalogue.
 *
 * Nobody here does Laravel security, so somebody is hired: from a real job,
 * knowing what the sentence named, sitting in a seat beside the people who do
 * similar work — and written as a file, like every agent.
 */
func TestHiringFromASentence(t *testing.T) {
	root, db := withCatalogue(t)

	hired, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, Templates(root),
		Wish{Sentence: "I need someone who specializes in Laravel security"})
	if err != nil {
		t.Fatal(err)
	}

	if hired.Existing {
		t.Fatalf("somebody already here was said to do Laravel security: %+v", hired.Agent)
	}

	a := hired.Agent

	if !strings.Contains(a.Title, "Laravel security") || a.Job == "" {
		t.Errorf("the hire does not say what it is for: %+v", a)
	}

	if a.Position == "" || hired.Seat == "" {
		t.Errorf("the hire was not given a seat: %+v", hired)
	}

	if _, err := os.Stat(filepath.Join(Folder(root), a.Name+".md")); err != nil {
		t.Errorf("the hire is not a file in the agents folder: %v", err)
	}

	fit := Settle(a, org.Chart(root), db, nil)

	if !knowsAll(fit.Can, []string{"laravel"}) {
		t.Errorf("the hire does not know Laravel: %v", fit.Can)
	}

	// And the seat is on the chart now, so the next person to look finds them.
	if _, _, ok := org.Seat(org.Chart(root), a.Position); !ok {
		t.Errorf("the seat %q is not on the chart", a.Position)
	}
}

// The same wish in other words finds the same person — including when the
// job already covered one of the things named, which is the case that once
// hired a second Laravel security specialist.
func TestAskingAgainInOtherWordsFindsTheSameHire(t *testing.T) {
	root, db := withCatalogue(t)

	first, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, Templates(root),
		Wish{Sentence: "specialises in Laravel security"})
	if err != nil || first.Existing {
		t.Fatalf("first hire: %+v %v", first, err)
	}

	again, err := Hire(root, Roster(root), org.Chart(root), db, nil, Templates(root),
		Wish{Sentence: "someone who knows Laravel security"})
	if err != nil {
		t.Fatal(err)
	}

	if !again.Existing || again.Agent.Name != first.Agent.Name {
		t.Errorf("asking again hired %s rather than finding %s", again.Agent.Name, first.Agent.Name)
	}
}

// Asking twice does not hire twice. Somebody who already does it is the
// answer, and two agents that differ in name make the planner's choice harder.
func TestHiringTheSameSpecialistTwiceFindsTheFirst(t *testing.T) {
	root, db := withCatalogue(t)

	wish := Wish{Sentence: "Create an agent specialized in PostgreSQL performance"}

	first, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, Templates(root), wish)
	if err != nil || first.Existing {
		t.Fatalf("first hire: %+v %v", first, err)
	}

	again, err := Hire(root, Roster(root), org.Chart(root), db, nil, Templates(root), wish)
	if err != nil {
		t.Fatal(err)
	}

	if !again.Existing || again.Agent.Name != first.Agent.Name {
		t.Errorf("a second identical specialist was hired: %+v", again.Agent)
	}
}

/*
 * A hire for one task is temporary, has no seat, may use no more than whoever
 * asked, and is gone when the task is.
 */
func TestATemporaryHireIsNarrowAndDissolves(t *testing.T) {
	root, db := withCatalogue(t)

	within := []string{"read_file", "list_directory"}

	hired, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, Templates(root),
		Wish{Sentence: "product manager", ForTask: 7, Within: &within})
	if err != nil {
		t.Fatal(err)
	}

	a := hired.Agent

	if a.State != Temporary || a.HiredFor != 7 || a.Position != "" {
		t.Errorf("a hire for one task is not temporary and unseated: %+v", a)
	}

	for _, tool := range a.Tools {
		if tool != "read_file" && tool != "list_directory" {
			t.Errorf("the hire may use %s, which whoever asked could not", tool)
		}
	}

	// Read back from the file, which is what anything else will see.
	back, ok := Find(Roster(root), a.Name)
	if !ok || back.HiredFor != 7 || back.State != Temporary {
		t.Fatalf("the file does not keep what it was hired for: %+v", back)
	}

	gone, err := Dissolve(root, Roster(root), 7)
	if err != nil || len(gone) != 1 {
		t.Fatalf("dissolving let go of %d: %v", len(gone), err)
	}

	if _, ok := Find(Roster(root), a.Name); ok {
		t.Error("a temporary hire is still on the roster after its task")
	}
}

// Nothing is hired on the strength of nothing.
func TestAWishTheCatalogueCannotAnswerHiresNobody(t *testing.T) {
	root, db := withCatalogue(t)

	if _, err := Hire(root, Roster(root), org.BuiltIn(), db, nil, Templates(root),
		Wish{Sentence: "someone who specialises in zxqvbn"}); err == nil {
		t.Error("somebody was hired for a word nothing in the catalogue knows")
	}

	entries, _ := os.ReadDir(Folder(root))

	if len(entries) != 0 {
		t.Errorf("files were written for a hire that did not happen: %d", len(entries))
	}
}

// A template is copied, not linked, and every one names a job that exists.
func TestEveryTemplateNamesARealJob(t *testing.T) {
	_, db := withCatalogue(t)

	for _, tpl := range BuiltInTemplates() {
		job, err := db.Occupation(tpl.Job)
		if err != nil || job == nil {
			t.Errorf("the %s template names a job that is not in the catalogue: %s", tpl.Name, tpl.Job)
		}

		if !validName.MatchString(tpl.Name) || tpl.For == "" {
			t.Errorf("the %s template could not be hired from", tpl.Name)
		}
	}

	if len(BuiltInTemplates()) != 16 {
		t.Errorf("%d templates, want the sixteen the specification names", len(BuiltInTemplates()))
	}
}

// Manner and second jobs survive the file, and a second job adds knowledge,
// never tools.
func TestMannerAndSecondJobsAreKeptAndOnlyAddKnowledge(t *testing.T) {
	root, db := withCatalogue(t)

	a := Agent{Name: "alex", Title: "Alex", For: "backend work", Job: "software.backend_engineer",
		Also:   []string{"software.software_architect"},
		Tools:  []string{"read_file"},
		Manner: Manner{Verbosity: "brief", RiskTolerance: "careful", Strengths: []string{"clear designs"}}}

	if err := Save(root, a); err != nil {
		t.Fatal(err)
	}

	Forget()

	back, _ := Find(Roster(root), "alex")

	if back.Manner.Verbosity != "brief" || len(back.Manner.Strengths) != 1 || len(back.Also) != 1 {
		t.Fatalf("the file lost the manner or the second job: %+v", back)
	}

	if told := back.Manner.Told(); !strings.Contains(told, "brief") || !strings.Contains(told, "careful") {
		t.Errorf("the manner is not said: %q", told)
	}

	plain := Settle(Agent{Name: "x", Job: "software.backend_engineer", Tools: []string{"read_file"}}, nil, db, nil)
	both := Settle(back, nil, db, nil)

	if len(both.Can) <= len(plain.Can) {
		t.Errorf("a second job added no knowledge: %d vs %d", len(both.Can), len(plain.Can))
	}

	if len(both.Tools) != 1 {
		t.Errorf("a second job widened what may be done: %v", both.Tools)
	}
}
