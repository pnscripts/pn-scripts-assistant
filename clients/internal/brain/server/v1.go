package server

import (
	"net/http"
	"runtime"
	"strconv"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/protocol"
)

/*
 * The versioned API, which is the one a program other than this page talks to.
 *
 * The routes under /api are this program's own interface talking to itself:
 * they change when the page changes, and nothing else was ever meant to hold
 * them. A phone, a window on another machine and — later — an execution node
 * are a different matter. They are updated on different days from the brain
 * they talk to, so what they rely on has to be a promise with a number on it.
 *
 * Hence /api/v1, added beside the old routes rather than instead of them.
 * Nothing moves until the thing that reads it does.
 */

// handleV1Version is what this program is and what it can speak, which is the
// first thing anything connecting should ask.
func (s *Server) handleV1Version(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"product":  config.Product,
		"version":  brainPart().Version,
		"protocol": protocol.Version,
		"platform": runtime.GOOS + "/" + runtime.GOARCH,

		// Where a client with no history of its own starts listening, so that
		// it can ask for everything after this and miss nothing in between.
		"activity": s.brain.Happens.Latest(),
	})
}

/*
 * handleV1Activity is what happened, after a number.
 *
 * The number, not a time: two things can happen in the same second, and a
 * client that asked for "since 12:04:31" would be told about one of them
 * twice and the other never. Whoever is catching up says the last number it
 * saw and is given what came after it, in order.
 */
func (s *Server) handleV1Activity(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.ParseInt(orElse(r.URL.Query().Get("since"), "0"), 10, 64)
	if err != nil || after < 0 {
		fail(w, http.StatusBadRequest, "since must be a number, which is the last one you saw")

		return
	}

	most, _ := strconv.Atoi(r.URL.Query().Get("most"))

	events, err := s.brain.Happens.Since(after, most)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"protocol": protocol.Version, "events": events, "latest": s.brain.Happens.Latest()})
}
