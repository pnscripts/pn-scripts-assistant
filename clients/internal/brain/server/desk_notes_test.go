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
 * Notes of the review of the window's key (20.11.2026).
 *
 * (1) The pairing code on offer is shown only to the window. A program here
 * that read GET /api/paired while the person was pairing a phone could claim
 * the code before the phone did, and come back as a device with every right.
 */
func TestThePairingCodeIsShownOnlyToTheWindow(t *testing.T) {
	s := plain(t, config.ReachHere)
	s.Handler()

	code, _, err := s.paired.Offer(pair.Full)
	if err != nil {
		t.Fatal(err)
	}

	read := func(set func(*http.Request)) (int, string, map[string]any) {
		w := ask(s, "GET", "/api/paired", "127.0.0.1:5000", set)

		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)

		return w.Code, w.Body.String(), body
	}

	status, raw, body := read(nil)
	if status != http.StatusOK {
		t.Fatalf("a program here could not read the devices page (%d): %s", status, raw)
	}

	if strings.Contains(raw, code) {
		t.Errorf("a program with no window was shown the pairing code: %s", raw)
	}

	if body["code"] != "" || body["code_can"] != "" || body["code_until"] != nil {
		t.Errorf("a program with no window was told a code is on offer: %s", raw)
	}

	if _, listed := body["devices"]; !listed {
		t.Errorf("the rest of the devices page went with the code: %s", raw)
	}

	status, raw, body = read(func(r *http.Request) { r.Header.Set(DeskHeader, "not-the-key") })
	if status != http.StatusOK || strings.Contains(raw, code) {
		t.Errorf("a wrong key was shown the pairing code (%d): %s", status, raw)
	}

	status, raw, body = read(func(r *http.Request) { r.Header.Set(DeskHeader, testDeskKey) })
	if status != http.StatusOK || body["code"] != code || body["code_can"] != pair.Full || body["code_until"] == nil {
		t.Errorf("the window was not shown the code it offered (%d): %s", status, raw)
	}
}

/*
 * (4) A device elsewhere is refused when the card cannot be read.
 *
 * Whether a card was asked for by another program is read from its row; a
 * row that cannot be read let the answer through to Decide, which happened
 * to fail on the same error. Refused at the check now, with nothing done.
 */
func TestADeviceElsewhereIsRefusedWhenTheCardCannotBeRead(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	db := s.brain.DB

	target := filepath.Join(t.TempDir(), "unread.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "written"})

	id, err := db.RecordInvocationAskedBy(0, "write_file", string(args), "Create "+target, "", "mutating", "Cursor 3.0")
	if err != nil {
		t.Fatal(err)
	}

	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "laptop", "192.168.1.9")

	carry := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }

	// The row can no longer be read.
	if _, err := db.SQL().Exec(`ALTER TABLE tool_invocations RENAME TO tool_invocations_away`); err != nil {
		t.Fatal(err)
	}

	for _, decision := range []string{"approve", "deny", "always-program"} {
		w := ask(s, "POST", "/api/approvals/"+itoa(id)+"/"+decision, "192.168.1.9:4000", carry)

		if w.Code < 400 || !strings.Contains(w.Body.String(), "could not be read") {
			t.Errorf("a device elsewhere was let through with %q to a card that cannot be read (%d): %s",
				decision, w.Code, w.Body.String())
		}
	}

	if _, err := os.Stat(target); err == nil {
		t.Error("the file was written on a device's word")
	}
}

/*
 * (2) A card another program asked for, seen on a device elsewhere.
 *
 * The device was refused in English only and still shown the buttons, so a
 * person on a phone pressed Approve and read a sentence in another language.
 * The list now says, card by card, that such a card is answered at the
 * computer (the page draws a line in place of the buttons), and the refusal
 * carries a word the page says in its own language, for a page that was
 * drawn before the card came.
 */
func TestACardAnotherProgramAskedForSaysWhereItIsAnswered(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	db := s.brain.DB

	target := filepath.Join(t.TempDir(), "asked.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "written"})

	asked, err := db.RecordInvocationAskedBy(0, "write_file", string(args), "Create "+target, "", "mutating", "Cursor 3.0")
	if err != nil {
		t.Fatal(err)
	}

	mine, err := db.RecordInvocation(0, "write_file", string(args), "Create "+target+" (mine)", "mutating")
	if err != nil {
		t.Fatal(err)
	}

	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "phone", "192.168.1.9")

	carry := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }

	cards := func(from string, set func(*http.Request)) map[int64]map[string]any {
		w := ask(s, "GET", "/api/approvals", from, set)
		if w.Code != http.StatusOK {
			t.Fatalf("the waiting cards could not be read from %s (%d): %s", from, w.Code, w.Body.String())
		}

		var list []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatalf("%v: %s", err, w.Body.String())
		}

		byID := map[int64]map[string]any{}
		for _, c := range list {
			byID[int64(c["id"].(float64))] = c
		}

		return byID
	}

	away := cards("192.168.1.9:4000", carry)
	if away[asked]["at_the_computer_only"] != true {
		t.Errorf("a device elsewhere was not told the card another program asked for is answered at the computer: %v", away[asked])
	}

	if _, said := away[mine]["at_the_computer_only"]; said {
		t.Errorf("the assistant's own card was marked as answered at the computer only: %v", away[mine])
	}

	here := cards("127.0.0.1:5000", nil)
	for id, c := range here {
		if _, said := c["at_the_computer_only"]; said {
			t.Errorf("the window was told card %d is answered elsewhere: %v", id, c)
		}
	}

	for _, decision := range []string{"approve", "deny", "always-program"} {
		w := ask(s, "POST", "/api/approvals/"+itoa(asked)+"/"+decision, "192.168.1.9:4000", carry)

		var said map[string]string
		json.Unmarshal(w.Body.Bytes(), &said)

		if w.Code != http.StatusForbidden || said["desk"] != "at the computer" || !strings.Contains(said["error"], "own window") {
			t.Errorf("%q from a device elsewhere was not refused with the word the page says (%d): %s", decision, w.Code, w.Body.String())
		}
	}

	if _, err := os.Stat(target); err == nil {
		t.Error("the file was written on a device's word")
	}

	if inv, err := db.Invocation(asked); err != nil || inv == nil || inv.Status != "pending" {
		t.Errorf("the card another program asked for should still wait: %+v (%v)", inv, err)
	}
}
