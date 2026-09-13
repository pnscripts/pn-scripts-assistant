/*
 * Package away is the window shown when the brain is somewhere unreachable.
 *
 * Its own tiny server for the same reason setup has one: this appears when the
 * brain cannot be opened, so the brain cannot be the thing that explains it.
 * Small on purpose — it says where everything is, offers to wait or to begin
 * again, and does nothing else.
 */
package away

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
)

type Server struct {
	listener net.Listener
	path     string

	mu    sync.Mutex
	fresh bool
}

func New(path string) (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	return &Server{listener: l, path: path}, nil
}

func (s *Server) URL() string { return "http://" + s.listener.Addr().String() }

// StartAgain reports whether the person chose to stop waiting for the old
// place. False when they closed the window, which means they are going to plug
// the drive in — the safe reading of doing nothing.
func (s *Server) StartAgain() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.fresh
}

func (s *Server) Serve(onDone func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, page, html.EscapeString(s.path))
	})

	mux.HandleFunc("/start-again", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.fresh = true
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)

		go onDone()
	})

	// Closing the window is the other answer, and needs no endpoint.
	_ = http.Serve(s.listener, mux)
}

const page = `<!doctype html>
<meta charset="utf-8">
<title>PN Scripts Assistant</title>
<style>
:root{--bg:#070a0f;--raised:#0d1219;--line:#1b2634;--text:#d6dee8;--fg:#eaf1f8;
--dim:#7d8b9c;--faint:#4a5769;--accent:#4dd0e1;--warn:#f0b26b}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);padding:36px 30px;
font:15px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
h1{font-size:19px;margin:0 0 6px;color:var(--accent);letter-spacing:.02em}
p{margin:0 0 14px;font-size:13.5px}
.where{font-family:ui-monospace,monospace;font-size:12px;color:var(--fg);
background:var(--raised);border:1px solid var(--line);border-radius:8px;
padding:11px 13px;margin:0 0 18px;word-break:break-all}
.dim{color:var(--dim);font-size:12.5px}
button{font:inherit;font-size:13px;font-weight:600;padding:9px 15px;border-radius:8px;
border:1px solid var(--line);background:transparent;color:var(--dim);cursor:pointer}
button:hover{border-color:var(--accent);color:var(--accent)}
.row{display:flex;gap:10px;margin-top:22px;align-items:center;flex-wrap:wrap}
.said{color:var(--warn);font-size:12.5px;margin-top:16px}
</style>

<h1>PN Scripts Assistant cannot reach what it knows</h1>

<p>Everything it has learned is kept here:</p>
<div class="where">%s</div>

<p><strong>If that is a drive, plug it in and start PN Scripts Assistant again.</strong>
Nothing is lost — it is found by being there, not by being remembered.</p>

<p class="dim">PN Scripts Assistant will not make a new brain on its own while it knows about
that one, because a new one would be empty and would look exactly like having
forgotten you.</p>

<div class="row">
  <button id="again">That place is gone — begin again</button>
  <span class="dim">or close this window and plug the drive in</span>
</div>

<p class="said" id="said" hidden>PN Scripts Assistant will stop waiting for it. Start the
program again to make a new, empty brain. Nothing at the old place is deleted.</p>

<script>
document.getElementById("again").onclick = async () => {
  await fetch("/start-again", {method: "POST"});
  document.getElementById("again").disabled = true;
  document.getElementById("said").hidden = false;
};
</script>
`
