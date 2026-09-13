package team

import (
	"fmt"
	"slices"
	"testing"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
)

func named(roster []Agent) []string {
	out := make([]string, 0, len(roster))

	for _, a := range roster {
		out = append(out, a.Name)
	}

	return out
}

/*
 * A shortlist stays short however many people there are.
 *
 * The property that makes an organisation allowed to grow. Three hundred
 * descriptions in every planning prompt is five to eight thousand tokens
 * before a word of the job is read, on a machine that reads at about ten
 * tokens a second.
 */
func TestAShortlistStaysShortHoweverManyPeopleThereAre(t *testing.T) {
	roster := BuiltIn()

	for i := range 500 {
		roster = append(roster, Agent{
			Name: fmt.Sprintf("specialist_%d", i), Title: "Specialist",
			For: fmt.Sprintf("subject %d and nothing whatever else", i),
		})
	}

	who := Who(roster, nil, nil, nil, Wanted{Doing: "write up what the code does"})

	if len(who) > Most {
		t.Errorf("%d people were put forward", len(who))
	}

	if !slices.Contains(named(who), "writer") {
		t.Errorf("the writer was not put forward: %v", named(who))
	}
}

/*
 * What somebody is good at counts for more than what their description says.
 *
 * A capability is a name chosen deliberately; a description is prose. The
 * agent that holds the job wins over the one whose sentence happens to share
 * a word with the request.
 */
func TestWhatSomebodyIsGoodAtOutweighsTheirDescription(t *testing.T) {
	chart := []org.Unit{{
		Name: "engineering", Title: "Engineering",
		Seats: []org.Position{{
			Name: "database_engineer", Title: "Database engineer",
			Job: "data.database_administrator",
		}},
	}}

	known := someJobs{"data.database_administrator": {
		ID: "data.database_administrator", Title: "Database administrator",
		Needs: []store.Need{{ID: "postgresql", Essential: true}},
	}}

	roster := []Agent{
		{Name: "assistant", Title: "Assistant", For: "anything"},
		{Name: "dana", Title: "Dana", For: "looking after data", Position: "database_engineer"},
		{Name: "pat", Title: "Pat", For: "postgresql things", Uses: UsesWork},
	}

	who := Who(roster, chart, known, nil, Wanted{
		Doing: "the database is slow", Needs: []string{"postgresql"},
	})

	if len(who) == 0 || who[0].Name != "dana" {
		t.Errorf("the shortlist was %v, want the one who holds the job first", named(who))
	}
}

// Suspended and retired people are kept on the roster and are not candidates
// for anything: what they did is still recorded against their name.
func TestSuspendedPeopleAreNotPutForward(t *testing.T) {
	roster := []Agent{
		{Name: "assistant", Title: "Assistant", For: "anything"},
		{Name: "gone", Title: "Gone", For: "writing and documents", State: Retired},
		{Name: "resting", Title: "Resting", For: "writing and documents", State: Suspended},
		{Name: "here", Title: "Here", For: "writing and documents", State: Active},
	}

	who := named(Who(roster, nil, nil, nil, Wanted{Doing: "write a document"}))

	if slices.Contains(who, "gone") || slices.Contains(who, "resting") {
		t.Errorf("somebody who does not work here was put forward: %v", who)
	}

	if !slices.Contains(who, "here") {
		t.Errorf("the one who does was not: %v", who)
	}
}

/*
 * Who knows a thing is a query, not a question for a model.
 *
 * Asking every agent would cost minutes and produce opinions. The roster, the
 * chart and the taxonomy already know, and they agree with each other.
 */
func TestWhoKnowsSomethingIsAnswearedFromTheData(t *testing.T) {
	chart := []org.Unit{{
		Name: "engineering",
		Seats: []org.Position{{
			Name: "backend_engineer", Job: "software.backend_engineer",
			Needs: []string{"postgresql"},
		}},
	}}

	known := someJobs{"software.backend_engineer": {
		ID: "software.backend_engineer", Needs: []store.Need{{ID: "api_design", Essential: true}},
	}}

	roster := []Agent{
		{Name: "alex", Position: "backend_engineer"},
		{Name: "sam", Can: []string{"postgresql"}},
		{Name: "jo", Can: []string{"copywriting"}},
		{Name: "old", Can: []string{"postgresql"}, State: Retired},
	}

	got := named(Knowing(roster, chart, known, nil, "postgresql"))

	if !slices.Equal(got, []string{"alex", "sam"}) {
		t.Errorf("who knows postgresql came back as %v", got)
	}
}

/*
 * Between two equally suited people, the record decides — and only then, and
 * only once it means something.
 */
func TestTheRecordBreaksTiesAndNothingElse(t *testing.T) {
	roster := []Agent{
		{Name: "first", Title: "First", For: "slow database queries"},
		{Name: "second", Title: "Second", For: "slow database queries"},
		{Name: "assistant", Title: "Assistant", For: "anything"},
	}

	want := Wanted{Doing: "fix the slow database queries", Record: map[string]store.AgentWork{
		"first":  {Name: "first", Steps: 10, Verified: 2},
		"second": {Name: "second", Steps: 10, Verified: 9},
	}}

	if got := Who(roster, nil, nil, nil, want); got[0].Name != "second" {
		t.Errorf("the better record did not break the tie: %s first", got[0].Name)
	}

	// Too little of a record to go on, and the order is left alone.
	want.Record["second"] = store.AgentWork{Name: "second", Steps: 2, Verified: 2}

	if got := Who(roster, nil, nil, nil, want); got[0].Name != "first" {
		t.Errorf("two steps of record reordered the shortlist: %s first", got[0].Name)
	}

	// And a record never outvotes being suited to the work.
	roster[1].For = "writing newsletters"
	want.Record["second"] = store.AgentWork{Name: "second", Steps: 50, Verified: 50}

	if got := Who(roster, nil, nil, nil, want); got[0].Name != "first" {
		t.Errorf("a perfect record outranked the person suited to the work: %s first", got[0].Name)
	}
}
