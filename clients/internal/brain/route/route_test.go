package route

import (
	"strings"
	"testing"
)

var here = []Candidate{
	{Name: "llama3.2:3b", Bytes: 2_000_000_000, Seconds: 4, ToolCall: "proper"},
	{Name: "qwen2.5-coder:7b", Bytes: 4_700_000_000, Seconds: 9, ToolCall: "proper"},
	{Name: "qwen2.5:14b", Bytes: 9_000_000_000, Seconds: 18, ToolCall: "proper"},
	{Name: "llava:7b", Bytes: 4_500_000_000, Seconds: 10, ToolCall: "none"},
	{Name: "claude-cloud", Cloud: true, Enabled: false, Paid: true},
}

// The routing table: what each kind of expert gets on one machine.
func TestEachExpertGetsTheModelItsWorkNeeds(t *testing.T) {
	cases := []struct {
		expert string
		needs  Needs
		want   string
	}{
		{"programmer", Needs{Coding: 3, Tools: true}, "qwen2.5-coder:7b"},
		{"strategist", Needs{Reasoning: 3, LongContext: 2}, "qwen2.5:14b"},
		{"voice", Needs{Speed: 3, Tools: true}, "llama3.2:3b"},
		{"illustrator", Needs{Vision: true}, "llava:7b"},
	}

	for _, c := range cases {
		got, err := Pick(c.expert, c.needs, here, Policy{})
		if err != nil {
			t.Fatalf("%s: %v", c.expert, err)
		}

		if got.Model != c.want {
			t.Errorf("%s got %s (%s), want %s", c.expert, got.Model, got.Reason, c.want)
		}

		if got.Reason == "" {
			t.Errorf("%s: a choice without a reason", c.expert)
		}
	}
}

func TestNothingPaidOrSwitchedOffAndNothingLeavesWhenOffline(t *testing.T) {
	cloud := []Candidate{
		{Name: "paid-cloud", Cloud: true, Enabled: true, Paid: true},
		{Name: "off-cloud", Cloud: true},
		{Name: "free-cloud:70b", Cloud: true, Enabled: true},
		{Name: "llama3.2:3b", Bytes: 2_000_000_000},
	}

	got, err := Pick("writer", Needs{Reasoning: 3}, cloud, Policy{})
	if err != nil {
		t.Fatal(err)
	}

	if got.Model != "free-cloud:70b" {
		t.Fatalf("got %s, want the enabled free cloud model", got.Model)
	}

	for _, f := range append([]string{got.Model}, got.Fallbacks...) {
		if f == "paid-cloud" || f == "off-cloud" {
			t.Fatalf("%s is in the order although it is paid or switched off", f)
		}
	}

	got, _ = Pick("writer", Needs{Reasoning: 3, Offline: true}, cloud, Policy{})
	if got.Model != "llama3.2:3b" || len(got.Fallbacks) != 0 {
		t.Fatalf("offline work got %s then %v", got.Model, got.Fallbacks)
	}
}

func TestAModelThatKeepsFailingFallsBehindAndTheNextOneIsUsed(t *testing.T) {
	failing := append([]Candidate{}, here...)
	failing[1].Failures = FailuresBeforeDemotion

	got, err := Pick("programmer", Needs{Coding: 3, Tools: true}, failing, Policy{})
	if err != nil {
		t.Fatal(err)
	}

	if got.Model == "qwen2.5-coder:7b" {
		t.Fatalf("the failing model is still first: %+v", got)
	}

	if got.Fallbacks[len(got.Fallbacks)-1] != "qwen2.5-coder:7b" {
		t.Fatalf("the failing model is not last in the fallbacks: %v", got.Fallbacks)
	}
}

func TestAPinWinsWhileItIsHereAndIsSaidWhenItIsNot(t *testing.T) {
	pol := Policy{Pins: map[string]string{"programmer": "qwen2.5:14b"}}

	got, _ := Pick("programmer", Needs{Coding: 3}, here, pol)
	if got.Model != "qwen2.5:14b" || !strings.Contains(got.Reason, "pinned") {
		t.Fatalf("pin ignored: %+v", got)
	}

	pol.Pins["programmer"] = "removed:32b"

	got, _ = Pick("programmer", Needs{Coding: 3}, here, pol)
	if got.Model != "qwen2.5-coder:7b" || !strings.Contains(got.Reason, "not available") {
		t.Fatalf("a missing pin was not explained: %+v", got)
	}
}

func TestWhatDoesNotFitInMemoryIsNotChosen(t *testing.T) {
	got, _ := Pick("strategist", Needs{Reasoning: 3}, here, Policy{MemoryBytes: 6_000_000_000})
	if got.Model == "qwen2.5:14b" {
		t.Fatalf("chose a model larger than memory: %+v", got)
	}
}

func TestNoModelIsAnErrorThatSaysWhy(t *testing.T) {
	_, err := Pick("illustrator", Needs{Vision: true}, here[:3], Policy{})
	if err == nil || !strings.Contains(err.Error(), "cannot see pictures") {
		t.Fatalf("got %v", err)
	}
}

func TestTraitsComeFromOllamaNames(t *testing.T) {
	if tr := TraitsOf("qwen2.5-coder:7b"); tr.Billions != 7 || !tr.Coding || !tr.LongContext {
		t.Errorf("qwen2.5-coder:7b read as %+v", tr)
	}

	if tr := TraitsOf("llama3.2-vision:11b"); tr.Billions != 11 || !tr.Vision {
		t.Errorf("llama3.2-vision:11b read as %+v", tr)
	}

	if tr := TraitsOf("phi3:3.8b"); tr.Billions != 3.8 {
		t.Errorf("phi3:3.8b read as %+v", tr)
	}
}

func TestOnAProcessorAloneTheBiggestModelIsNotChosenForEverything(t *testing.T) {
	got, _ := Pick("assistant", Needs{Reasoning: 1, Tools: true}, here, Policy{SpeedFloor: 1})
	if got.Model == "qwen2.5:14b" {
		t.Fatalf("a processor-only machine got the 14B model for everyday work: %+v", got)
	}

	got, _ = Pick("programmer", Needs{Coding: 3, Tools: true}, here, Policy{SpeedFloor: 1})
	if got.Model != "qwen2.5-coder:7b" {
		t.Fatalf("the programmer lost the coder on a processor-only machine: %+v", got)
	}
}
