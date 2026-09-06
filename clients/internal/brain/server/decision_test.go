package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

/*
 * Approving something has to say what it did.
 *
 * The page read the outcome from a field the server does not send, so every
 * approved action reported "Done — undefined". The moment somebody most needs
 * to be told what happened is the moment after they have agreed to it, and it
 * said nothing at all.
 *
 * Tested at the endpoint rather than in the page, because the fault was the
 * two disagreeing about the shape: one test that reads the real response is
 * worth more than two that each assume it.
 */
func TestADecisionSaysWhatHappened(t *testing.T) {
	ts, _, _ := newServer(t)

	// Nothing with that id, which is the shape this test is about: the reply
	// carries an error and an invocation, and the page has to find both.
	res, err := http.Post(ts.URL+"/api/approvals/9999/approve", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	var body map[string]any

	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	/*
	 * Whatever the outcome, the fields the page reads have to be the fields
	 * the server writes. "result" at the top level is the one it looked for
	 * and the one that was never there.
	 */
	if _, wrong := body["result"]; wrong {
		t.Fatal("the server now sends a top-level result; the page reads invocation.result")
	}

	if _, right := body["invocation"]; !right {
		if _, failed := body["error"]; !failed {
			t.Fatalf("the reply carries neither an invocation nor an error: %v", body)
		}
	}
}

/*
 * And Reject has to reach something that knows the word.
 *
 * The button said "reject" and the endpoint accepted only "deny", so pressing
 * it answered 400 and rejected nothing — the action stayed in the queue, and
 * the only sign was an error where a confirmation should have been.
 */
func TestBothWordsForRejectingAreAccepted(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, word := range []string{"deny", "reject"} {
		res, err := http.Post(ts.URL+"/api/approvals/9999/"+word, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}

		body := make(map[string]any)

		json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()

		// There is no such action, so an error about that is right. An error
		// about the word is not.
		if said, _ := body["error"].(string); strings.Contains(said, `must be "approve"`) {
			t.Errorf("%q was not understood as a decision", word)
		}
	}
}

// A word that is neither is still refused, or the guard means nothing.
func TestAnUnknownDecisionIsRefused(t *testing.T) {
	ts, _, _ := newServer(t)

	res, err := http.Post(ts.URL+"/api/approvals/1/maybe", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("a nonsense decision answered %s", res.Status)
	}
}
