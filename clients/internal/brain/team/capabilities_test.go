package team

import (
	"slices"
	"testing"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/store"
)

type someJobs map[string]*store.Occupation

func (j someJobs) Occupation(id string) (*store.Occupation, error) { return j[id], nil }

// a toolbox that answers from a fixed mapping, so the test is about the
// resolution order rather than about the real table.
type someTools map[string][]string

func (b someTools) ForCapabilities(capabilities []string) []string {
	out := []string{}

	for _, c := range capabilities {
		for _, tool := range b[c] {
			if !slices.Contains(out, tool) {
				out = append(out, tool)
			}
		}
	}

	slices.Sort(out)

	return out
}

func bench() ([]org.Unit, someJobs, someTools) {
	chart := []org.Unit{{
		Name: "engineering", Title: "Engineering", Kind: org.Department,
		Seats: []org.Position{{
			Name: "backend_engineer", Title: "Backend engineer",
			Job: "software.backend_engineer", Seniority: org.Senior,
			Needs: []string{"postgresql"},
			Never: []string{"send_email"},
		}, {
			Name: "bookkeeper", Title: "Bookkeeper", Job: "finance.bookkeeper",
		}},
	}}

	jobs := someJobs{
		"software.backend_engineer": {
			ID: "software.backend_engineer", Title: "Backend engineer",
			Needs: []store.Need{
				{ID: "performance", Essential: false},
				{ID: "api_design", Essential: true},
				{ID: "databases", Essential: true},
			},
		},
		"finance.bookkeeper": {
			ID: "finance.bookkeeper", Title: "Bookkeeper",
			Needs: []store.Need{{ID: "bookkeeping", Essential: true}},
		},
	}

	box := someTools{
		"api_design":  {"read_file", "write_file"},
		"databases":   {"read_file", "run_command"},
		"postgresql":  {"run_command"},
		"performance": {"run_command"},
		// bookkeeping deliberately maps to nothing, which is the ordinary
		// case for most of the taxonomy.
	}

	return chart, jobs, box
}

/*
 * What somebody is good at comes from three places, in one order.
 *
 * The job first, and within it what the job *is* before what it often also
 * involves; then what the post asks for on top; then what this one brings.
 * The order survives into the brief and into the shortlist, so it is not
 * cosmetic.
 */
func TestCapabilitiesComeFromTheJobTheSeatAndThePerson(t *testing.T) {
	chart, jobs, box := bench()

	fit := Settle(Agent{
		Name: "alex", Position: "backend_engineer", Can: []string{"laravel"},
	}, chart, jobs, box)

	want := []string{"api_design", "databases", "performance", "postgresql", "laravel"}

	if !slices.Equal(fit.Can, want) {
		t.Errorf("capabilities came out as %v, want %v", fit.Can, want)
	}

	if fit.Job == nil || fit.Job.Title != "Backend engineer" {
		t.Errorf("the job did not resolve: %+v", fit.Job)
	}

	if fit.Seat.Seniority != org.Senior || fit.Unit.Name != "engineering" {
		t.Errorf("the seat did not resolve: %+v", fit.Seat)
	}
}

// Two agents in the same seat share a job and differ in everything they were
// given. That is the whole reason a seat and a person are separate things.
func TestTwoAgentsInOneSeatShareAJobAndNotThemselves(t *testing.T) {
	chart, jobs, box := bench()

	alex := Settle(Agent{Name: "alex", Position: "backend_engineer"}, chart, jobs, box)
	maria := Settle(Agent{
		Name: "maria", Position: "backend_engineer", Can: []string{"security"},
	}, chart, jobs, box)

	if alex.Job != maria.Job {
		t.Error("two agents in one seat resolved to different jobs")
	}

	if slices.Contains(alex.Can, "security") {
		t.Error("what one agent was given leaked onto the other")
	}
}

/*
 * A list written on the agent beats one written on the seat, which beats what
 * its capabilities imply.
 *
 * Most specific first. The six lists that shipped were arrived at by watching
 * small models choose badly, and a list derived from a job definition is a
 * different list — so where somebody has said exactly what they mean, that is
 * what is used.
 */
func TestTheMostSpecificToolListWins(t *testing.T) {
	chart, jobs, box := bench()

	own := Settle(Agent{
		Name: "alex", Position: "backend_engineer", Tools: []string{"read_file"},
	}, chart, jobs, box)

	if !slices.Equal(own.Tools, []string{"read_file"}) {
		t.Errorf("the agent's own list was not used: %v", own.Tools)
	}

	derived := Settle(Agent{Name: "alex", Position: "backend_engineer"}, chart, jobs, box)

	if !slices.Equal(derived.Tools, []string{"read_file", "run_command", "write_file"}) {
		t.Errorf("the derived list came out as %v", derived.Tools)
	}
}

/*
 * An agent whose capabilities reach no tool gets none, not all of them.
 *
 * This is the trap the three states exist for. An empty list means
 * "everything" everywhere else in this program, so a bookkeeper — whose
 * capabilities genuinely map to no tool, like most of the taxonomy — would
 * otherwise have been quietly handed all sixty-six.
 */
func TestSomebodyWhoseWorkIsJudgementGetsNoToolsRatherThanAllOfThem(t *testing.T) {
	chart, jobs, box := bench()

	fit := Settle(Agent{Name: "bo", Position: "bookkeeper"}, chart, jobs, box)

	if !fit.Narrowed {
		t.Fatal("the bookkeeper was left unlimited")
	}

	if len(fit.Tools) != 0 {
		t.Fatalf("the bookkeeper was handed %v", fit.Tools)
	}

	only, withTools := fit.Only()

	if withTools {
		t.Error("a turn would have been offered tools")
	}

	if len(only) != 0 {
		t.Errorf("the turn would have been given %v", only)
	}
}

// And the generalist, which names nothing and holds a seat that names nothing,
// stays unlimited — the only way to get everything.
func TestTheGeneralistStaysUnlimited(t *testing.T) {
	fit := Settle(Agent{Name: "assistant"}, nil, nil, nil)

	if fit.Narrowed {
		t.Error("the generalist was narrowed by having nothing said about it")
	}

	only, withTools := fit.Only()

	if !withTools || only != nil {
		t.Errorf("the generalist would be run with %v / %v", only, withTools)
	}
}

// What a seat forbids and what an agent forbids are both subtracted, and
// neither can be widened by the other.
func TestWhatIsForbiddenAddsUp(t *testing.T) {
	chart, jobs, box := bench()

	fit := Settle(Agent{
		Name: "alex", Position: "backend_engineer", Never: []string{"run_command"},
	}, chart, jobs, box)

	for _, want := range []string{"send_email", "run_command"} {
		if !slices.Contains(fit.Never, want) {
			t.Errorf("%s is not forbidden: %v", want, fit.Never)
		}
	}

	if fit.Agent.Allows("run_command") {
		t.Error("an agent may use what it forbids itself")
	}
}

/*
 * A roster file written before there was an organisation still works.
 *
 * The single most important property of this whole change: an agent with no
 * position, no job and no capabilities is exactly what every agent was, and
 * resolves to exactly what it used to be.
 */
func TestAnAgentFromBeforeAllThisStillWorks(t *testing.T) {
	chart, jobs, box := bench()

	old := Agent{
		Name: "old_hand", Title: "Old hand", For: "whatever it used to do",
		Uses: UsesWork, Tools: []string{"read_file", "write_file"},
		Brief: "Do it the way it was always done.",
	}

	fit := Settle(old, chart, jobs, box)

	if !slices.Equal(fit.Tools, old.Tools) || !fit.Narrowed {
		t.Errorf("its tools came out as %v", fit.Tools)
	}

	if fit.Job != nil || fit.Seat.Name != "" || len(fit.Can) != 0 {
		t.Errorf("it was given an organisation it never asked for: %+v", fit)
	}

	if !old.Working() {
		t.Error("an agent with no state recorded is not working")
	}
}

// An agent may name a job directly, for the case the organisation has not
// caught up with — somebody hired to do a thing before there is a seat.
func TestAnAgentMayNameItsOwnJob(t *testing.T) {
	chart, jobs, box := bench()

	fit := Settle(Agent{
		Name: "temp", Position: "bookkeeper", Job: "software.backend_engineer",
	}, chart, jobs, box)

	if fit.Job == nil || fit.Job.ID != "software.backend_engineer" {
		t.Errorf("the agent's own job lost to its seat: %+v", fit.Job)
	}
}

// Suspended and retired agents are kept, and are not given work.
func TestWhoMayBeGivenWork(t *testing.T) {
	for _, c := range []struct {
		state string
		works bool
	}{
		{"", true}, {Active, true}, {Temporary, true},
		{Suspended, false}, {Retired, false},
	} {
		if got := (Agent{State: c.state}).Working(); got != c.works {
			t.Errorf("state %q works = %v", c.state, got)
		}
	}
}
