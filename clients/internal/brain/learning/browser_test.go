package learning

import (
	"strings"
	"testing"
)

// Every per-site fact is worded almost identically, so they are all
// near-neighbours in embedding space and "which do I use most" recalls an
// arbitrary handful. The ranking is a property of the whole set, so it needs to
// live in one fact.
func TestSitesIncludeARankedSummary(t *testing.T) {
	sites := []Site{
		{Domain: "pnscripts.local", Visits: 1160},
		{Domain: "staging.tools.swytchbike.com", Visits: 651},
		{Domain: "www.youtube.com", Visits: 282},
		{Domain: "onchebonche.bg", Visits: 26},
	}

	obs := FromSites(sites, "Petar")

	if len(obs) != len(sites)+1 {
		t.Fatalf("got %d observations, want %d", len(obs), len(sites)+1)
	}

	summary := obs[0]

	if summary.Source != "browser:__ranking__" {
		t.Fatalf("first observation is not the summary: %+v", summary)
	}

	// The order in the sentence is what the model reads off.
	positions := make([]int, len(sites))

	for i, s := range sites {
		positions[i] = strings.Index(summary.Content, s.Domain)

		if positions[i] < 0 {
			t.Fatalf("%s missing from the summary: %q", s.Domain, summary.Content)
		}
	}

	for i := 1; i < len(positions); i++ {
		if positions[i] < positions[i-1] {
			t.Errorf("%s appears before %s despite fewer visits",
				sites[i].Domain, sites[i-1].Domain)
		}
	}

	// The counts have to be there, or the model has nothing to rank by if it
	// recalls the summary alongside individual sites.
	if !strings.Contains(summary.Content, "1160") {
		t.Errorf("visit counts missing: %q", summary.Content)
	}
}

func TestNoSitesMeansNoSummary(t *testing.T) {
	if obs := FromSites(nil, "Petar"); len(obs) != 0 {
		t.Errorf("got %d observations for no sites", len(obs))
	}
}

func TestSummaryIsBoundedToTheTopFew(t *testing.T) {
	var many []Site

	for i := 0; i < 40; i++ {
		many = append(many, Site{Domain: "site", Visits: 100 - i})
	}

	summary := FromSites(many, "Petar")[0].Content

	if got := strings.Count(summary, "("); got > TopSitesInSummary {
		t.Errorf("summary lists %d sites, want at most %d", got, TopSitesInSummary)
	}
}

func TestReverseHostUndoesFirefoxEncoding(t *testing.T) {
	// Built by reversing the expected answer, rather than typed by hand — the
	// hand-written version of this test was wrong, and a test that asserts a
	// mistyped constant fails against correct code.
	for _, want := range []string{
		"www.example.com",
		"onchebonche.bg",
		"pnscripts.local",
		"staging.tools.swytchbike.com",
	} {
		runes := []rune(want)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}

		encoded := string(runes) + "."

		if got := reverseHost(encoded); got != want {
			t.Errorf("reverseHost(%q) = %q, want %q", encoded, got, want)
		}
	}
}

// The URL must never survive; only its host.
func TestDomainOfKeepsOnlyTheHost(t *testing.T) {
	cases := map[string]string{
		"https://github.com/PNScripts/pn-brain/issues/1?x=2#y": "github.com",
		"http://user:pass@internal.host:8080/admin":            "internal.host",
		"https://Example.COM/":                                 "example.com",
	}

	for in, want := range cases {
		got := domainOf(in)

		if got != want {
			t.Errorf("domainOf(%q) = %q, want %q", in, got, want)
		}

		if strings.Contains(got, "/") || strings.Contains(got, "?") {
			t.Errorf("path or query survived: %q", got)
		}
	}
}
