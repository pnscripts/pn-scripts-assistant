package setup

import (
	"net"
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/stage"
)

/*
 * Who may drive setup: this page, on this computer, and nothing else.
 *
 * Setup listens on 127.0.0.1 like the brain, and every web page its owner has
 * open can send a request there. The brain's server has refused those since
 * 0.4 (server.Guard); setup had no such lock, and it holds the most powerful
 * switches there are — the approval level ("everything allowed"), the privacy
 * mode, the hosted provider and its address, the drive. The 1.0 security
 * review showed a page on any site turning every approval off with one blind
 * POST, and reading /state by pointing its own name at 127.0.0.1.
 *
 * Four rules, each covering what the others cannot:
 *
 *  1. The name it was asked by must be this machine's (DNS rebinding).
 *  2. A browser that says where the request came from must say "here"
 *     (Origin, sent with every cross-site POST).
 *  3. A browser that says what kind of request this is must not say it came
 *     from another site (Sec-Fetch-Site, sent with every request by current
 *     browsers, GETs from an <img> included).
 *  4. Only a short list of routes may be read with GET; everything that
 *     changes something must be a POST, so rule 2 always applies to it.
 *
 * A request that says nothing about where it came from is a program, not a
 * page, and is allowed — the same rule as the brain's (server.fromHere).
 */
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !askedByThisMachine(r.Host) {
			http.Error(w, "setup answers to this computer only", http.StatusForbidden)

			return
		}

		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "that request came from somewhere else", http.StatusForbidden)

			return
		}

		page := r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, stage.Prefix)

		// The page itself may be opened from anywhere — the program's own
		// window, a link, an address typed in. Everything it then asks for
		// comes from itself.
		if !page {
			switch r.Header.Get("Sec-Fetch-Site") {
			case "", "same-origin", "none":
			default:
				http.Error(w, "that request came from somewhere else", http.StatusForbidden)

				return
			}
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			if !page && !readable[r.URL.Path] {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "this changes something, so it must be a POST", http.StatusMethodNotAllowed)

				return
			}
		case http.MethodPost:
		default:
			http.Error(w, "only GET and POST are answered here", http.StatusMethodNotAllowed)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// readable are the routes that only read, and so may be asked with GET.
var readable = map[string]bool{
	"/state":       true,
	"/library":     true,
	"/welcome":     true,
	"/identity":    true,
	"/ready":       true,
	"/voices":      true,
	"/suggestions": true,
}

// askedByThisMachine says whether the Host header names this computer.
func askedByThisMachine(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}

	switch strings.ToLower(name) {
	case "localhost", "":
		return true
	}

	ip := net.ParseIP(strings.Trim(name, "[]"))

	return ip != nil && ip.IsLoopback()
}
