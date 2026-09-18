package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/pair"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tunnel"
)

// plain builds a server exactly as the program does, with nothing replaced.
func plain(t *testing.T, reach string) *Server {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Reach = reach

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger)
}

/*
 * guarded is the same server with a handler that records whether anything got
 * past the door, so a test can tell "refused" from "allowed and then failed
 * for its own reasons".
 */
func guarded(t *testing.T, reach string) (*Server, *bool) {
	t.Helper()

	s := plain(t, reach)

	got := false

	s.through = s.Guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { got = true }))

	return s, &got
}

// ask makes one request and says what happened.
func ask(s *Server, method, path, from string, set func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = from

	if set != nil {
		set(r)
	}

	w := httptest.NewRecorder()

	s.through.ServeHTTP(w, r)

	return w
}

/*
 * A request from this machine is not a trust decision.
 *
 * Nothing outside this computer can put a packet on the loopback interface, so
 * a request that arrived on it came from here. That was the whole of the
 * security model for the life of this program, and it stays true.
 */
func TestThisMachineIsAlwaysLetIn(t *testing.T) {
	for _, reach := range []string{config.ReachHere, config.ReachNetwork} {
		s, got := guarded(t, reach)

		for _, from := range []string{"127.0.0.1:5555", "[::1]:5555"} {
			*got = false

			if w := ask(s, "POST", "/api/settings", from, nil); w.Code != 200 || !*got {
				t.Errorf("reach %s, from %s: the computer itself was refused (%d)", reach, from, w.Code)
			}
		}
	}
}

/*
 * A stranger is refused everything that is not the way in.
 *
 * The memory, the mail, the files and the ability to run a command are behind
 * this. Before it, they were behind nothing but the loopback interface — which
 * was enough only for as long as that was the only way to reach them.
 */
func TestAStrangerIsRefused(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	for _, path := range []string{
		"/api/chat", "/api/knowledge", "/api/search", "/api/settings",
		"/api/approvals", "/api/mail", "/api/tasks", "/api/paired",
	} {
		*got = false

		w := ask(s, "GET", path, "192.168.1.55:4000", nil)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s answered a stranger with %d", path, w.Code)
		}

		if *got {
			t.Errorf("%s was reached by a stranger", path)
		}
	}
}

/*
 * And refused even harder when nothing has been opened up.
 *
 * It should not be possible to arrive here at all — the listener refuses to
 * bind anywhere but loopback unless reach is on — so this is the second of two
 * locks. Belt and braces on the one thing in this program where being wrong
 * cannot be taken back.
 */
func TestWithTheDoorShutNothingFromOutsideIsEvenConsidered(t *testing.T) {
	s, got := guarded(t, config.ReachHere)

	code, _, _ := s.paired.Offer(pair.Full)
	_, token, err := s.paired.Accept(code, "a paired laptop", "192.168.1.9")
	if err != nil {
		t.Fatal(err)
	}

	// Paired, and still refused, because the door is shut.
	w := ask(s, "GET", "/api/chat", "192.168.1.9:4000", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("a paired device was answered with %d while the door was shut", w.Code)
	}

	if *got {
		t.Error("it got through while the door was shut")
	}
}

// A paired device gets in, by cookie or by header, because a browser has one
// and everything else has the other.
func TestAPairedDeviceIsLetIn(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	code, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(code, "laptop", "192.168.1.9")

	for _, how := range []struct {
		name string
		set  func(*http.Request)
	}{
		{"header", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }},
		{"cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: TokenCookie, Value: token})
		}},
	} {
		*got = false

		if w := ask(s, "GET", "/api/chat", "192.168.1.9:4000", how.set); w.Code != 200 || !*got {
			t.Errorf("by %s: a paired device was refused (%d)", how.name, w.Code)
		}
	}
}

/*
 * A device away from the computer can talk to it and watch it, and cannot make
 * the decisions.
 *
 * The line is one question: may this approve what the assistant wants to do? A
 * phone on a train is a fine place to ask it something and a poor place to
 * approve it deleting files — and somebody who loses that phone should not
 * have lost the gate with it.
 */
func TestDecisionsStayAtTheDesk(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	code, _, _ := s.paired.Offer(pair.Limited)
	_, token, _ := s.paired.Accept(code, "phone", "192.168.1.9")

	carry := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }

	// It can do the things it is for.
	for _, path := range []string{"/api/chat", "/api/tasks", "/api/knowledge", "/api/status",
		"/api/integrations", "/api/packages", "/api/proposals", "/api/inspect"} {
		*got = false

		if w := ask(s, "GET", path, "192.168.1.9:4000", carry); w.Code != 200 {
			t.Errorf("a limited device was refused %s (%d)", path, w.Code)
		}
	}

	// And not the ones that decide.
	for _, path := range []string{
		"/api/approvals/3/approve", "/api/permissions/decide", "/api/settings",
		"/api/protection", "/api/privacy", "/api/reach", "/api/pair/offer",
		"/api/paired/abc", "/api/places", "/api/parts/install", "/api/models/use",
		"/api/organisation/hire/4/confirm", "/api/provision/install", "/api/projects/propose",
		"/api/projects/7/start", "/api/integrations/memory/activate", "/api/integrations/own",
		"/api/integrations/memory/secret",
	} {
		*got = false

		w := ask(s, "POST", path, "192.168.1.9:4000", carry)

		if w.Code != http.StatusForbidden {
			t.Errorf("a limited device was allowed %s (%d)", path, w.Code)
		}

		if *got {
			t.Errorf("a limited device reached %s", path)
		}
	}
}

/*
 * Claiming a code is the one route a stranger may call.
 *
 * It has to be: it is how a device stops being a stranger. What protects it is
 * that the code is short-lived, single-use, and only exists while somebody is
 * standing at the computer having asked for one.
 */
func TestOnlyTheWayInIsOpenToStrangers(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	*got = false

	if w := ask(s, "POST", "/api/pair/claim", "192.168.1.55:4000", nil); w.Code != 200 || !*got {
		t.Errorf("a stranger could not reach the way in (%d)", w.Code)
	}

	// The page and its assets, too, or a device with no token cannot load the
	// screen that asks for the code.
	for _, path := range []string{"/", "/js/console.js", "/health"} {
		*got = false

		if w := ask(s, "GET", path, "192.168.1.55:4000", nil); w.Code != 200 || !*got {
			t.Errorf("a stranger could not load %s (%d)", path, w.Code)
		}
	}
}

/*
 * A page on another site cannot drive this with a device's own cookie.
 *
 * SameSite keeps a browser from sending it at all; this is the belt to that
 * brace, because a browser that gets it wrong should not be the only thing
 * between somebody's assistant and a page they happened to open.
 */
func TestAPageSomewhereElseCannotDriveIt(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	code, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(code, "laptop", "192.168.1.9")

	*got = false

	w := ask(s, "POST", "/api/chat", "192.168.1.9:4000", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: TokenCookie, Value: token})
		r.Header.Set("Origin", "https://somewhere-else.example")
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("a page on another site drove it (%d)", w.Code)
	}

	if *got {
		t.Error("a request from another site got through")
	}

	// The assistant's own page is fine.
	*got = false

	w = ask(s, "POST", "/api/chat", "192.168.1.9:4000", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: TokenCookie, Value: token})
		r.Host = "192.168.1.4:8790"
		r.Header.Set("Origin", "https://192.168.1.4:8790")
	})

	if w.Code != 200 || !*got {
		t.Errorf("its own page was refused (%d)", w.Code)
	}
}

/*
 * A header claiming to be this machine is not this machine.
 *
 * X-Forwarded-For and everything like it is written by whoever sent the
 * request. Consulting one would let anybody be the computer by saying so,
 * which is the whole of the door gone for the sake of one convenience.
 */
func TestSayingYouAreThisMachineDoesNotMakeYouIt(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	*got = false

	w := ask(s, "GET", "/api/knowledge", "192.168.1.55:4000", func(r *http.Request) {
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("X-Real-IP", "127.0.0.1")
	})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("a header claiming to be loopback was believed (%d)", w.Code)
	}

	if *got {
		t.Error("it got in by claiming to be this machine")
	}
}

/*
 * The guard is in the path that actually runs.
 *
 * It was not. Serve mounted the bare mux, so every route answered with no
 * check at all — correct for as long as the listener could only be loopback,
 * and a hole the moment it could not. The guard existed, was tested, and was
 * simply never called, which no test of the guard itself could ever have
 * found.
 */
func TestTheGuardIsInThePathThatRuns(t *testing.T) {
	// Nothing replaced: the real mux, behind the real door, the way Serve
	// mounts it.
	s := plain(t, config.ReachNetwork)

	mounted := s.Handler()

	r := httptest.NewRequest("GET", "/api/knowledge", nil)
	r.RemoteAddr = "192.168.1.55:4000"

	w := httptest.NewRecorder()
	mounted.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a stranger reached %s through the handler Serve mounts (%d)",
			r.URL.Path, w.Code)
	}
}

/*
 * Unpairing a device takes its way in from outside with it.
 *
 * A phone left in a taxi is the case all of this exists for. Unpaired but
 * still holding a tunnel key, it would still reach the home network — where it
 * would find a locked door, which is not what somebody unpairing a lost phone
 * is asking for.
 */
func TestUnpairingTakesTheTunnelKeyToo(t *testing.T) {
	s := plain(t, config.ReachNetwork)

	code, _, _ := s.paired.Offer(pair.Full)

	device, _, err := s.paired.Accept(code, "the phone in the taxi", "192.168.1.9")
	if err != nil {
		t.Fatal(err)
	}

	tun, _ := tunnel.Load(s.brain.Root)
	tun.Start()
	tun.Endpoint = "home.example.net"

	if _, err := tun.Add(device.ID, device.Name); err != nil {
		t.Fatal(err)
	}

	if err := tun.Save(s.brain.Root); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("DELETE", "/api/paired/"+device.ID, nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.SetPathValue("id", device.ID)

	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("unpairing answered %d", w.Code)
	}

	after, _ := tunnel.Load(s.brain.Root)

	if _, still := after.Peer(device.ID); still {
		t.Error("the lost phone still has a way into the home network")
	}
}
