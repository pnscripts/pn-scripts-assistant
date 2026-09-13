package tools

import (
	"slices"
	"testing"

	"pn-scripts-assistant/internal/brain/occupations"
)

type servingFake struct {
	fake
	serves []string
}

func (s servingFake) Serves() []string { return s.serves }
func (servingFake) AddedAtRunTime()    {}

/*
 * Every capability a tool claims to serve is one somebody has described.
 *
 * Without this the failure is silent and slow. A tool says it serves
 * "postgres", nothing in the taxonomy is called that, and from then on nobody
 * good at PostgreSQL is handed the tool — which looks exactly like the tool
 * not being useful, rather than like a spelling mistake.
 */
func TestEveryCapabilityAToolServesIsDescribed(t *testing.T) {
	_, described := occupations.Seed()

	known := map[string]bool{}

	for _, c := range described {
		known[c.ID] = true
	}

	for _, c := range EveryCapabilityServed() {
		if !known[c] {
			t.Errorf("a tool serves %q, which nothing describes", c)
		}
	}
}

/*
 * Capabilities narrow, and the narrowing is the point.
 *
 * A researcher handed the three tools that reach the web and not the one that
 * runs commands cannot be talked into running a command — not by a page it
 * reads, not by a clever instruction. That is a stronger guarantee than
 * asking it nicely in a prompt, and it is the whole reason for deriving tools
 * from what somebody is good at.
 */
func TestWhatSomebodyIsGoodAtDecidesWhatTheyAreHanded(t *testing.T) {
	r := NewRegistry(
		fake{name: "web_search"}, fake{name: "fetch_url"}, fake{name: "read_a_page"},
		fake{name: "run_command"}, fake{name: "read_file"}, fake{name: "make_a_picture"},
		fake{name: "ask_first"},
	)

	got := r.ForCapabilities([]string{"web_research", "research"})

	for _, want := range []string{"web_search", "fetch_url", "read_a_page", "read_file"} {
		if !slices.Contains(got, want) {
			t.Errorf("a researcher was not handed %s: %v", want, got)
		}
	}

	for _, unwanted := range []string{"run_command", "make_a_picture"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("a researcher was handed %s", unwanted)
		}
	}
}

/*
 * A tool that belongs to no profession is never handed out by capability.
 *
 * ask_first is how an agent behaves rather than something it is good at, and
 * decide_waiting is the owner's own decision. Neither should arrive because
 * somebody was hired for being good at research.
 */
func TestToolsThatBelongToNoProfessionAreNeverHandedOut(t *testing.T) {
	r := NewRegistry(
		fake{name: "ask_first"}, fake{name: "decide_waiting"}, fake{name: "list_waiting"},
		fake{name: "remember_how_to_do_this"}, fake{name: "do_in_background"},
	)

	for _, c := range EveryCapabilityServed() {
		if got := r.ForCapabilities([]string{c}); len(got) > 0 {
			t.Errorf("being good at %q handed out %v", c, got)
		}
	}
}

// Something added at run time says what it is for, and is believed. The table
// cannot know about a skill written this morning.
func TestSomethingAddedLaterSaysWhatItIsFor(t *testing.T) {
	r := NewRegistry(fake{name: "read_file"})

	if err := r.Register(servingFake{
		fake:   fake{name: "file_the_invoices"},
		serves: []string{"invoicing", "bookkeeping"},
	}); err != nil {
		t.Fatal(err)
	}

	got := r.ForCapabilities([]string{"bookkeeping"})

	if !slices.Contains(got, "file_the_invoices") {
		t.Errorf("a skill that said what it was for was not handed out: %v", got)
	}

	if slices.Contains(got, "read_file") {
		t.Errorf("bookkeeping was handed read_file: %v", got)
	}
}

// Asking for nothing gets nothing, rather than everything. An agent with no
// capabilities recorded is not an agent that may do anything it likes.
func TestNoCapabilitiesMeansNoToolsRatherThanAllOfThem(t *testing.T) {
	r := NewRegistry(fake{name: "read_file"}, fake{name: "run_command"})

	if got := r.ForCapabilities(nil); len(got) != 0 {
		t.Errorf("an empty list handed out %v", got)
	}
}
