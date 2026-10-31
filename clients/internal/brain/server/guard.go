package server

import (
	"net"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/pair"
)

/*
 * Who is allowed to reach this, now that it is not only this machine.
 *
 * For the whole life of this program the answer was the loopback interface:
 * nothing else could connect to it, so a connection was a proof of identity
 * and there was nothing to check. That was correct and it is worth saying
 * plainly that it stops being correct the moment the listener binds wider —
 * at which point the memory, the mail, the files and the ability to run a
 * command are behind a door with no lock.
 *
 * Two rules, and only two:
 *
 * A request that arrived on loopback came from this computer. Nothing else can
 * put a packet on it, so this is not a trust decision, it is a fact about how
 * networking works.
 *
 * Anything else must present a token it was given in person, by somebody
 * reading a code off the screen of the computer the brain is on.
 */

// TokenCookie is where a browser keeps its device token. Strict, so a page on
// another site cannot make a request that carries it.
const TokenCookie = "pn-device"

// Caller is who is asking.
type Caller struct {
	// Here is true when the request came from this machine.
	Here bool

	Device pair.Device
	Known  bool
}

// Can reports whether this caller may do everything, or only talk and watch.
func (c Caller) Can() string {
	if c.Here {
		return pair.Full
	}

	return c.Device.Can
}

/*
 * openToEveryone is what must work before anybody can be recognised.
 *
 * Kept to the smallest possible set. Health, because something has to be able
 * to ask whether the program is up without holding a key. The pairing routes,
 * because that is how a device stops being a stranger. And the page and its
 * assets, because a device with no token has to be able to load the screen
 * that asks for the code — it gets the pairing screen and nothing else, since
 * every route behind it is guarded.
 */
func openToEveryone(path string) bool {
	switch {
	case path == "/health":
		return true
	case path == "/api/pair/claim":
		return true
	case path == "/" || !strings.HasPrefix(path, "/api/"):
		return true
	default:
		return false
	}
}

/*
 * atTheDeskOnly are the things a device away from the computer may not do.
 *
 * The line is one question: may this make the decisions? A phone on a train is
 * a fine place to ask the assistant something and a poor place to approve it
 * deleting files — and somebody who loses that phone should not have lost the
 * gate along with it.
 *
 * Matched by prefix, so a route added under one of these is covered without
 * anybody having to remember. The default for anything new is allowed, which
 * is the right way round for a list about decisions rather than about data:
 * forgetting to add a new settings route is a mistake, and one this list makes
 * visible by being short enough to read.
 */
var atTheDeskOnly = []string{
	"/api/approvals/",  // approving what the assistant wants to do
	"/api/permissions", // what it may do without asking
	"/api/privacy",     // what may leave this machine
	"/api/protection",  // what it must not read
	"/api/settings",    // how it behaves
	"/api/pair",        // pairing another device, or unpairing this one
	"/api/reach",       // whether it is reachable at all
	"/api/places",      // what it is allowed to learn from
	"/api/home",        // where its memory lives
	"/api/parts",       // installing and removing pieces of the machine

	/*
	 * Hiring, firing and rearranging the organisation.
	 *
	 * A decision by any reading: it changes who may do what, and it outlives
	 * the conversation that made it. Reading the chart is deliberately not on
	 * this list — a phone on a train is a fine place to look up who does what,
	 * and a poor place to give somebody the ability to run commands.
	 */
	"/api/organisation/units",
	"/api/organisation/agents",
	"/api/organisation/hire",
	"/api/organisation/import", // bringing in a classification, and stopping one

	/*
	 * Installing software, starting a project in somebody's folders, and
	 * switching an integration on or giving it a credential. Each is the
	 * owner's own approval, given by pressing the button — which is exactly
	 * why a device away from the computer may not press it.
	 */
	"/api/provision/",
	"/api/projects/",
	"/api/integrations/",
	"/api/resources/",  // how work is done: the policy, what may be installed and spent
	"/api/models/",     // changing which model answers
	"/api/upkeep/self", // updating the program

	// Writing a translated file into somebody's folders.
	"/api/translate/file",

	// Sending a message in its owner's name.
	"/api/mail/send/",
}

/*
 * changedAtTheDeskOnly are routes a device away from the computer may read but
 * not post to.
 *
 * Kept apart from atTheDeskOnly because reading them from a phone is fine —
 * looking at a draft, a goal, the list of skills — while changing them is a
 * decision: where the owner's mail password is sent, a file written or read
 * into the brain by its path on this computer, unattended work started, a
 * credential or a connection given, what the program is allowed to learn.
 */
var changedAtTheDeskOnly = []string{
	"/api/mail",            // the mailbox, its servers, drafts waiting to be sent
	"/api/tables/look",     // reads a spreadsheet by its path here
	"/api/tables/save",     // writes a file into the owner's folders
	"/api/forms/",          // reads and fills a PDF by its path here
	"/api/books/read",      // reads a file by its path here
	"/api/books/import",    // the same, into the books
	"/api/diary/import",    // the same, into the diary
	"/api/glossary/import", // the same, into the glossary
	"/api/goals",           // a goal, and unattended work towards it
	"/api/skills",          // what it is taught and may load
	"/api/open-source",     // opening a file or folder on this computer
	"/api/desktop",         // the applications menu entry
	"/api/environment/",    // what it may notice around it
	"/api/copies",          // where copies of the brain go
	"/api/tunnel",          // being reachable from the internet
	"/api/devices/",        // connecting another brain or device
	"/api/providers/",      // a hosted model and its key
	"/api/team/",           // what a specialist may use
	"/api/lessons/",        // lessons it keeps for good
	"/api/voiceprint/",     // whose voice it answers
	"/api/vault/",          // the recovery key
	"/api/journey/",        // its record of what happened
	"/api/organisation/",   // the organisation, every change
}

func decisionRoute(path string) bool {
	for _, prefix := range atTheDeskOnly {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// decision is decisionRoute, plus the routes that are decisions only when
// something is being changed.
func decision(r *http.Request) bool {
	if decisionRoute(r.URL.Path) {
		return true
	}

	if !changesSomething(r) {
		return false
	}

	for _, prefix := range changedAtTheDeskOnly {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}

	return false
}

/*
 * Guard decides every request before anything else sees it.
 *
 * Wrapped around the whole mux rather than added per route, because a rule
 * that has to be remembered on each of ninety routes is a rule that will be
 * missing from the ninety-first.
 */
func (s *Server) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/*
		 * No answer of the API is kept by the window's own cache.
		 *
		 * The brain is sealed on disk; WebKit's and WebView2's HTTP caches are
		 * not, and for the portable copy the WebView2 folder sits on the same
		 * stick. A conversation kept there in the clear would undo the seal.
		 */
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}

		who := s.whoIsAsking(r)

		if who.Here {
			/*
			 * On this computer is not the same as from this program.
			 *
			 * Every web page the owner opens can send a request to
			 * 127.0.0.1, and the browser sends it from this machine, so it
			 * arrives here looking exactly like the assistant's own page.
			 * This was allowed through: a page on any site could post to
			 * /api/desktop, /api/settings or an approval and the brain would
			 * do it. Demonstrated, not imagined — a POST from
			 * "Origin: http://evil.test" with a form's content type took the
			 * program out of the applications menu.
			 *
			 * Two locks, because each covers what the other cannot.
			 *
			 * The name it was asked by, first. A page can have its own name
			 * resolve to 127.0.0.1 — it is its own domain and it may point it
			 * anywhere — and then the browser calls this same origin and the
			 * page may read every answer. What it cannot forge is the name in
			 * the request: a browser sends the one it was given. From this
			 * machine, that name has to be this machine.
			 */
			if !askedByThisMachinesName(r.Host) {
				fail(w, http.StatusForbidden,
					"this assistant answers to localhost, not to "+hostOnly(r.Host))

				return
			}

			// And then where the page came from, for anything that changes
			// something. A request that says nothing came from a program
			// rather than a browser, and is allowed; see fromHere.
			if changesSomething(r) && !fromHere(r) {
				fail(w, http.StatusForbidden, "that request came from somewhere else")

				return
			}

			next.ServeHTTP(w, r)

			return
		}

		/*
		 * A request from elsewhere, when nothing has been opened up.
		 *
		 * It should not be possible to arrive here — the listener refuses to
		 * bind anywhere but loopback unless reach is on — so this is the
		 * second of two locks rather than the first. Belt and braces on the
		 * one thing in this program where being wrong is unrecoverable.
		 */
		if !s.brain.Cfg.OpenToNetwork() {
			http.Error(w, "this assistant only answers on the computer it runs on", http.StatusForbidden)

			return
		}

		if who.Known {
			s.paired.Seen(who.Device.ID, fromAddress(r))

			if who.Device.Can != pair.Full && decision(r) {
				fail(w, http.StatusForbidden,
					"this device can talk to it and watch it, but decisions are made at the computer")

				return
			}

			/*
			 * And a page on another site cannot drive it with this cookie.
			 *
			 * SameSite keeps a browser from sending the cookie from another
			 * origin at all, and this is the belt to that brace: a request
			 * that says where it came from must have come from here.
			 */
			if changesSomething(r) && !fromHere(r) {
				fail(w, http.StatusForbidden, "that request came from somewhere else")

				return
			}

			next.ServeHTTP(w, r)

			return
		}

		if openToEveryone(r.URL.Path) {
			next.ServeHTTP(w, r)

			return
		}

		fail(w, http.StatusUnauthorized, "this device is not paired with this assistant")
	})
}

// whoIsAsking works out who is on the other end, without deciding anything.
func (s *Server) whoIsAsking(r *http.Request) Caller {
	if cameFromThisMachine(r.RemoteAddr) {
		return Caller{Here: true}
	}

	if s.paired == nil {
		return Caller{}
	}

	if device, known := s.paired.Who(tokenFrom(r)); known {
		return Caller{Device: device, Known: true}
	}

	return Caller{}
}

// tokenFrom reads the token a device presented, from a header or a cookie.
// Both, because a browser has cookies and everything else has headers.
func tokenFrom(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		if token, found := strings.CutPrefix(header, "Bearer "); found {
			return strings.TrimSpace(token)
		}
	}

	if cookie, err := r.Cookie(TokenCookie); err == nil {
		return cookie.Value
	}

	return ""
}

/*
 * cameFromThisMachine is a fact about networking rather than a trust decision.
 *
 * Nothing outside this computer can put a packet on the loopback interface, so
 * a request that arrived on it came from here. Note what is deliberately not
 * consulted: X-Forwarded-For and every header like it, because a header is
 * written by whoever sent the request and trusting one would let anybody claim
 * to be this machine by saying so.
 */
func cameFromThisMachine(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))

	return ip != nil && ip.IsLoopback()
}

func fromAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

/*
 * askedByThisMachinesName says whether the Host header names this computer.
 *
 * Only for callers that are already on it. A device across the room reaches
 * this by the machine's own name or address and has its own rules — pairing,
 * a token, a cookie. This is the one case where the caller is trusted for
 * being local, and so the name has to be local too.
 */
func askedByThisMachinesName(host string) bool {
	name := hostOnly(host)

	switch strings.ToLower(name) {
	case "localhost", "":
		// Empty is HTTP/1.0 or a program that sent no Host at all, which is
		// not a browser and cannot be a page.
		return true
	}

	if ip := net.ParseIP(strings.Trim(name, "[]")); ip != nil {
		return ip.IsLoopback()
	}

	// A name that is not an address and not localhost: somebody's domain
	// pointed here.
	return false
}

// hostOnly is the Host header without its port.
func hostOnly(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		return name
	}

	return host
}

func changesSomething(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// fromHere reports whether a browser says the request began on this
// assistant's own page. A request that says nothing is allowed: everything
// that is not a browser says nothing, and this is a rule about browsers.
func fromHere(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	if origin == "" {
		return true
	}

	return sameHost(origin, r.Host)
}

func sameHost(origin, host string) bool {
	trimmed := origin

	for _, prefix := range []string{"https://", "http://"} {
		if rest, found := strings.CutPrefix(origin, prefix); found {
			trimmed = rest

			break
		}
	}

	return trimmed == host
}
