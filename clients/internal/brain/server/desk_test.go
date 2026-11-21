package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/pair"
)

/*
 * The tests of this package are, unless they say otherwise, the window.
 *
 * Every server a test makes takes the same key, and the default client
 * carries it as the page does: on what it posts, never on what it reads.
 * A test about a program that is not the window uses `bare` (desk_repro_test.go)
 * or builds its request by hand.
 */
const testDeskKey = "the-key-the-tests-hold-as-the-window"

type asTheWindow struct{ plain http.RoundTripper }

func (a asTheWindow) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(DeskHeader) == "" {
		r = r.Clone(r.Context())
		r.Header.Set(DeskHeader, testDeskKey)
	}

	return a.plain.RoundTrip(r)
}

func TestMain(m *testing.M) {
	fixedDeskKey = testDeskKey

	// And every test server seals its answers for the MCP server's tools
	// with one key (vouch_test.go), as the program does with its user's.
	fixedAnswersKey = answersKey

	// The default client only: http.DefaultTransport stays what the
	// program's own clients expect it to be.
	http.DefaultClient.Transport = asTheWindow{plain: http.DefaultTransport}

	os.Exit(m.Run())
}

// A server made by the program itself has a key of its own each time, long
// enough that nobody guesses it.
func TestEveryRunHasItsOwnKey(t *testing.T) {
	fixedDeskKey = ""
	defer func() { fixedDeskKey = testDeskKey }()

	one, two := newDesk(), newDesk()

	if one.key == two.key {
		t.Fatal("two runs were given the same key")
	}

	if len(one.key) != 64 || !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(one.key) {
		t.Errorf("the key is not 256 bits in hex: %d characters", len(one.key))
	}

	if one.holds("") || one.holds(two.key) || one.holds(one.key[:63]) || !one.holds(one.key) {
		t.Error("the key is held by something other than exactly itself")
	}
}

func enter(t *testing.T, client *http.Client, base, code string) (int, map[string]string) {
	t.Helper()

	req, _ := http.NewRequest(http.MethodPost, base+"/api/desk/enter", strings.NewReader(`{"code":"`+code+`"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	out := map[string]string{}
	json.NewDecoder(resp.Body).Decode(&out)

	return resp.StatusCode, out
}

// The link the window is given works once, and gives the key to nobody who
// does not have it.
func TestTheWindowsLinkWorksOnce(t *testing.T) {
	s := plain(t, config.ReachHere)

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	link := s.DeskLink(ts.URL + "/")

	if !strings.HasPrefix(link, ts.URL+"/desk/") {
		t.Fatalf("the link is not at this assistant's own address: %s", link)
	}

	code := strings.TrimPrefix(link, ts.URL+"/desk/")

	// The page behind it says nothing a reader could use.
	resp, err := bare.Get(link)
	if err != nil {
		t.Fatal(err)
	}

	raw := make([]byte, 16384)
	n, _ := resp.Body.Read(raw)
	resp.Body.Close()

	page := string(raw[:n])

	if resp.StatusCode != 200 || strings.Contains(page, testDeskKey) || strings.Contains(page, code) {
		t.Errorf("the page behind the link (%d) carries the key or the code", resp.StatusCode)
	}

	if !strings.Contains(page, "PN Scripts Assistant") || !strings.Contains(page, `src="/js/desk-enter.js"`) {
		t.Error("the page behind the link is not the one that takes the key")
	}

	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("the page behind the link may be kept by a cache")
	}

	// Reading the page used nothing up, and a wrong code gets nothing.
	for _, wrong := range []string{"", "nope", code[:63], code + "0", strings.Repeat("0", 64)} {
		if status, out := enter(t, bare, ts.URL, wrong); status != http.StatusForbidden && status != http.StatusBadRequest || out["key"] != "" {
			t.Errorf("the code %q was answered %d with a key %q", wrong, status, out["key"])
		}
	}

	status, out := enter(t, bare, ts.URL, code)
	if status != 200 || out["key"] != testDeskKey {
		t.Fatalf("the window's own link did not give it the key (%d)", status)
	}

	// Once.
	if status, out := enter(t, bare, ts.URL, code); status != http.StatusForbidden || out["key"] != "" {
		t.Errorf("the link worked a second time (%d)", status)
	}
}

// Only so many links wait: the oldest stops working.
func TestOnlyAFewLinksWaitAtOnce(t *testing.T) {
	d := newDesk()

	first := d.link()

	var later []string
	for i := 0; i < mostDeskLinks; i++ {
		later = append(later, d.link())
	}

	if _, known := d.enter(first); known {
		t.Error("the oldest link still works with more than the most waiting")
	}

	for _, code := range later {
		if _, known := d.enter(code); !known {
			t.Error("a link among the newest did not work")
		}
	}
}

// The key is given on this computer, to a page of the assistant, and to
// nothing else.
func TestTheKeyIsGivenOnlyHere(t *testing.T) {
	s := plain(t, config.ReachNetwork)

	// The real handler behind the real guard.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/desk/enter", s.handleDeskEnter)
	s.through = s.Guard(mux)

	body := func(code string) func(*http.Request) {
		return func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"code":"` + code + `"}`))
		}
	}

	// A device paired with every right, elsewhere.
	offer, _, _ := s.paired.Offer(pair.Full)
	_, token, _ := s.paired.Accept(offer, "laptop", "192.168.1.9")

	code := s.desk.link()

	w := ask(s, "POST", "/api/desk/enter", "192.168.1.9:4000", func(r *http.Request) {
		body(code)(r)
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), testDeskKey) {
		t.Errorf("a device elsewhere was answered %d: %s", w.Code, w.Body.String())
	}

	// A stranger elsewhere.
	if w := ask(s, "POST", "/api/desk/enter", "192.168.1.10:4000", body(code)); w.Code == 200 || strings.Contains(w.Body.String(), testDeskKey) {
		t.Errorf("a stranger was answered %d", w.Code)
	}

	// A page of another site, in a browser on this computer.
	w = ask(s, "POST", "/api/desk/enter", "127.0.0.1:5000", func(r *http.Request) {
		body(code)(r)
		r.Header.Set("Origin", "http://evil.test")
	})
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), testDeskKey) {
		t.Errorf("a page of another site was answered %d", w.Code)
	}

	// None of those used the link up: the window still gets in.
	if w := ask(s, "POST", "/api/desk/enter", "127.0.0.1:5000", body(code)); w.Code != 200 || !strings.Contains(w.Body.String(), testDeskKey) {
		t.Errorf("the window was refused its own link after others tried it (%d): %s", w.Code, w.Body.String())
	}
}

// What needs the key, what does not, and who is never asked for it.
func TestTheGuardAsksTheWindowsKeyOnlyWhereItMatters(t *testing.T) {
	s, got := guarded(t, config.ReachNetwork)

	key := func(k string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set(DeskHeader, k) }
	}

	for _, path := range []string{
		"/api/approvals/3/approve", "/api/approvals/3/deny", "/api/approvals/3/reject",
		"/api/approvals/3/always-program", "/api/outside/allow",
	} {
		for name, set := range map[string]func(*http.Request){
			"no key":           nil,
			"a wrong key":      key("not-the-key"),
			"a key cut short":  key(testDeskKey[:len(testDeskKey)-1]),
			"the key and more": key(testDeskKey + "0"),
		} {
			*got = false

			w := ask(s, "POST", path, "127.0.0.1:5000", set)

			if w.Code != http.StatusForbidden || *got {
				t.Errorf("%s with %s was let through (%d)", path, name, w.Code)
			}

			var said map[string]string
			json.Unmarshal(w.Body.Bytes(), &said)

			if said["desk"] != "not the window" || !strings.Contains(said["error"], "own window") {
				t.Errorf("%s with %s was refused without saying why: %s", path, name, w.Body.String())
			}
		}

		*got = false

		if w := ask(s, "POST", path, "127.0.0.1:5000", key(testDeskKey)); w.Code != 200 || !*got {
			t.Errorf("the window could not post to %s (%d)", path, w.Code)
		}

		// The key does not excuse a page of another site.
		*got = false

		w := ask(s, "POST", path, "127.0.0.1:5000", func(r *http.Request) {
			r.Header.Set(DeskHeader, testDeskKey)
			r.Header.Set("Origin", "http://evil.test")
		})
		if w.Code != http.StatusForbidden || *got {
			t.Errorf("a page of another site posted to %s with the key (%d)", path, w.Code)
		}
	}

	// Reading is not a decision, and the rest of the program is as it was:
	// the door other programs ask through, a conversation, the settings.
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/approvals"}, {"GET", "/api/outside"},
		{"POST", "/api/outside/ask"}, {"POST", "/api/chat"}, {"POST", "/api/settings"},
		{"POST", "/api/programs/stop"}, {"POST", "/api/terms"},
	} {
		*got = false

		if w := ask(s, c.method, c.path, "127.0.0.1:5000", nil); w.Code != 200 || !*got {
			t.Errorf("%s %s needs the window's key now (%d)", c.method, c.path, w.Code)
		}
	}

	// A device paired with every right answers a card with its token, as
	// before, and a device with fewer rights still does not.
	offer, _, _ := s.paired.Offer(pair.Full)
	_, full, _ := s.paired.Accept(offer, "laptop", "192.168.1.9")

	*got = false

	w := ask(s, "POST", "/api/approvals/3/approve", "192.168.1.9:4000", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+full)
	})
	if w.Code != 200 || !*got {
		t.Errorf("a device paired with every right can no longer answer a card (%d)", w.Code)
	}

	offer, _, _ = s.paired.Offer(pair.Limited)
	_, limited, _ := s.paired.Accept(offer, "phone", "192.168.1.8")

	*got = false

	w = ask(s, "POST", "/api/approvals/3/approve", "192.168.1.8:4000", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+limited)
		r.Header.Set(DeskHeader, testDeskKey)
	})
	if w.Code != http.StatusForbidden || *got {
		t.Errorf("a device with fewer rights answered a card by carrying the key (%d)", w.Code)
	}
}

// The window takes its key before the terms are accepted: the agreement is
// the first page it shows.
func TestTheWindowTakesItsKeyBeforeTheTerms(t *testing.T) {
	if !beforeTheTerms["POST /api/desk/enter"] {
		t.Error("the window cannot take its key until the terms are accepted")
	}
}
