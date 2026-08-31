package tools

import "testing"

/*
 * Ordinary questions reach the web; personal ones do not.
 *
 * Searching is the only thing this program does that leaves the machine, and
 * the query is written by the model rather than typed by the person — so this
 * is the one place where "no personal information is shared" is either
 * enforced or merely hoped for.
 *
 * Both halves are tested, and the first half is the one that matters most. A
 * filter that blocks real questions is a filter that gets switched off, and a
 * filter that is switched off checks nothing at all.
 */
func TestOrdinaryQuestionsStillReachTheWeb(t *testing.T) {
	for _, query := range []string{
		"weather in Sofia today",
		"what is the weather right now in Sofia",
		"how to remove a stripped screw",
		"golang sync.Once deadlock",
		"train times Sofia to Plovdiv",
		"euro to lev exchange rate 2026",
		"best price for a 4TB drive under 200",
		"Dr Ivanov cardiologist Sofia reviews",
	} {
		if ok, why := PrivateEnough(query); !ok {
			t.Errorf("blocked an ordinary search %q: %s", query, why)
		}
	}
}

func TestPersonalDetailsNeverReachASearchEngine(t *testing.T) {
	for _, c := range []struct{ what, query string }{
		{"an email address", "when did petar.v.nikolov@gmail.com reply about the invoice"},
		{"a telephone number", "who called me from +359 88 123 4567"},
		{"a card number", "charge on 4111 1111 1111 1111"},
		{"an identity number", "check 8501015555 registration"},
		{"an API key", "why is my api_key rejected by the server"},
		{"a password", "reset the password for my router admin"},
	} {
		if ok, _ := PrivateEnough(c.query); ok {
			t.Errorf("%s would have been sent to a search engine: %q", c.what, c.query)
		}
	}
}

// The refusal explains itself, so the model searches for the general question
// rather than concluding the web has nothing and answering from imagination.
func TestARefusedSearchSaysWhy(t *testing.T) {
	ok, why := PrivateEnough("what did petar.v.nikolov@gmail.com say")
	if ok {
		t.Fatal("an email address passed the check")
	}

	if why == "" {
		t.Error("the search was refused without saying why, so the model learns nothing")
	}
}
