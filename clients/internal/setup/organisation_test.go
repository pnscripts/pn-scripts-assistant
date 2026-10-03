package setup

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/org"
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
 * The researcher's work is mostly the web, and with privacy private nothing
 * leaves. That is the setting working, not a fault — so it is a warning with
 * a name and a reason, never a red cross on the last step of setup.
 */
func TestTheResearcherIsHeldBackWhileNothingMayLeave(t *testing.T) {
	machine := here{
		privacy:  llm.ModePrivate,
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

	machine.privacy = llm.ModeResearch

	if got := canWorkHere(org.BuiltIn(), team.BuiltIn(), jobsThatShip(), machine); got.State != "yes" {
		t.Errorf("with the web open and everything installed, somebody is still held back: %+v", got)
	}
}

func TestTheGameDeveloperIsHeldBackWithoutGodot(t *testing.T) {
	machine := here{
		privacy:  llm.ModeResearch,
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
		privacy: llm.ModeResearch, godot: func() bool { return false },
		pictures: func() bool { return true },
	})

	if got.State != "yes" {
		t.Fatalf("a retired agent was reported as held back: %+v", got)
	}
}

/*
 * Setup writes how much it asks, and leaves what may leave alone.
 *
 * It used to write privacy from the same answer, so "never stop" here opened
 * privacy and "ask me" closed it. They are two settings, and the one setup
 * asks about is the only one it changes.
 */
func TestTheSwitchLeavesPrivacyAlone(t *testing.T) {
	server, envPath := startServer(t)

	before, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(envPath, append(before, "BRAIN_PRIVACY=research\n"...), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, level := range []string{"everything", "ask", "granted"} {
		got := postJSON(t, server.URL()+"/freedom", `{"level":"`+level+`"}`)

		if got["ok"] != true {
			t.Fatalf("%s was not saved: %v", level, got)
		}

		data, err := os.ReadFile(envPath)
		if err != nil {
			t.Fatal(err)
		}

		text := string(data)

		if !strings.Contains(text, "BRAIN_FREEDOM="+level) || strings.Count(text, "BRAIN_FREEDOM=") != 1 {
			t.Errorf("after choosing %s the settings file says:\n%s", level, text)
		}

		if !strings.Contains(text, "BRAIN_PRIVACY=research") || strings.Count(text, "BRAIN_PRIVACY=") != 1 {
			t.Errorf("choosing %s changed privacy:\n%s", level, text)
		}

		if state := getJSON(t, server.URL()+"/state"); state["freedom"] != level {
			t.Errorf("setup reads %s back as %v", level, state["freedom"])
		}
	}

	if bad := postJSON(t, server.URL()+"/freedom", `{"level":"sometimes"}`); bad["ok"] != false {
		t.Errorf("an answer that is not one of the three was accepted: %v", bad)
	}
}

// Privacy is chosen on its own in setup, and choosing it leaves how much it
// asks alone.
func TestPrivacyIsItsOwnChoiceInSetup(t *testing.T) {
	server, envPath := startServer(t)

	postJSON(t, server.URL()+"/freedom", `{"level":"ask"}`)

	state := getJSON(t, server.URL()+"/state")

	if state["privacy"] != "private" {
		t.Errorf("a new setup reads privacy as %v, want private", state["privacy"])
	}

	if offered, _ := state["privacies"].([]any); len(offered) != 3 {
		t.Errorf("setup offers %d privacy choices, want 3: %v", len(offered), state["privacies"])
	}

	for _, mode := range []string{"open", "research", "private"} {
		if got := postJSON(t, server.URL()+"/privacy", `{"mode":"`+mode+`"}`); got["ok"] != true {
			t.Fatalf("%s was not saved: %v", mode, got)
		}

		data, _ := os.ReadFile(envPath)
		text := string(data)

		if !strings.Contains(text, "BRAIN_PRIVACY="+mode) || strings.Count(text, "BRAIN_PRIVACY=") != 1 {
			t.Errorf("after choosing %s the settings file says:\n%s", mode, text)
		}

		if !strings.Contains(text, "BRAIN_FREEDOM=ask") {
			t.Errorf("choosing privacy %s changed how much it asks:\n%s", mode, text)
		}

		if state := getJSON(t, server.URL()+"/state"); state["privacy"] != mode || state["freedom"] != "ask" {
			t.Errorf("after %s setup reads privacy %v, freedom %v", mode, state["privacy"], state["freedom"])
		}
	}

	if bad := postJSON(t, server.URL()+"/privacy", `{"mode":"public"}`); bad["ok"] != false {
		t.Errorf("a privacy that is not one of the three was accepted: %v", bad)
	}
}
