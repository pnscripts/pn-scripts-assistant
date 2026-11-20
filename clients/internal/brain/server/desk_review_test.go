package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/pair"
)

/*
 * The way round the window's key that the review of this change found.
 *
 * A program on this computer could ask for a pairing code with every right,
 * claim it, and then answer a card as "a device elsewhere" by using this
 * computer's own network address, where no key is asked. So: a pairing code
 * is offered, and the assistant is opened to the network or the internet,
 * only from the window.
 */
func TestAProgramHereCannotPairItselfOrOpenTheAssistantUp(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, c := range []struct{ path, body string }{
		{"/api/pair/offer", `{"can":"full"}`},
		{"/api/reach", `{"reach":"network"}`},
		{"/api/tunnel", `{}`},
		{"/api/tunnel/run", `{}`},
		{"/api/tunnel/x", `{}`},
	} {
		code, said := barePost(t, ts.URL+c.path, c.body)

		if code != http.StatusForbidden || !strings.Contains(said, `"not the window"`) {
			t.Errorf("a program with no window posted to %s (%d): %s", c.path, code, said)
		}
	}

	// The window still offers a code.
	if code, said := post(t, ts.URL+"/api/pair/offer", `{"can":"limited"}`); code != http.StatusOK {
		t.Errorf("the window could not offer a pairing code (%d): %s", code, said)
	}
}

/*
 * And what another program asked for is answered at this computer.
 *
 * The guide has said since 1.2.0 step 5 that a paired device cannot answer
 * another program's permission question; nothing made it so. A device with
 * every right goes on answering the assistant's own cards.
 */
func TestADeviceElsewhereDoesNotAnswerWhatAnotherProgramAskedFor(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	db := s.brain.DB

	file := func(name string) (string, string) {
		target := filepath.Join(t.TempDir(), name)
		args, _ := json.Marshal(map[string]string{"path": target, "content": "written"})

		return target, string(args)
	}

	theirs, args := file("asked-by-a-program.txt")

	asked, err := db.RecordInvocationAskedBy(0, "write_file", args, "Create "+theirs, "", "mutating", "Cursor 3.0")
	if err != nil {
		t.Fatal(err)
	}

	mine, args := file("asked-at-the-desk.txt")

	own, err := db.RecordInvocation(0, "write_file", args, "Create "+mine, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "laptop", "192.168.1.9")

	carry := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }

	for _, decision := range []string{"approve", "deny", "always-program"} {
		w := ask(s, "POST", "/api/approvals/"+itoa(asked)+"/"+decision, "192.168.1.9:4000", carry)

		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "computer") {
			t.Errorf("a device elsewhere answered what another program asked for with %q (%d): %s", decision, w.Code, w.Body.String())
		}
	}

	if _, err := os.Stat(theirs); err == nil {
		t.Error("the file another program asked for was written on a device's word")
	}

	if waiting, _ := db.PendingInvocations(); len(waiting) != 2 {
		t.Errorf("%d questions wait, want both", len(waiting))
	}

	// Its own cards, as before.
	if w := ask(s, "POST", "/api/approvals/"+itoa(own)+"/approve", "192.168.1.9:4000", carry); w.Code != 200 {
		t.Errorf("a device with every right could not answer the assistant's own card (%d): %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(mine); err != nil {
		t.Error("the assistant's own card, approved from a device with every right, did nothing")
	}

	// And the window answers the other one.
	w := ask(s, "POST", "/api/approvals/"+itoa(asked)+"/approve", "127.0.0.1:5000", func(r *http.Request) {
		r.Header.Set(DeskHeader, testDeskKey)
	})
	if w.Code != 200 {
		t.Errorf("the window could not answer what another program asked for (%d): %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(theirs); err != nil {
		t.Error("the window's yes did nothing")
	}
}
