package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bare is a program on this computer that is not the assistant's window:
// it posts with no page behind it and nothing the window was given.
var bare = &http.Client{Transport: &http.Transport{}}

func barePost(t *testing.T, url, body string) (int, string) {
	t.Helper()

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := bare.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw := make([]byte, 4096)
	n, _ := resp.Body.Read(raw)

	return resp.StatusCode, string(raw[:n])
}

/*
 * The fault the review of 1.2.0 found, as a program would do it.
 *
 * Any program running under the person's account could turn on what other
 * programs may ask, and answer a card, with one request each: nothing told
 * it apart from the assistant's own window.
 */
func TestAProgramHereCannotTurnOnWhatOtherProgramsMayAsk(t *testing.T) {
	ts, db, _ := newServer(t)

	for _, subject := range []string{"programs", "ask", "memory", "permissions"} {
		code, said := barePost(t, ts.URL+"/api/outside/allow", `{"subject":"`+subject+`","allowed":true}`)

		if code != http.StatusForbidden {
			t.Errorf("a program with no window turned on %q by itself (%d): %s", subject, code, said)
		}

		if on, err := db.OutsideAllowed(); err != nil || len(on) != 0 {
			t.Errorf("after a program with no window asked for %q these are on: %v (%v)", subject, on, err)
		}
	}
}

func TestAProgramHereCannotPostTheAnswerToACard(t *testing.T) {
	ts, db, _ := newServer(t)

	target := filepath.Join(t.TempDir(), "answered-by-a-program.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "nobody said yes to this"})

	id, err := db.RecordInvocation(0, "write_file", string(args), "Create "+target, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	for _, decision := range []string{"approve", "deny", "reject", "always-program"} {
		code, said := barePost(t, ts.URL+"/api/approvals/"+itoa(id)+"/"+decision, `{}`)

		if code != http.StatusForbidden {
			t.Errorf("a program with no window answered a card with %q (%d): %s", decision, code, said)
		}
	}

	waiting, err := db.PendingInvocations()
	if err != nil {
		t.Fatal(err)
	}

	if len(waiting) != 1 || waiting[0].ID != id {
		t.Errorf("the card is no longer waiting for its person: %+v", waiting)
	}

	if _, err := os.Stat(target); err == nil {
		t.Error("the file was written on the word of a program with no window")
	}
}
