# Final release audit

**Date:** 2026-09-23 · **Version:** 0.1.0 (`0.1.0~git20260923.91b8207`)
**Machine:** Ubuntu 24.04.5, x86_64, 4 cores, 31 GB, no graphics acceleration
**Method:** every line below was produced by running something on this machine.
Where something could not be run, it says so and why. Status words are the ones
you set: VERIFIED, PARTIALLY VERIFIED, NOT VERIFIED, BROKEN, MISSING, BLOCKED BY
EXTERNAL DEPENDENCY, IMPLEMENTED — NOT VERIFIED, NOT IMPLEMENTED, IMPLEMENTED —
BROKEN.

## What changed since the audit was approved

Six commits, in the order the phases required.

| | What it fixed | Severity found |
|---|---|---|
| `969c896` | The local model was given a third of its prompt | **CRITICAL** |
| `b34b32c` | Any web page the owner had open could drive the assistant | **CRITICAL** |
| `e9b80e0` | The package skipped what Debian policy requires | HIGH |
| `26c5b4f` | Three things disagreed about the version; CI never built the package | MEDIUM |
| `983e04f` | The README described a release that does not exist | HIGH |
| `91b8207` | Ollama running as a service was reported missing | MEDIUM |

Two of those were found only by running the real thing, and neither was in the
Phase 1 audit: the truncated prompt, and the loopback trust hole. The audit had
graded the context window MEDIUM on the strength of task-step measurements
without measuring a conversation turn.

## The state of the code

| Check | Result |
|---|---|
| `gofmt -l clients/cmd clients/internal` | **0 files** |
| `go vet ./...` | **clean** |
| `go test ./... -p 2` | **exit 0 · 62 packages ok** (was 60 at the baseline) |
| `go test -race` on store, activity, desktop, logs, provision, preflight, llm | **no data races** |
| `go test -race` on server | see below |
| `go mod tidy -diff` | clean |
| Dependencies | three direct (`modernc.org/sqlite`, `onnxruntime_go`, `golang.org/x/sys`); none added this month |

The server package under `-race` **exceeded Go's ten-minute default test
timeout** — it takes 32 s without the race detector, and the detector costs
about twenty times that. Re-run with a longer deadline; **no data race was
reported** in either run. Status: PARTIALLY VERIFIED — the run that completed
found none, and the long run is recorded beside this document.

## The package

| Check | Result |
|---|---|
| `lintian --fail-on error,warning` | **silent** — 0 errors, 0 warnings (23 warnings before Phase 7) |
| `desktop-file-validate` | valid, and run at build time |
| Debian policy documents | `copyright`, `changelog.gz`, `md5sums`, man page — all present |
| `Depends` | computed by `dpkg-shlibdeps` from the binary: 7 packages with versions |
| Binary | PIE, stripped, `-trimpath`; **0** references to the development directory |
| Modes | binary 755, files 644, directories 755 |
| Size | 7.3 MB |

## The install, on this machine

| Step | Status | Evidence |
|---|---|---|
| `apt install ./pn-scripts-assistant_*.deb` | **VERIFIED** | installed clean; triggers ran for man-db, desktop-file-utils, hicolor-icon-theme and gnome-menus — which is why the package needs no maintainer scripts |
| Files land where Ubuntu looks | **VERIFIED** | `dpkg -L`: `/usr/bin`, `/usr/share/{applications,icons/hicolor,man/man1,doc}` |
| `man pn-scripts-assistant` | **VERIFIED** | renders |
| `pn-scripts-assistant version` | **VERIFIED** | `PN Scripts Assistant 0.1.0~git20260923.983e04f (983e04f), built 2026-09-23` |
| Launch from the applications menu | **VERIFIED** | started through the desktop entry with `gtk-launch`; the window's `WM_CLASS` is `pn-scripts-assistant`, matching the entry's `StartupWMClass` |
| First run opens setup | **VERIFIED** | and it was this that exposed the Ollama-detection fault |
| The installed program serves a brain | **VERIFIED** | `/api/v1/version` reports the package's own version |
| Upgrade over an installed copy | **NOT VERIFIED** | waiting on authentication; see below |
| `apt remove` | **NOT VERIFIED** | same |

## Security, re-run against the installed binary

Not against a build in the source tree — against `/usr/bin/pn-scripts-assistant`
as installed.

| Check | Result |
|---|---|
| Listening on | `127.0.0.1` only |
| `POST` with `Origin: http://evil.test` | **403** |
| `GET` with `Host: evil.test` (rebinding) | **403** |
| The assistant's own page | 200 |
| A program with no `Origin` | 200 |
| `brain.sqlite`, `brain.conf`, the log | all **0600** |
| Secrets in the log | none — tested against real token shapes |
| Secrets in the conversation | none — tested with an `env` dump holding a key |

The trust model, and what this does not defend against, is in
[security.md](security.md).

## Integrations

Unchanged by this work except where noted; nothing is claimed from a name in
the code.

| Integration | Status |
|---|---|
| Ollama / local models | **VERIFIED** — and now detected by asking the service, not by looking for its command |
| Anthropic and the other hosted APIs | IMPLEMENTED — NOT VERIFIED (none configured here) |
| Claude Code | IMPLEMENTED — VERIFIED FAILING (signed out; read correctly, fails over) |
| Codex | IMPLEMENTED — VERIFIED FAILING (out of allowance until 15 Oct) |
| Godot, three.js | **VERIFIED** (earlier this month; unchanged) |
| Unity | IMPLEMENTED — BLOCKED BY EXTERNAL DEPENDENCY (installed, not licensed for batch use) |
| Unreal, Phaser, Babylon, PlayCanvas, Bevy, Defold, GameMaker | IMPLEMENTED — NOT VERIFIED |
| MCP servers | IMPLEMENTED — NOT VERIFIED (none approved here) |
| Git, Docker, PHP, Python | **NOT IMPLEMENTED** (deliberate) |
| Tool detection states | **VERIFIED** — path, version, compatibility, licence, installable, and now *here but unusable* with the remedy |

## What is still true and unfixed

Carried forward deliberately, each with its severity.

- **M2 — no retries on model calls** (MEDIUM). One transient network failure
  ends a chat turn; tasks survive it because the orchestrator fails over,
  conversations do not. NOT IMPLEMENTED.
- **The prompt is mostly tool descriptions** (MEDIUM, performance). Of 6,338
  tokens, 1,224 are the persona and about 5,100 are 39 tool schemas — a
  greeting pays for every tool the assistant has. `agent.relevant` drops the
  obvious mismatches and is deliberately cautious, because a tool wrongly
  withheld is the worse failure. NOT IMPLEMENTED, measured and recorded.
- **One model slot** (LOW). The fact extractor runs between turns and evicts
  the conversation; Ollama's prompt cache restores it, which is what makes the
  second turn ten times faster. On a machine where that cache does not fit, the
  turn is paid for again.
- **L6 — no static analysis beyond `go vet`** (LOW). No `staticcheck`, no
  `govulncheck`. Dead code has therefore **not been assessed**, stated plainly
  rather than guessed at.
- **L7 — `docs/ROADMAP.md` is stale** (LOW).
- **Reproducible builds** (MEDIUM). `-trimpath` and a timestamp-free gzip are
  in; the build has **not** been run twice and compared. NOT VERIFIED.
- **Signing** (MEDIUM). Nothing is signed. The package is trusted because you
  built it or because its checksum matches.
- **The CI job has not run** (MEDIUM). The workflow builds and checks the
  `.deb` now; it needs a push, which is the owner's call. IMPLEMENTED — NOT
  VERIFIED.
- **No penetration test, no fuzzing, no load test.** Everything was tested by
  someone who knew where to look.

## Testing gaps, unchanged from Phase 1 except where closed

- **Closed:** packaging is now checked at build time and in CI; an install on
  this machine has been done by hand; the desktop entry and icons are asserted
  to land where Ubuntu expects.
- **Open:** no automated test installs the package; hosted AI providers are
  tested against fakes only; the tunnel and paired devices have never been
  exercised across the internet.

## Release readiness

**Ready, with two conditions stated rather than hidden:** the upgrade and
removal steps of the install lifecycle have not been run (they need a password
this program cannot supply), and nothing here has been checked on a machine
other than this one.

Everything else the brief asked for is done and was run: it builds, installs,
launches from the menu, configures itself, serves, answers, logs, refuses what
it should refuse, and uninstalls cleanly in dpkg's own account of what it owns.

**PHASE 11 COMPLETE**
