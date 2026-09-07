package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/places"
)

/*
 * Being told to learn everything, and doing it.
 *
 * The complaint this answers is "it is not doing what I tell it". Asked to
 * learn everything, the brain replied with a question — which folder — while
 * holding a list of two drives it had found by itself. It was right that it
 * had nowhere on its list to start; it was wrong to make somebody go and type
 * a path it could already see.
 */
func withCandidates(t *testing.T, found ...Candidate) Places {
	t.Helper()

	return Places{
		Root:       t.TempDir(),
		Owner:      "Petar",
		Candidates: func() []Candidate { return found },
	}
}

func run(t *testing.T, tool Places, args string) string {
	t.Helper()

	said, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("running the tool: %v", err)
	}

	return said
}

func TestToldToLearnEverythingItStartsOnWhatItCanSee(t *testing.T) {
	// Places that actually exist, since taking one on checks that it is there.
	first, second := t.TempDir(), t.TempDir()

	tool := withCandidates(t,
		Candidate{Path: first, Name: "your home folder", Home: true},
		Candidate{Path: second, Name: "work"},
	)

	said := run(t, tool, `{"start_learning":"everything"}`)

	for _, want := range []string{first, second, "Started on"} {
		if !strings.Contains(said, want) {
			t.Errorf("did not start on %q:\n%s", want, said)
		}
	}

	// And they are actually on the list afterwards, not merely described.
	list, err := places.List(tool.Root)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 2 {
		t.Fatalf("watching %d places, want 2", len(list))
	}
}

// Named one by one as well, since "learn my work drive" is the commoner
// instruction than "learn everything".
func TestItStartsOnOneNamedPlace(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()

	tool := withCandidates(t,
		Candidate{Path: home, Name: "your home folder"},
		Candidate{Path: work, Name: "work"},
	)

	said := run(t, tool, `{"start_learning":"work"}`)

	if !strings.Contains(said, work) {
		t.Fatalf("did not start on the named drive:\n%s", said)
	}

	if strings.Contains(said, home) {
		t.Fatalf("started on somewhere that was not asked for:\n%s", said)
	}
}

/*
 * Only from the list it assembled itself.
 *
 * This is the whole safety argument. Choosing what gets read used to be
 * something somebody typed, because a misheard word is a plausible path — and
 * a recogniser that turns "Brain" into "brainlue" will happily turn a sentence
 * into a folder. A misheard word is not a plausible entry in a list the brain
 * built by looking at the machine.
 */
func TestItWillNotStartOnSomewhereItDidNotFind(t *testing.T) {
	home := t.TempDir()

	tool := withCandidates(t, Candidate{Path: home, Name: "your home folder"})

	said := run(t, tool, `{"start_learning":"/etc/shadow and everything under it"}`)

	if strings.Contains(said, "Started on") {
		t.Fatalf("started on a path nobody offered:\n%s", said)
	}

	if !strings.Contains(said, home) {
		t.Fatalf("did not say what it can actually see:\n%s", said)
	}

	if list, _ := places.List(tool.Root); len(list) != 0 {
		t.Fatalf("added %d places from an invented path", len(list))
	}
}

/*
 * With nothing on the list, it says what it could start on rather than sending
 * somebody to a panel.
 *
 * "One can be added in the storage panel" is an assistant telling somebody to
 * go and do it themselves, and it was said in answer to "learn everything"
 * with two drives sitting in front of it.
 */
func TestWithNothingWatchedItOffersWhatItFound(t *testing.T) {
	tool := withCandidates(t,
		Candidate{Path: "/home/petar", Name: "your home folder"},
		Candidate{Path: "/media/petar/work", Name: "work"},
	)

	said := run(t, tool, `{}`)

	for _, want := range []string{"your home folder", "work", "say everything"} {
		if !strings.Contains(said, want) {
			t.Errorf("the offer is missing %q:\n%s", want, said)
		}
	}
}

// A machine where it can see nothing at all still has to answer.
func TestWithNothingToSeeItSaysSo(t *testing.T) {
	tool := withCandidates(t)

	if said := run(t, tool, `{}`); !strings.Contains(said, "storage panel") {
		t.Fatalf("gave no way forward at all:\n%s", said)
	}
}

// Asking twice is not an error: the second time it is already on the list.
func TestStartingTwiceIsNotAFailure(t *testing.T) {
	tool := withCandidates(t, Candidate{Path: t.TempDir(), Name: "your home folder"})

	run(t, tool, `{"start_learning":"everything"}`)

	said := run(t, tool, `{"start_learning":"everything"}`)

	if !strings.Contains(said, "already") {
		t.Fatalf("the second time should say it is already on the list:\n%s", said)
	}
}

/*
 * It asks before taking on a drive.
 *
 * Everything in a folder becomes something the brain knows, and that is a
 * decision about somebody's data made on the strength of a sentence heard
 * across a room. The approval prompt names the places in full.
 */
func TestTakingOnADriveWaitsToBeAgreed(t *testing.T) {
	if got := (Places{}).Risk(); got != Mutating {
		t.Fatalf("taking on a whole drive should ask first, got %v", got)
	}

	summary := Places{}.Summarize(json.RawMessage(`{"start_learning":"everything"}`))

	if !strings.Contains(summary, "every drive and folder") {
		t.Fatalf("the prompt does not say what it will take on: %q", summary)
	}
}

// Reporting is still what it does when asked nothing in particular.
func TestAskingNothingStillReports(t *testing.T) {
	root := t.TempDir()

	if _, err := places.Watch(root, filepath.Dir(root), "somewhere", places.Both); err != nil {
		t.Skip("cannot watch a temporary folder here")
	}

	tool := Places{Root: root, Owner: "Petar"}

	if said := run(t, tool, `{}`); !strings.Contains(said, "somewhere") {
		t.Fatalf("did not report the place it watches:\n%s", said)
	}
}
