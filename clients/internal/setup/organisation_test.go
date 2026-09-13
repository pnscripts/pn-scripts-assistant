package setup

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/permits"
	"pn-scripts-assistant/internal/brain/team"
)

// The organisation as it ships holds together. If this fails, every new
// brain opens with an agent sitting nowhere, and setup would say so on the
// first run anybody ever does.
func TestTheOrganisationThatShipsHoldsTogether(t *testing.T) {
	got := holdsTogether(org.BuiltIn(), team.BuiltIn(), jobsThatShip())

	if got.State != "yes" {
		t.Fatalf("the built-in organisation does not hold together: %s", got.Note)
	}

	if !strings.Contains(got.Note, "jobs to hire from") {
		t.Errorf("the note does not say what can be hired: %q", got.Note)
	}
}

// A hand edit that renames a seat leaves an agent sitting nowhere. It keeps
// answering, routes on its one-line description alone, and nothing says why —
// unless this does.
func TestAnAgentInASeatThatIsNotThereIsNamed(t *testing.T) {
	roster := team.BuiltIn()
	roster[1].Position = "lead_developper"

	got := holdsTogether(org.BuiltIn(), roster, jobsThatShip())

	if got.State != "no" || !strings.Contains(got.Note, "lead_developper") {
		t.Fatalf("a seat that is not on the chart went unmentioned: %+v", got)
	}
}

func TestASeatNamingAnUnknownJobIsNamed(t *testing.T) {
	chart := org.BuiltIn()

	for i := range chart {
		for j := range chart[i].Seats {
			if chart[i].Seats[j].Name == "writer" {
				chart[i].Seats[j].Job = "media.wrtier"
			}
		}
	}

	got := holdsTogether(chart, team.BuiltIn(), jobsThatShip())

	if got.State != "no" || !strings.Contains(got.Note, "media.wrtier") {
		t.Fatalf("a seat with a job nobody knows went unmentioned: %+v", got)
	}
}

/*
 * The researcher's work is mostly the web, and on "ask me first" nothing
 * leaves. That is the switch working, not a fault — so it is a warning with
 * a name and a reason, never a red cross on the last step of setup.
 */
func TestTheResearcherIsHeldBackWhileNothingMayLeave(t *testing.T) {
	machine := here{
		freedom:  permits.AskEveryTime,
		godot:    func() bool { return true },
		pictures: func() bool { return true },
	}

	got := canWorkHere(org.BuiltIn(), team.BuiltIn(), jobsThatShip(), machine)

	if got.State != "skip" {
		t.Fatalf("state is %q, want the warning mark: %+v", got.State, got)
	}

	if !strings.Contains(got.Note, "Researcher") || !strings.Contains(got.Note, "web") {
		t.Errorf("the note does not say who, or why: %q", got.Note)
	}

	machine.freedom = permits.Everything

	if got := canWorkHere(org.BuiltIn(), team.BuiltIn(), jobsThatShip(), machine); got.State != "yes" {
		t.Errorf("with the web open and everything installed, somebody is still held back: %+v", got)
	}
}

func TestTheGameDeveloperIsHeldBackWithoutGodot(t *testing.T) {
	machine := here{
		freedom:  permits.Everything,
		godot:    func() bool { return false },
		pictures: func() bool { return true },
	}

	got := canWorkHere(org.BuiltIn(), team.BuiltIn(), jobsThatShip(), machine)

	if !strings.Contains(got.Note, "Game developer: Godot") {
		t.Fatalf("a game developer with no Godot went unmentioned: %+v", got)
	}

	// The generalist has every tool and is never the one named.
	if strings.Contains(got.Note, "Assistant") {
		t.Errorf("the generalist was named as held back: %q", got.Note)
	}
}

// Somebody retired is not on the chart for this purpose. A warning about a
// person who does not work here is a warning nobody can act on.
func TestNobodyRetiredIsNamed(t *testing.T) {
	roster := team.BuiltIn()

	for i := range roster {
		if roster[i].Name == "game_developer" {
			roster[i].State = team.Retired
		}
	}

	got := canWorkHere(org.BuiltIn(), roster, jobsThatShip(), here{
		freedom: permits.Everything, godot: func() bool { return false },
		pictures: func() bool { return true },
	})

	if got.State != "yes" {
		t.Fatalf("a retired agent was reported as held back: %+v", got)
	}
}

/*
 * The switch writes what leaves as well as what it asks, in one go.
 *
 * Written in two steps once, the file said private while the program was
 * open. Setup is the first place the switch can be thrown.
 */
func TestTheSwitchIsWrittenWithWhatLeaves(t *testing.T) {
	server, envPath := startServer(t)

	got := postJSON(t, server.URL()+"/freedom", `{"level":"everything"}`)

	if got["ok"] != true {
		t.Fatalf("the switch was not saved: %v", got)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"BRAIN_FREEDOM=everything", "BRAIN_PRIVACY=open"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the settings file does not say %s:\n%s", want, data)
		}
	}

	if state := getJSON(t, server.URL()+"/state"); state["freedom"] != "everything" {
		t.Errorf("setup reads the switch back as %v", state["freedom"])
	}

	postJSON(t, server.URL()+"/freedom", `{"level":"ask"}`)

	data, _ = os.ReadFile(envPath)

	if !strings.Contains(string(data), "BRAIN_PRIVACY=private") ||
		strings.Count(string(data), "BRAIN_FREEDOM=") != 1 {
		t.Errorf("turning it back did not replace both lines:\n%s", data)
	}

	if bad := postJSON(t, server.URL()+"/freedom", `{"level":"sometimes"}`); bad["ok"] != false {
		t.Errorf("an answer that is not one of the three was accepted: %v", bad)
	}
}
