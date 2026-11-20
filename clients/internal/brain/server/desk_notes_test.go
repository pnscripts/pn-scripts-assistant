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
