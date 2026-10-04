---
name: run-app
description: Build PN Scripts Assistant from this checkout, serve it on loopback with a throwaway brain, and drive its interface in the browser pane (Models, System and the other panels). Use to see a change working in the real app, take a screenshot, or check what a view shows, without touching the owner's real brain or an instance they already have running.
---

# Run PN Scripts Assistant

The program is one Go binary. `serve` runs the whole brain and its interface
over HTTP on loopback, which is the easiest thing to drive; the native window
(`app`) shows the same pages through WebKitGTK.

## Start

```bash
RUN_DIR=<scratchpad>/pnsa-run .claude/skills/run-app/run.sh start
```

It builds `clients/cmd/pn-scripts-assistant`, prepares a scratch brain in
`$RUN_DIR/root` and serves it on `127.0.0.1:8799` (`PORT=` to change), then
waits until the page answers. It prints `ready: http://127.0.0.1:8799 (pid …)`.
`run.sh status` says whether it is up; `run.sh stop` stops it.

What the script takes care of, so it is not rediscovered:

- **A scratch brain, never the real one.** `PN_SCRIPTS_ASSISTANT_DATA_ROOT`
  points at `$RUN_DIR/root`. A root given this way is "borrowed" and is never
  remembered as the machine's brain. The folder needs a `.brain-root.json`
  marker or the program refuses it; the script writes one.
- **No naming card.** With no `brain.conf` the page opens on "Before we
  start", asking for a name. The script writes a two-line `brain.conf`
  (name "Assistant", no owner), so the page opens straight on the panels.
- **Another instance may be running.** The owner often has a release binary
  serving its own brain on another port (8791 at the time of writing). Leave
  it alone. The script refuses a port that is taken rather than reusing it.
- **Stopping by PID.** `stop` kills only the PID it started, and only if that
  PID is still this binary. Never `pkill -f` a pattern: it matches the agent's
  own shell.

On a fresh brain the Activity panel shows "importing onet" and "importing
esco" for a minute; that is the occupations import and harmless. Ollama must
be running on `127.0.0.1:11434` for the model lists; "Models you can install"
also fetches ollama.com's library, unless looking things up is switched off.

## Drive it

Open `http://127.0.0.1:8799/` in the browser pane and give it a few seconds:
a splash plays first.

The page is one panel at a time, named in the header between two arrows
(starts on "Command centre"). There is no URL per panel. **Click the arrows
by coordinate from a fresh screenshot** — at the pane's 800x600 frame the
right arrow is at about (547, 20). Clicking them by `ref` from `find` or
`read_page` reported success and did not change the panel. Read the header
in the next screenshot to see where you are.

Order going right from Command centre: **Models**, System, Operations, …
(left goes back). So Models is one click right of Command centre.

For checking what a panel holds without reading pixels, read the DOM, e.g.
the install list:

```js
const host = document.getElementById('catalogue-list');
({ rows: host.children.length, undefined: /undefined/i.test(host.innerText) })
```

Useful API routes to `curl` alongside: `/api/status`, `/api/models`,
`/api/models/available`.

## Stop

```bash
RUN_DIR=<scratchpad>/pnsa-run .claude/skills/run-app/run.sh stop
```

Use the same `RUN_DIR` as for start; the PID file lives there. Deleting
`$RUN_DIR` afterwards removes the scratch brain, its log and the binary.
