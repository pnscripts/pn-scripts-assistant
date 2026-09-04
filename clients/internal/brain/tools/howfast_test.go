package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func asked(t *testing.T, s Speed) string {
	t.Helper()

	said, err := HowFast{Reading: func() Speed { return s }}.
		Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("asking how fast: %v", err)
	}

	return said
}

/*
 * The account has to name the stage that is actually costing the time.
 *
 * "It is slow" is where everybody starts, and the four waits behind it have
 * four different answers. An account that recites all four equally is the same
 * as no account: whichever one matters is buried among three that do not.
 */
func TestItNamesTheStageThatIsCostingTheTime(t *testing.T) {
	said := asked(t, Speed{
		Over: 6, Verdict: "About 44.4s from your last word to my first sound.",
		Hearing: 800, Thinking: 42000, Writing: 900, Speaking: 700,
		Model: "qwen3:8b",
		Findings: []Finding{{
			Stage: "thinking", Costs: 42000,
			Because: "qwen3:8b runs on this machine's processor",
			Change:  "install a small model for conversation",
		}},
	})

	for _, want := range []string{"42.0s", "the model thinking", "qwen3:8b", "small model"} {
		if !strings.Contains(said, want) {
			t.Errorf("the account is missing %q:\n%s", want, said)
		}
	}
}

// A stage costing nothing is not information. Reciting it teaches somebody to
// stop reading the list.
func TestStagesTooSmallToMatterAreLeftOut(t *testing.T) {
	said := asked(t, Speed{
		Over: 8, Verdict: "quick", Hearing: 12, Thinking: 400, Writing: 8, Speaking: 20,
	})

	if strings.Contains(said, "12ms") || strings.Contains(said, "8ms") {
		t.Fatalf("recited stages that cost nothing:\n%s", said)
	}

	if !strings.Contains(said, "400ms") {
		t.Fatalf("left out the one that did cost something:\n%s", said)
	}
}

/*
 * Having no small model installed is the fact, on a machine like this one.
 *
 * It is the single largest thing somebody could change here, and until this
 * was measured the program had never once mentioned it.
 */
func TestItSaysWhenEveryGreetingGoesToTheLargeModel(t *testing.T) {
	said := asked(t, Speed{Over: 4, Verdict: "slow", Thinking: 30000, Model: "qwen3:8b"})

	if !strings.Contains(said, "no small model installed") {
		t.Fatalf("did not say that everything goes to the large model:\n%s", said)
	}
}

func TestItSaysWhichModelHandlesConversationWhenThereIsOne(t *testing.T) {
	said := asked(t, Speed{
		Over: 4, Verdict: "fine", Thinking: 900,
		Model: "qwen3:8b", Quick: "llama3.2:3b",
	})

	if !strings.Contains(said, "Conversation goes to llama3.2:3b") {
		t.Fatalf("did not name the conversational model:\n%s", said)
	}

	if strings.Contains(said, "no small model") {
		t.Fatalf("claimed there was none:\n%s", said)
	}
}

// Asked before anything has been timed, it says so — rather than reporting a
// row of zeroes, which reads as "instant".
func TestNothingTimedYetIsSaidPlainly(t *testing.T) {
	said := asked(t, Speed{})

	if !strings.Contains(said, "Nothing has been timed yet") {
		t.Fatalf("reported an untimed brain as fast:\n%s", said)
	}

	if strings.Contains(said, "0ms") {
		t.Fatalf("printed zeroes as measurements:\n%s", said)
	}
}

func TestAskingHowFastChangesNothing(t *testing.T) {
	if got := (HowFast{}).Risk(); got != Safe {
		t.Fatalf("reading a stopwatch should be safe, got %v", got)
	}
}

func TestWithNothingTimingItSaysSo(t *testing.T) {
	if _, err := (HowFast{}).Execute(context.Background(), nil); err == nil {
		t.Fatal("with no timings wired up it should say so, not report zero")
	}
}

func TestTheSpeedToolDescribesItselfToTheModel(t *testing.T) {
	var shape map[string]any

	if err := json.Unmarshal(HowFast{}.Parameters(), &shape); err != nil {
		t.Fatalf("unreadable parameters: %v", err)
	}

	if shape["type"] != "object" {
		t.Fatal("the parameters are not an object")
	}

	// The words somebody actually uses, so the model reaches for this rather
	// than apologising in prose.
	for _, phrase := range []string{"slow", "how fast", "real time"} {
		if !strings.Contains(strings.ToLower(HowFast{}.Description()), phrase) {
			t.Errorf("the description does not cover %q", phrase)
		}
	}
}

// Milliseconds read the way somebody says them.
func TestDurationsAreReadableAtBothEnds(t *testing.T) {
	for _, c := range []struct {
		ms   int
		want string
	}{
		{40, "40ms"}, {999, "999ms"}, {1000, "1.0s"}, {44400, "44.4s"},
	} {
		if got := milliseconds(c.ms); got != c.want {
			t.Errorf("%dms read as %q, want %q", c.ms, got, c.want)
		}
	}
}
