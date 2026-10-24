package team

import (
	"testing"

	"pn-scripts-assistant/internal/brain/route"
)

type routeNeeds = route.Needs

func TestEachAgentSaysWhatItsWorkNeedsOfAModel(t *testing.T) {
	cases := []struct {
		agent Agent
		check func(n routeNeeds) bool
	}{
		{Agent{Title: "Game developer", Uses: UsesWork}, func(n routeNeeds) bool { return n.Coding == 3 && n.Tools }},
		{Agent{Title: "Програмист", Uses: UsesBest}, func(n routeNeeds) bool { return n.Coding == 3 && n.Reasoning == 2 }},
		{Agent{Title: "Receptionist", Uses: UsesQuick}, func(n routeNeeds) bool { return n.Speed == 3 && n.Coding == 0 }},
		{Agent{Title: "Strategist", Uses: UsesTalk, Needs: Needs{Thinks: true, Local: true}}, func(n routeNeeds) bool { return n.Reasoning == 3 && n.Offline }},
		{Agent{Title: "Illustrator", Needs: Needs{Vision: true}}, func(n routeNeeds) bool { return n.Vision }},
	}

	for _, c := range cases {
		if n := c.agent.RouteNeeds(); !c.check(n) {
			t.Errorf("%s: %+v", c.agent.Title, n)
		}
	}
}

// Work outside software goes to the specialist of its field.
func TestWorkOutsideSoftwareGoesToItsSpecialist(t *testing.T) {
	for instruction, want := range map[string]string{
		"analyse the competitors and write the business strategy for the market": "business_strategist",
		"build the budget forecast with costs and revenue":                       "finance_analyst",
		"check the VAT on these invoices":                                        "accountant",
		"review the contract terms and the privacy obligations":                  "legal_advisor",
		"plan the launch campaign and social posts for the audience":             "marketing_specialist",
		"compose music and jingles for the trailer":                              "music_producer",
	} {
		if got := For(BuiltIn(), "", "do", instruction); got.Name != want {
			t.Errorf("%q went to %s, want %s", instruction, got.Name, want)
		}
	}
}
