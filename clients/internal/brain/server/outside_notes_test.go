package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/pair"
)

/*
 * The notes of the security review of 1.2.0 (21.11.2026), each as the
 * fault it described.
 *
 * (1) One number a minute was shared by every question of every program.
 * A program asking "is it running" in a loop used it up, and Claude Code's
 * permission questions were then answered no without anybody being asked.
 */
func TestAProgramAskingInALoopDoesNotAnswerPermissionQuestionsNo(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	status := `{"tool":"assistant_status","client":{"name":"loop","version":"1"}}`

	for i := 0; i < otherprograms.MostAMinute+20; i++ {
		code, said := post(t, ts.URL+"/api/outside/ask", status)

		if want := http.StatusOK; i < otherprograms.MostAMinute && code != want {
			t.Fatalf("question %d of the minute: %d %s", i+1, code, said)
		}

		if i >= otherprograms.MostAMinute && code != http.StatusTooManyRequests {
			t.Fatalf("question %d, over the number: %d %s", i+1, code, said)
		}
	}

	// The flood is on. A permission question is still put to the person,
	// and their yes is what the program is told.
	asked := askTheDoor(t, ts.URL, permitBash)

	card := permitCards(t, ts.URL, 1)[0]
	if card.AskedBy != "claude-code 2.1.303" {
		t.Fatalf("the card during a flood of other questions: %+v", card)
	}

	if code, body := post(t, ts.URL+"/api/approvals/"+itoa(card.ID)+"/approve", `{}`); code != http.StatusOK {
		t.Fatalf("approving: %d %s", code, body)
	}

	if said := doorAnswered(t, asked); said.code != http.StatusOK || !strings.Contains(said.body, `"behavior":"allow"`) {
		t.Fatalf("a permission question during a flood of other questions: %d %s", said.code, said.body)
	}

	if d := theirs(t, db)[0]; d.Tool != otherprograms.PermissionTool || d.Outcome != "done" {
		t.Errorf("the record of the permission question: %+v", d)
	}
}

/*
 * And the other way round: permission questions have a number of their
 * own, a small one. Over it they are refused with one line on the record,
 * and the other questions keep their minute.
 */
func TestPermissionQuestionsInALoopAreStoppedByTheirOwnNumber(t *testing.T) {
	ts, db, b := newServer(t)
	permitWaits(t, time.Minute)

	if err := b.AllowOutside(otherprograms.Permit, true); err != nil {
		t.Fatal(err)
	}

	// Answered at once, with no card: the quickest permission question.
	own := `{"tool":"permission_prompt","arguments":{"tool_name":"AskUserQuestion","input":{}},"client":{"name":"loop","version":"1"}}`

	for i := 0; i < otherprograms.MostPermissionsAMinute; i++ {
		if code, said := post(t, ts.URL+"/api/outside/ask", own); code != http.StatusOK {
			t.Fatalf("permission question %d of the minute: %d %s", i+1, code, said)
		}
	}

	before := len(theirs(t, db))

	for i := 0; i < 40; i++ {
		body := own
		if i%2 == 1 {
			body = permitBash
		}

		code, said := post(t, ts.URL+"/api/outside/ask", body)
		if code != http.StatusTooManyRequests || !strings.Contains(said, `"outside":"too many"`) {
			t.Fatalf("permission question %d over the number: %d %s", i+1, code, said)
		}
	}

	// Nothing reached the desk, and the forty are one line.
	permitCards(t, ts.URL, 0)

	deeds := theirs(t, db)
	if len(deeds) != before+1 {
		t.Fatalf("%d lines on the record for 40 permission questions over the number", len(deeds)-before)
	}

	en, _ := otherprograms.TooManyPermissions()
	if deeds[0].Tool != otherprograms.PermissionTool || deeds[0].Outcome != "refused" || deeds[0].Result != en || deeds[0].AskedBy != "loop 1" {
		t.Errorf("the line for the flood of permission questions: %+v", deeds[0])
	}

	// The other questions were not touched by it.
	if code, said := post(t, ts.URL+"/api/outside/ask", `{"tool":"assistant_status","client":{"name":"another"}}`); code != http.StatusOK {
		t.Errorf("whether it is running, during a flood of permission questions: %d %s", code, said)
	}
}

// The page reads the number out of the record's line: both lines have the
// shape its expression looks for (programs.js, "out.rec.tooMany").
func TestBothFloodLinesAreReadByThePage(t *testing.T) {
	all, _ := otherprograms.TooMany()
	permits, _ := otherprograms.TooManyPermissions()

	for line, n := range map[string]int{all: otherprograms.MostAMinute, permits: otherprograms.MostPermissionsAMinute} {
		if !strings.Contains(line, fmt.Sprintf("more than %d questions in a minute", n)) {
			t.Errorf("the page would not read the number out of %q", line)
		}
	}
}

// claim posts a pairing code as a device at from would.
func claim(s *Server, from, code string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"code": code, "name": "a test"})

	r := httptest.NewRequest(http.MethodPost, "/api/pair/claim", bytes.NewReader(body))
	r.RemoteAddr = from
	r.Host = "127.0.0.1:8896"
	r.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	s.through.ServeHTTP(w, r)

	return w
}

/*
 * (2) While the person pairs a phone, a program on this computer could
 * take the code off the screen with one request and no key: POST
 * /api/pair/stop. Withdrawing the code is the window's to do, like
 * offering it.
 */
func TestOnlyTheWindowWithdrawsThePairingCode(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	code, _, err := s.paired.Offer(pair.Limited)
	if err != nil {
		t.Fatal(err)
	}

	for name, set := range map[string]func(*http.Request){
		"no key":      nil,
		"a wrong key": func(r *http.Request) { r.Header.Set(DeskHeader, "not-the-key") },
	} {
		w := ask(s, "POST", "/api/pair/stop", "127.0.0.1:5000", set)

		var said map[string]string
		json.Unmarshal(w.Body.Bytes(), &said)

		if w.Code != http.StatusForbidden || said["desk"] != "not the window" {
			t.Errorf("a program with %s withdrew the pairing code (%d): %s", name, w.Code, w.Body.String())
		}

		if waiting, _, _ := s.paired.Waiting(); waiting != code {
			t.Fatalf("after a program with %s asked, the code on offer is %q, not %q", name, waiting, code)
		}
	}

	// The window still can.
	if w := ask(s, "POST", "/api/pair/stop", "127.0.0.1:5000", func(r *http.Request) { r.Header.Set(DeskHeader, testDeskKey) }); w.Code != http.StatusOK {
		t.Fatalf("the window could not withdraw the code (%d): %s", w.Code, w.Body.String())
	}

	if waiting, _, _ := s.paired.Waiting(); waiting != "" {
		t.Errorf("the window withdrew the code and %q is still on offer", waiting)
	}
}

/*
 * And a wrong guess spends the code, which is right for a stranger on the
 * network and was a free denial for a program here: one wrong guess from
 * this computer, repeated, and no phone could ever be paired. From this
 * computer a wrong guess now spends nothing, and only a few are looked at
 * (pair.AcceptHere). "This computer" is every loopback address, and its
 * own addresses on its networks too: with the assistant open to the
 * network, a program here reaches it by the machine's own address and
 * arrives looking like a device elsewhere.
 */
func TestAProgramHereCannotSpendThePairingCode(t *testing.T) {
	s := plain(t, config.ReachNetwork)
	s.Handler()

	// This computer has these two addresses on its networks.
	before := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)},
			&net.IPNet{IP: net.ParseIP("fd00::5"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("fe80::5"), Mask: net.CIDRMask(64, 128)},
		}, nil
	}

	defer func() { interfaceAddrs = before }()

	code, _, err := s.paired.Offer(pair.Full)
	if err != nil {
		t.Fatal(err)
	}

	// Far more guesses than are looked at, from every address that is
	// this computer.
	for i := 0; i < 40; i++ {
		for _, from := range []string{"127.0.0.1:5000", "127.0.0.9:5000", "[::1]:5000", "[::ffff:127.0.0.1]:5000", "192.168.1.5:5000", "[fd00::5]:5000", "[fe80::5%eth0]:5000"} {
			w := claim(s, from, "WRONG1")

			var said map[string]string
			json.Unmarshal(w.Body.Bytes(), &said)

			if w.Code != http.StatusForbidden || said["pair"] != "from this computer" || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("a wrong guess from %s: %d %s", from, w.Code, w.Body.String())
			}

			if waiting, _, _ := s.paired.Waiting(); waiting != code {
				t.Fatalf("a wrong guess from %s spent the code on offer", from)
			}
		}
	}

	// The guesses that are looked at ran out long ago: the right code from
	// here is not read either, so a program cannot go through them all.
	// Whoever typed it is told what happened and what to do, not that
	// the code is wrong.
	if w := claim(s, "127.0.0.1:5000", code); w.Code != http.StatusForbidden || s.paired.Any() ||
		!strings.Contains(w.Body.String(), "Ask for a new code") || strings.Contains(w.Body.String(), "is not right") ||
		!strings.Contains(w.Body.String(), `"pair":"from this computer"`) {
		t.Fatalf("the right code from here after the guesses ran out: %d %s", w.Code, w.Body.String())
	}

	// The phone across the room pairs with the same code afterwards.
	if w := claim(s, "192.168.1.9:4000", code); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Set-Cookie"), TokenCookie+"=") {
		t.Fatalf("after the guesses from this computer the phone could not pair (%d): %s", w.Code, w.Body.String())
	}

	// A stranger's wrong guess still spends it, as before: a code that
	// survives a wrong guess from the network can be guessed at.
	again, _, _ := s.paired.Offer(pair.Limited)

	if w := claim(s, "192.168.1.55:4000", "WRONG1"); w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "from this computer") {
		t.Fatalf("a wrong guess from the network: %d %s", w.Code, w.Body.String())
	}

	if w := claim(s, "192.168.1.9:4000", again); w.Code != http.StatusForbidden {
		t.Errorf("the right code still worked after a wrong guess from the network (%d)", w.Code)
	}

	// And the right code typed at this computer's own address pairs, as
	// it always did (a device on a forwarded port).
	third, _, _ := s.paired.Offer(pair.Limited)

	if w := claim(s, "127.0.0.1:5000", "WRONG1"); w.Code != http.StatusForbidden {
		t.Fatal("a wrong guess from here was not refused")
	}

	if w := claim(s, "127.0.0.1:5000", third); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Set-Cookie"), TokenCookie+"=") {
		t.Errorf("the right code from this computer's own address (%d): %s", w.Code, w.Body.String())
	}
}

// An address that is not this computer's is not taken for it.
func TestOnlyThisComputersOwnAddressesAreThisComputer(t *testing.T) {
	before := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)}}, nil
	}

	defer func() { interfaceAddrs = before }()

	for host, own := range map[string]bool{
		"192.168.1.5": true, "::ffff:192.168.1.5": true, "[192.168.1.5]": true, "192.168.1.5%eth0": true,
		"192.168.1.50": false, "192.168.1.9": false, "10.0.0.5": false, "": false, "not an address": false,
	} {
		if ownAddress(host) != own {
			t.Errorf("%q: this computer's own address is %v", host, !own)
		}
	}

	interfaceAddrs = func() ([]net.Addr, error) { return nil, errors.New("no list") }

	if ownAddress("192.168.1.5") {
		t.Error("with no list of addresses, one was taken for this computer's")
	}
}
