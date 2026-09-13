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
	"/api/models/",             // changing which model answers
	"/api/upkeep/self",         // updating the program
}

func decisionRoute(path string) bool {
	for _, prefix := range atTheDeskOnly {
		if strings.HasPrefix(path, prefix) {
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
		who := s.whoIsAsking(r)

		if who.Here {
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

			if who.Device.Can != pair.Full && decisionRoute(r.URL.Path) {
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
