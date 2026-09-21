package brain

import (
	"context"
	"pn-scripts-assistant/internal/brain/llm"
	"strings"
	"testing"
	"time"
)

/*
 * Coming back after five minutes is not an arrival.
 *
 * The greeting is written for opening the program after a while, and delivered
 * every launch it turns a conversation into a machine rebooting at somebody.
 * During an afternoon of restarts it was said eight times in an hour, word for
 * word, to a person who had never left the room.
 */
func TestItDoesNotGreetSomebodyWhoNeverLeft(t *testing.T) {
	line := lastTopicLine(&LastTalk{
		Topic: "fixing the core background",
		When:  time.Now().Add(-5 * time.Minute),
	}, true)

	// Under two minutes it says nothing about the topic; at five it does, and
	// that is the useful part of the greeting.
	if line == "" {
		t.Fatal("said nothing at all about a conversation five minutes old")
	}

	if !strings.Contains(line, "Carrying on") {
		t.Fatalf("did not carry on: %q", line)
	}
}

// And the recital about how much it knows is for somebody arriving, not for
// somebody who has been here all along.
func TestJustCameBackIsShorterThanHalfAnHour(t *testing.T) {
	if JustCameBack > 30*time.Minute {
		t.Fatalf("JustCameBack is %v, which would silence a real hello", JustCameBack)
	}

	if JustCameBack <= TooRecentToMention {
		t.Fatal("coming back must cover a longer span than being told the topic again")
	}
}

// a model that answers greetings with whatever it is given.
type phrasing struct{ said string }

func (phrasing) Name() string                   { return "ollama" }
func (phrasing) Available(context.Context) bool { return true }

func (p phrasing) Chat(_ context.Context, _ llm.Request) (llm.Response, error) {
	return llm.Response{Content: p.said}, nil
}

/*
 * The assistant greets in its own words, from the facts it worked out.
 *
 * What it must not be is a sentence formatted in Go. What it must not do
 * either is go quiet when there is no model: the composed greeting is still
 * there, and it is what gets said when nobody answers.
 */
func TestTheGreetingIsSaidByTheAssistant(t *testing.T) {
	t.Setenv("PN_TEST_WORDING", "1")

	b := quietBrain(t)

	plain := b.Greet()
	if len(plain.Facts) == 0 {
		t.Fatal("the greeting is made of no facts at all")
	}

	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", phrasing{said: "Evening. Nothing is waiting."})

	said := b.GreetInWords(context.Background())

	if said.Text != "Evening. Nothing is waiting." {
		t.Errorf("the greeting was not the assistant's: %q", said.Text)
	}

	// Asked again, the same greeting is not paid for twice.
	b.Router = llm.NewRouter(llm.ModeOpen, "ollama", phrasing{said: "something else entirely"})

	if again := b.GreetInWords(context.Background()); again.Text != said.Text {
		t.Errorf("it said it again differently: %q then %q", said.Text, again.Text)
	}

	// And with no model to ask, it still says what it knows.
	quiet := quietBrain(t)
	quiet.Router = llm.NewRouter(llm.ModeOpen, "nobody")

	// Its own composed sentences, which vary on purpose, so the test compares
	// the greeting with the facts it was made of rather than with another go
	// at composing it.
	fallen := quiet.GreetInWords(context.Background())

	if fallen.Text == "" || fallen.Text != strings.Join(fallen.Facts, " ") {
		t.Errorf("with no model it said %q, from facts %q", fallen.Text, fallen.Facts)
	}
}
