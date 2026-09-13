package org

import (
	"os"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/occupations"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(Path(root, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

/*
 * Every seat in the chart names a job that exists.
 *
 * A seat pointing at a job nothing describes is the worst kind of broken:
 * everything still works, the seat still shows, and the agent in it simply
 * arrives knowing nothing about what it is for. Silent, and only visible as
 * slightly worse answers.
 */
func TestEverySeatNamesAJobThatExists(t *testing.T) {
	jobs, _ := occupations.Seed()

	known := map[string]bool{}

	for _, job := range jobs {
		known[job.ID] = true
	}

	for _, seat := range Seats(BuiltIn()) {
		if !known[seat.Job] {
			t.Errorf("%s is filed as %q, which no job definition matches", seat.Name, seat.Job)
		}
	}
}

// And every capability a seat asks for on top of its job is one somebody has
// described, for the same reason the tools table is checked.
func TestEverySeatAsksForCapabilitiesThatExist(t *testing.T) {
	_, capabilities := occupations.Seed()

	known := map[string]bool{}

	for _, c := range capabilities {
		known[c.ID] = true
	}

	for _, seat := range Seats(BuiltIn()) {
		for _, need := range seat.Needs {
			if !known[need] {
				t.Errorf("%s asks for %q, which nothing describes", seat.Name, need)
			}
		}
	}
}

// A seat with no job could never be filled, so it is refused at the door
// rather than written and discovered later.
func TestASeatWithNoJobIsRefused(t *testing.T) {
	root := t.TempDir()

	err := Save(root, Unit{
		Name: "engineering", Title: "Engineering",
		Seats: []Position{{Name: "somebody", Title: "Somebody"}},
	})

	if err == nil {
		t.Fatal("a seat with no job was saved")
	}

	if !strings.Contains(err.Error(), "no job") {
		t.Errorf("the refusal said %q", err)
	}
}

/*
 * What is written and what is read back are the same thing.
 *
 * The file is the record, so a unit that survived being saved but lost its
 * seats would be a chart that quietly emptied itself every time somebody
 * used the interface.
 */
func TestAUnitSurvivesBeingWrittenAndReadBack(t *testing.T) {
	root := t.TempDir()

	want := Unit{
		Name: "support", Title: "Support", Kind: Team, Parent: "practice",
		Purpose: "Getting somebody working again.",
		Seats: []Position{
			{
				Name: "first_line", Title: "First line",
				Job: "support.technical_support_specialist", Seniority: Junior,
				ReportsTo: "assistant",
				Needs:     []string{"troubleshooting", "machine_upkeep"},
				Never:     []string{"run_command"},
			},
		},
	}

	if err := Save(root, want); err != nil {
		t.Fatal(err)
	}

	got, ok := Find(Chart(root), "support")
	if !ok {
		t.Fatal("the unit was not read back")
	}

	if got.Title != want.Title || got.Kind != want.Kind || got.Parent != want.Parent {
		t.Errorf("the unit came back as %+v", got)
	}

	if got.Purpose != want.Purpose {
		t.Errorf("the purpose came back as %q", got.Purpose)
	}

	if len(got.Seats) != 1 {
		t.Fatalf("%d seats came back", len(got.Seats))
	}

	seat := got.Seats[0]

	if seat.Job != want.Seats[0].Job || seat.Seniority != Junior || seat.ReportsTo != "assistant" {
		t.Errorf("the seat came back as %+v", seat)
	}

	if len(seat.Needs) != 2 || len(seat.Never) != 1 || seat.Never[0] != "run_command" {
		t.Errorf("the seat's limits came back as %+v", seat)
	}
}

// A file naming a unit that shipped replaces it; one with a new name adds to
// the chart. The same rule the roster uses, so nobody has to learn a second.
func TestAFileReplacesWhatShippedOrAddsToIt(t *testing.T) {
	root := t.TempDir()

	write(t, root, "engineering", "title: The workshop\nkind: team\nparent: practice\n\nWhat we actually do.\n")
	write(t, root, "legal", "title: Legal\nparent: practice\n")

	chart := Chart(root)

	mine, ok := Find(chart, "engineering")
	if !ok {
		t.Fatal("engineering disappeared")
	}

	if mine.Title != "The workshop" || mine.Kind != Team {
		t.Errorf("the built-in was not replaced: %+v", mine)
	}

	if mine.BuiltIn {
		t.Error("a unit somebody edited still says it shipped")
	}

	if len(mine.Seats) != 0 {
		t.Error("the replaced unit kept the seats of the one it replaced")
	}

	if _, ok := Find(chart, "legal"); !ok {
		t.Error("a new unit was not added")
	}

	if _, ok := Find(chart, "studio"); !ok {
		t.Error("an untouched unit disappeared")
	}
}

/*
 * Two units naming each other cost a walk, not the program.
 *
 * And neither disappears. A cycle is somebody mid-edit, and the interface
 * going blank — or worse, hanging — is a far bigger problem than a chart
 * drawn slightly wrongly for a minute.
 */
func TestACycleDoesNotHangAndNothingDisappears(t *testing.T) {
	root := t.TempDir()

	write(t, root, "one", "title: One\nparent: two\n")
	write(t, root, "two", "title: Two\nparent: one\n")

	chart := Chart(root)

	done := make(chan []Unit, 1)

	go func() { done <- Shape(chart) }()

	var shape []Unit

	select {
	case shape = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drawing the chart did not finish")
	}

	if len(shape) != len(chart) {
		t.Errorf("the chart lost %d units to a cycle", len(chart)-len(shape))
	}

	depths := DepthOf(chart)

	if _, ok := depths["one"]; !ok {
		t.Error("a unit in a cycle has no depth at all")
	}
}

// A unit whose parent was renamed still appears, at the top rather than
// nowhere. The file is still on disk and the seats in it still exist.
func TestAUnitWhoseParentIsGoneStillAppears(t *testing.T) {
	root := t.TempDir()

	write(t, root, "orphan", "title: Orphan\nparent: a_department_that_was_renamed\n")

	shape := Shape(Chart(root))

	for _, u := range shape {
		if u.Name == "orphan" {
			if got := DepthOf(Chart(root))["orphan"]; got != 0 {
				t.Errorf("the orphan is drawn at depth %d", got)
			}

			return
		}
	}

	t.Error("a unit whose parent is gone disappeared from the chart")
}

// Reporting lines cross units, which is why positions are named across the
// whole organisation rather than within one.
func TestReportingLinesCrossUnits(t *testing.T) {
	chart := BuiltIn()

	if under := Manages(chart, "lead_developer"); len(under) != 2 {
		t.Errorf("the lead developer manages %d seats", len(under))
	}

	up := AnswersTo(chart, "backend_engineer")

	if len(up) != 1 || up[0].Name != "lead_developer" {
		t.Errorf("the backend engineer answers to %+v", up)
	}

	// And most seats answer to the owner, which is a person and not a seat.
	if up := AnswersTo(chart, "assistant"); len(up) != 0 {
		t.Errorf("the assistant answers to %+v, want nobody", up)
	}
}

// Parents come before their children, so the chart reads as a chart.
func TestTheChartReadsFromTheTopDown(t *testing.T) {
	shape := Shape(BuiltIn())

	if len(shape) == 0 || shape[0].Name != "practice" {
		t.Fatalf("the chart starts at %+v", shape)
	}

	depths := DepthOf(BuiltIn())

	for _, u := range shape[1:] {
		if depths[u.Name] != 1 {
			t.Errorf("%s is drawn at depth %d", u.Name, depths[u.Name])
		}
	}
}
