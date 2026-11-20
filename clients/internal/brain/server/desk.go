package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/http"
	"strings"
	"sync"
)

/*
 * The assistant's own window, told apart from every other program here.
 *
 * A request on the loopback interface came from this computer, and for the
 * whole life of this program that was enough to make a decision: the page
 * posts to /api/approvals/<id>/approve and the action runs. Since other
 * programs can put their own permission questions on a card (1.2.0), it is
 * not enough any more. The program that asked can post the answer to the
 * same address itself, with no page at all, and so can anything else running
 * under the same account. The guard cannot tell them from the window: a
 * request that names no page is exactly what a program sends.
 *
 * So the window holds something a program that only posts does not. Each run
 * makes one key, which this program keeps in memory only. The program hands its own
 * window a link that works once; the page behind the link swaps the code for
 * the key, keeps it in the tab's session storage (which belongs to this
 * address and this tab, and unlike a cookie is not shared with other ports of
 * this host; where the web view keeps that storage is its own business, and
 * some keep it in their profile folder until the window closes), and sends it
 * as a header with everything it posts. The routes in
 * fromTheWindowOnly refuse a post from this computer without it.
 *
 * What this does not do, said here because the limit is real: a program
 * that reads another program's memory or its browser profile, or that types
 * and clicks in the window as a person would, is not stopped by a key. It
 * stops a program answering by itself with one request, which is what the
 * review of 1.2.0 found.
 */

// DeskHeader carries the window's key on what it posts.
const DeskHeader = "X-PN-Desk"

// mostDeskLinks is how many unused links wait at once: the window's, and a
// few for a person who asks `serve` for one. An older one stops working when
// a newer one would be one too many.
const mostDeskLinks = 4

// fixedDeskKey is set by this package's tests and by nothing else: a server
// made while it is set uses it as its key, so a test can post as the window.
var fixedDeskKey string

type desk struct {
	mu    sync.Mutex
	key   string
	codes []string
}

func newDesk() *desk {
	if fixedDeskKey != "" {
		return &desk{key: fixedDeskKey}
	}

	return &desk{key: randomWord()}
}

// randomWord is 256 bits from the system, in hex. With no randomness there
// is no key worth having, and a program that cannot keep its own decisions
// to itself should not start.
func randomWord() string {
	raw := make([]byte, 32)

	if _, err := rand.Read(raw); err != nil {
		panic("no randomness for the window's key: " + err.Error())
	}

	return hex.EncodeToString(raw)
}

// link makes a code that can be swapped for the key once.
func (d *desk) link() string {
	code := randomWord()

	d.mu.Lock()
	defer d.mu.Unlock()

	d.codes = append(d.codes, code)

	if len(d.codes) > mostDeskLinks {
		d.codes = d.codes[len(d.codes)-mostDeskLinks:]
	}

	return code
}

// enter swaps a waiting code for the key. Once: the code is gone afterwards,
// whoever used it.
func (d *desk) enter(code string) (string, bool) {
	if code == "" {
		return "", false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for i, waiting := range d.codes {
		if subtle.ConstantTimeCompare([]byte(waiting), []byte(code)) == 1 {
			d.codes = append(d.codes[:i:i], d.codes[i+1:]...)

			return d.key, true
		}
	}

	return "", false
}

// holds is whether a request carried this run's key.
func (d *desk) holds(key string) bool {
	return key != "" && subtle.ConstantTimeCompare([]byte(d.key), []byte(key)) == 1
}

/*
 * fromTheWindowOnly are the posts that need the window's key when they come
 * from this computer.
 *
 * Short on purpose, and exactly what another program gains by answering for
 * the person: the answer on a card (its own permission question among them),
 * and the switches for what other programs may ask. A device that was paired
 * with every right is not asked for the key: it proved who it is with its
 * token, it cannot reach the switches at all (onThisComputerOnly), and it
 * answers only the assistant's own cards (handleDecision).
 */
var fromTheWindowOnly = []string{
	"/api/approvals/",
	"/api/outside/allow",

	/*
	 * And the ways to become "a device elsewhere", which is not asked for
	 * the key: a pairing code, and opening the assistant to the network or
	 * the internet. Without these a program here could pair itself with
	 * every right and come back by this computer's own network address
	 * (found by the review of this change). Taking a device away, and
	 * claiming a code somebody was shown, stay as they were.
	 */
	"/api/pair/offer",
	"/api/reach",
	"/api/tunnel",
}

func windowOnly(r *http.Request) bool {
	if !changesSomething(r) {
		return false
	}

	for _, prefix := range fromTheWindowOnly {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}

	return false
}

// notTheWindow is the refusal, with a word the page can say in its own
// language beside the English.
func notTheWindow(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "That is decided in the assistant's own window. This request did not come from it: " +
			"open PN Scripts Assistant and answer there.",
		"desk": "not the window",
	})
}

/*
 * DeskLink is the address the program's own window opens first.
 *
 * It works once. Whoever owns the window calls this and gives the result
 * straight to the window, inside the program; nothing serves it. Printed or
 * passed to a browser on a command line it can be seen by other programs for
 * as long as it is unused, which is why the native window is the strong case
 * and a browser the weaker one (docs/guide/known-limits.md).
 */
func (s *Server) DeskLink(base string) string {
	return strings.TrimRight(base, "/") + "/desk/" + s.desk.link()
}

// handleDeskPage is the page behind the link. It changes nothing and knows
// nothing: its script posts the code, and only that answer carries the key.
func (s *Server) handleDeskPage(w http.ResponseWriter, r *http.Request) {
	words := deskWords["en"]
	language := "en"

	if uiLanguage(s.brain.Cfg.Language) == "bg" {
		words = deskWords["bg"]
		language = "bg"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	page := strings.NewReplacer(
		"{language}", language,
		"{opening}", html.EscapeString(words.Opening),
		"{failed}", html.EscapeString(words.Failed),
		"{what}", html.EscapeString(words.What),
		"{go}", html.EscapeString(words.Go),
	).Replace(deskPage)

	w.Write([]byte(page))
}

// handleDeskEnter swaps the link's code for the key.
func (s *Server) handleDeskEnter(w http.ResponseWriter, r *http.Request) {
	// The guard already keeps devices elsewhere away from this route; said
	// again here because this is the one answer that carries the key.
	if !s.whoIsAsking(r).Here {
		fail(w, http.StatusForbidden, "only the assistant's own window asks here")

		return
	}

	var body struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "That is not a link's code.")

		return
	}

	key, known := s.desk.enter(body.Code)
	if !known {
		// Worth a line in the log: a link used twice means either a page
		// loaded again, or somebody else got there first.
		s.log.Warn("a link for the window was used a second time, or was never given")

		fail(w, http.StatusForbidden, "This link was used already, or is from an earlier start.")

		return
	}

	ok(w, map[string]string{"key": key})
}

type deskSaid struct {
	Opening, Failed, What, Go string
}

// deskWords is what the page behind the link says, for the moment it is
// seen at all and for when the link no longer works.
var deskWords = map[string]deskSaid{
	"en": {
		Opening: "Opening PN Scripts Assistant.",
		Failed:  "This link no longer works.",
		What: "It works once, and only until PN Scripts Assistant is closed. Everything else is here as usual. " +
			"Answers on the cards under “Waiting for you” and the switches for what other programs may ask " +
			"are given in the assistant’s own window. If this is that window, close PN Scripts Assistant and open it again.",
		Go: "Continue",
	},
	"bg": {
		Opening: "PN Scripts Assistant се отваря.",
		Failed:  "Тази връзка вече не работи.",
		What: "Тя работи еднократно и само докато PN Scripts Assistant е отворен. Всичко останало е тук както обикновено. " +
			"Отговорите по картите в „Чака вашето решение“ и превключвателите за това какво могат да питат другите програми " +
			"се дават в собствения прозорец на асистента. Ако това е този прозорец, затворете PN Scripts Assistant и го отворете отново.",
		Go: "Продължи",
	},
}

const deskPage = `<!doctype html>
<html lang="{language}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>PN Scripts Assistant</title>
<style>
html { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0e1116; color: #e8eaee;
       font: 16px/1.5 system-ui, sans-serif; }
main { max-width: 34rem; padding: 1.5rem; }
h1 { font-size: 1.25rem; margin: 0 0 .75rem; }
p { margin: 0 0 1rem; }
a { display: inline-block; padding: .6rem 1.1rem; border-radius: .5rem; background: #2f6fed; color: #fff;
    text-decoration: none; }
[hidden] { display: none; }
</style>
</head>
<body>
<main>
<p id="desk-opening">{opening}</p>
<div id="desk-failed" hidden>
<h1>{failed}</h1>
<p>{what}</p>
<a href="/">{go}</a>
</div>
</main>
<script src="/js/desk-enter.js"></script>
</body>
</html>
`
