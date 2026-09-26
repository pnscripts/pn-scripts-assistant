package brain

import (
	"strings"
	"testing"
	"time"
)

/*
 * The greeting is decided from what is true, not reworded from a sentence.
 *
 * Its owner noticed the fault before this test existed: "the program is
 * always telling/speaking one phrase in the beginning". It was — the model
 * was handed finished English ("We have not met properly: ask me what I can
 * do…") and asked to put it in its own words, and a model given a finished
 * sentence hands that sentence back with the words moved about.
 */
func TestTheGreetingIsMadeOfFactsRatherThanSentences(t *testing.T) {
	b := &Brain{}
	b.Cfg.Owner = "Petar"

	facts := b.about(nil, false, true)

	if facts["who you are greeting"] != "Petar" {
		t.Errorf("it does not know who it is greeting: %v", facts)
	}

	if facts["you have never met them before"] != true {
		t.Error("a first meeting is not in the facts")
	}

	// Nothing in it is a finished sentence for the model to parrot.
	for key, value := range facts {
		switch key {
		case "what you can do here",
			// A block of headings and counts, not prose — except on a brain
			// with nothing loaded, where the honest answer is a sentence.
			"something that has just happened to this brain":
			// Carefully worded already: being wrong about a brain that has
			// been carried to another machine matters more than the phrasing.
			continue
		}

		said, isText := value.(string)
		if !isText {
			continue
		}

		if strings.HasSuffix(said, ".") && strings.Count(said, " ") > 6 {
			t.Errorf("%q is a written sentence, not a fact: %q", key, said)
		}
	}
}

/*
 * What is not true is left out, rather than given as nothing.
 *
 * A model handed "waiting: 0" will find something to say about nothing
 * waiting. A model handed nothing will not.
 */
func TestNothingToSayIsSaidByLeavingItOut(t *testing.T) {
	b := &Brain{}

	facts := b.about(nil, false, false)

	for _, absent := range []string{
		"things you would like to remember, waiting for them",
		"actions waiting for their approval",
		"the last thing you talked about",
		"conversations the last run was cut off in the middle of",
		"you have never met them before",
		"the drive it lives on",
	} {
		if _, there := facts[absent]; there {
			t.Errorf("%q is in the facts when it is not true: %v", absent, facts[absent])
		}
	}

	// And what is always true is there: a greeting has to know the hour.
	if facts["part of the day"] == nil {
		t.Error("it does not know what part of the day it is")
	}
}

// Coming back minutes later is not an arrival, and the facts say so, so the
// model can say almost nothing rather than reintroducing itself.
func TestComingBackMinutesLaterIsInTheFacts(t *testing.T) {
	b := &Brain{}

	last := &LastTalk{Topic: "the Godot game", When: time.Now().Add(-4 * time.Minute)}

	facts := b.about(last, true, false)

	if facts["they were talking to you minutes ago"] != true {
		t.Error("it does not know they were just here")
	}

	about, ok := facts["the last thing you talked about"].(About)
	if !ok {
		t.Fatalf("the last conversation is %T", facts["the last thing you talked about"])
	}

	if about["about"] != "the Godot game" {
		t.Errorf("it is about %v", about["about"])
	}

	if about["still the same conversation"] != true {
		t.Error("four minutes later is not the same conversation")
	}
}

// The part of the day is said the way somebody would say it.
func TestThePartOfTheDayIsSaidPlainly(t *testing.T) {
	for _, c := range []struct {
		hour int
		want string
	}{{2, "the middle of the night"}, {9, "morning"}, {14, "afternoon"}, {21, "evening"}} {
		if got := partOfDay(c.hour); got != c.want {
			t.Errorf("%02d:00 is %q, not %q", c.hour, got, c.want)
		}
	}
}

/*
 * A greeting is one line, whatever shape the model gave it.
 *
 * Asked about an evening with three approvals waiting, the model here
 * answered "Hello Petar,\n\nThree actions waiting for approval." Read out
 * loud, that break is a silence in the middle of a sentence.
 */
func TestAGreetingIsOneLine(t *testing.T) {
	got := oneLine("Hello Petar,\n\nThree actions waiting for approval.")

	if got != "Hello Petar, Three actions waiting for approval." {
		t.Errorf("it came out as %q", got)
	}

	if oneLine("Good morning, Petar.") != "Good morning, Petar." {
		t.Error("it changed a line that was already one line")
	}
}
