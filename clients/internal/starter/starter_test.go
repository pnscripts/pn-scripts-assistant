package starter

import (
	"strings"
	"testing"
)

// The starter's whole value is being trustworthy at the moment someone is
// deciding whether to let this software near their files. Confidently answering
// the wrong question would undermine that faster than saying nothing.

func TestAnswersTheQuestionActuallyAsked(t *testing.T) {
	cases := []struct {
		question string
		expect   string // a phrase the correct answer must contain
	}{
		{"is my data private?", "nothing leaves your computer"},
		// Asked because it used to need Docker, and somebody who read about
		// this program a year ago still will. The answer that matters is that
		// it does not — "database" was the old answer and passing on it now
		// would only prove the sentence still contains the word.
		{"why do I need docker?", "does not use Docker"},
		{"do I need containers?", "does not use Docker"},
		{"how long does this take?", "2GB"},
		{"does it cost anything?", "Nothing here costs money"},
		{"will it work offline?", "no internet connection"},
		{"what can it do?", "asks your permission"},
		{"do I need a gpu?", "not required"},
		{"where is my data stored?", "external drive"},
		{"how do I uninstall it?", "Delete the data folder"},
		{"why does it want my password?", "never stores it"},
	}

	for _, c := range cases {
		reply := Ask(c.question)

		if !reply.Matched {
			t.Errorf("%q went unanswered", c.question)

			continue
		}

		if !strings.Contains(reply.Answer, c.expect) {
			t.Errorf("%q answered with the wrong topic; expected text containing %q, got %.70s",
				c.question, c.expect, reply.Answer)
		}
	}
}

// Specificity has to win, or every question mentioning space gets whichever
// topic happens to be listed first.
func TestPrefersTheMoreSpecificTopic(t *testing.T) {
	reply := Ask("how much disk space do I need")

	if !strings.Contains(reply.Answer, "external drive") {
		t.Errorf("expected the storage topic, got: %.70s", reply.Answer)
	}
}

func TestAdmitsWhenItDoesNotKnow(t *testing.T) {
	for _, q := range []string{
		"what is the capital of France",
		"write me a poem",
		"",
	} {
		reply := Ask(q)

		if reply.Matched {
			t.Errorf("%q should not have matched anything", q)
		}

		// It must not imply it is the assistant, or that expectation carries
		// into the rest of the session.
		if !strings.Contains(reply.Answer, "not the assistant itself") {
			t.Errorf("the fallback should say plainly what it is not; got: %.70s", reply.Answer)
		}
	}
}

func TestOffersSomewhereToStart(t *testing.T) {
	if len(Suggestions()) < 3 {
		t.Error("an empty box with no prompts is a box nobody types into")
	}

	for _, s := range Suggestions() {
		if !Ask(s).Matched {
			t.Errorf("suggested question %q has no answer behind it", s)
		}
	}
}
