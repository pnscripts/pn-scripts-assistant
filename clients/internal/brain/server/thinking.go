package server

import (
	"net/http"
	"time"
)

/*
 * Holding a connection open while a model thinks.
 *
 * The server sets a fifteen-minute write deadline, which is generous for a
 * web server and not generous enough for this one: measured on this machine,
 * a first conversation turn that reaches for a tool took **twenty-eight
 * minutes** — four cores, no graphics card, a browser open. The deadline cut
 * the connection at fifteen, so the person asking got an empty reply while
 * the assistant finished the work, wrote the answer into the conversation and
 * stopped for approval exactly as it should. The worst shape a fault can
 * have: everything worked and nobody was told.
 *
 * Raising the server's own timeout would apply it to every connection,
 * including the ones that are merely stuck. The deadline belongs to the few
 * requests that wait on a model, so it is set on those.
 */

// Thinking is how long a request that waits on a model may hold its
// connection. Long enough for the slowest turn measured here, with room:
// a machine slower than this one exists, and an answer arriving late is
// better than no answer at all.
const Thinking = time.Hour

/*
 * letItThink gives this one request until Thinking to produce its answer.
 *
 * Quiet when the deadline cannot be set: a connection that does not support
 * one is a connection with no deadline to exceed, and refusing to answer
 * because the clock could not be moved would be worse than the fault this
 * fixes.
 */
func letItThink(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(Thinking))
}
