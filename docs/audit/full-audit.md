# Full technical audit

**Date:** 2026-09-23 · **Commit:** `8d9c69f` · **Machine:** Ubuntu 24.04.5, x86_64, Go 1.26.0
**Method:** everything below was run on this machine. Where something could not
be run, it says so and why.

## What was run

| Check | Result |
|---|---|
| `gofmt -l ./internal ./cmd` | 0 files |
| `go vet ./...` | clean |
| `go test ./... -p 2` | exit 0 · 60 packages ok · 7 without tests |
| `go test -race` on the 8 concurrent packages | **exit 0 · no data races** (tasks, brain, activity, jobs, lanes, server, store, agent) |
| `go mod tidy -diff` | metadata drift only (see L1) |
| `scripts/build-packages.sh deb` | a real 7.0 MB `.deb` in 1m47s |
| `dpkg-deb --info` / `--contents` | inspected file by file |
| Packaged binary, fresh `HOME`, outside the repository | `help`, `status`, `serve` all work; creates only XDG paths |
| Port already in use | fails, exit 1, raw Go message |
| No model service (`OLLAMA_BASE_URL` → dead port) | degrades, no crash, raw Go message |
| Package installed / removed | **not yet done — Phase 8** |
| `lintian` | not installed here — **BLOCKED BY EXTERNAL DEPENDENCY** |

## What is sound

Stated first because an audit that lists only faults misrepresents the thing.
Each was checked, not assumed.

- **No shell anywhere.** Every execution is an argv array; there is no
  `sh -c` in shipped code, so a semicolon is an argument. VERIFIED by grep over
  all non-test sources.
- **Approval gate.** Mutating tools never execute in the loop; they are stored,
  the turn stops, and the approval runs the *stored* arguments. VERIFIED by tests.
- **Kernel confinement.** Project work runs under Landlock, confined to the
  project, temp and tool caches. VERIFIED on this machine last week.
- **Credential paths** are refused on read and write, in code rather than prompt.
- **SSRF guard:** every *resolved* address is checked, not the hostname, and
  redirects are re-checked.
- **Device access:** loopback or a token given in person and stored hashed;
  cookie is `HttpOnly`, `Secure`, `SameSite=Strict`; TLS ≥ 1.2; everything under
  `/api/` is guarded except `/health` and the pairing claim.
- **API keys** are never returned to the page — only the last four characters —
  and are not logged anywhere.
- **Self-contained binary.** Interface, icons, three.js and the capability
  packages are embedded; no runtime dependency on the source tree. VERIFIED by
  running the packaged binary outside the repository with a fresh `HOME`.
- **XDG paths** already: `~/.local/share/pn-scripts-assistant`,
  `~/.config/pn-scripts-assistant`. No developer path in shipped code.
- **Shutdown** handles `SIGINT`/`SIGTERM` with a graceful HTTP shutdown; tasks
  that were running are parked on the way back up, never silently resumed.

## Findings

Severity is about this release. Status uses the vocabulary you set.

### CRITICAL

None found.

### HIGH

**H1 — `serve` does not take the single-instance lock**
- Location: `clients/cmd/pn-scripts-assistant/main.go` — `server.Claim(root.Path)` is called at line 657 in the windowed path only; `runServe` never claims it.
- Status: IMPLEMENTED — BROKEN (partial)
- Current behaviour: two `pn-scripts-assistant serve` processes open the same data root. Verified: the second process opened the database, started its model keep-alive and only then failed on the port. With different ports, both would run.
- Expected: one brain per data root, whichever way it is started; the second should say so and stop.
- Root cause: the lock was added for the app path when the window was added.
- Fix: claim the lock in `runServe` too, releasing it on shutdown.
- Verification: start two `serve` processes on different ports against one data root; the second must refuse with a clear sentence.

**H2 — the package has no `copyright` file**
- Location: `scripts/build-packages.sh`, `build_deb`.
- Status: MISSING
- Current: `/usr/share/doc/pn-scripts-assistant/` does not exist in the package.
- Expected: Debian policy §12.5 requires a copyright file; the project is MIT-licensed and must say so where the system looks.
- Fix: install `copyright` and a `changelog.Debian.gz`.
- Verification: `dpkg-deb --contents`; `lintian` once installed.

**H3 — diagnostics disappear when launched from the desktop**
- Location: `cmd/pn-scripts-assistant/main.go` — every logger writes to `os.Stderr`.
- Status: IMPLEMENTED — INSUFFICIENT
- Current: started from the applications menu, stderr goes nowhere a person can find. "It did not start" is then unanswerable.
- Expected: a log file under `~/.local/state/pn-scripts-assistant/` (or the journal), rotated, with the same lines.
- Fix: tee to a log file; say where it is in `status` and in the troubleshooting docs.
- Verification: launch from the menu, then read the file.

**H4 — the documentation describes a release that does not exist**
- Location: `README.md` lines 98–120.
- Status: BROKEN (documentation)
- Current: tells the reader to "download the AppImage"; there are **no git tags and no published artifacts**, and the `.deb` — the mandated path — is not mentioned at all.
- Fix: rewrite around the `.deb`; describe AppImage only if it is actually built and published.
- Verification: follow the README on a clean machine.

### MEDIUM

**M1 — common failures report raw Go errors**
- Evidence, both reproduced: port clash → `listen tcp 127.0.0.1:8899: bind: address already in use`; no model service → `reaching ollama at http://127.0.0.1:1: Post "…": dial tcp …: connect: connection refused`.
- Expected (your §10): what failed, what was expected, what to do.
- Fix: a small number of actionable messages at the few places a new user meets first — port in use, Ollama missing, data root unwritable, model not installed.

**M2 — no retries on model calls**
- Location: `internal/brain/llm/*.go`; no retry or backoff anywhere.
- Current: one transient network failure ends a chat turn. Tasks survive it (the orchestrator fails over); conversations do not.
- Fix: one bounded retry for idempotent calls; never for anything already streaming.

**M3 — the window does not set its WM class**
- Location: `internal/brain/window/window_linux.go` sets the title only; the desktop entry declares `StartupWMClass=pn-scripts-assistant`.
- Current: Ubuntu may not match the window to the entry — generic icon, or a second dock item. NOT VERIFIED (needs the package installed; Phase 8).
- Fix: set `g_set_prgname`/WM class to `pn-scripts-assistant`.

**M4 — secrets are scrubbed in evidence but not in transcripts or logs**
- Location: `redact.Text` is applied in `store/evidence.go` only; `store.AddMessage` and every `slog` line are unscrubbed.
- Current: an approved command whose output contains a token stores that token in the conversation, which a hosted model may later be given (privacy mode permitting).
- Fix: scrub tool output on the way into messages, and at the log handler.

**M5 — package dependencies are hand-written**
- Location: `build_deb` control block.
- Current: `Depends:` lists webkit and gtk by hand; the binary links ~60 libraries transitively.
- Fix: compute with `dpkg-shlibdeps`, or verify the hand-written set against `ldd` in the release script.

**M6 — no single version source; CI never builds the package**
- Current: no git tags, so versions are `0.1.0~git<date>.<hash>`; the desktop entry carries no version; `.github/workflows/release.yml` builds loose binaries and never the `.deb`.
- Fix: one version source (a tag, or `VERSION` read by the build and reported by `--version`), and a CI job that builds and validates the package.

**M7 — microphone and voice are on by default**
- Location: `config.Default()` — `AlwaysListen: true`, `AlwaysSpeak: true`.
- Current: a fresh install opens the microphone at start, before anybody has agreed to it in the interface.
- Fix: decide deliberately. Recommended: off until setup asks, given this is a first-run experience on someone else's machine.

**M8 — local models run at Ollama's default 4,096-token context**
- Known and documented in ADR 0005; long steps can lose their instruction.
- **Corrected in Phase 4 — this was not MEDIUM.** Measured afterwards: a
  conversation turn is 6,338 tokens, of which the model was given 2,050. The
  whole persona was discarded on every turn, and the visible symptom was a
  model that called tools in a loop and never answered. Severity was
  **CRITICAL**; fixed and verified in `docs/audit/phase-4-core.md`. The
  finding was graded from the steps ADR 0005 had measured (1.8–2.4 k tokens)
  without measuring a conversation turn — the reason this audit's own rule is
  to measure rather than infer.

**M9 — `menu` can duplicate the packaged desktop entry**
- Location: `desktop.Install` writes `~/.local/share/applications/…`; the package installs `/usr/share/applications/…`.
- Fix: `menu` should say it is already installed system-wide and do nothing.

### LOW

- **L1** — `go.mod` marks three real dependencies `// indirect` (sqlite, onnxruntime_go, x/sys). `go mod tidy` fixes it; no change to what is built.
- **L2** — the desktop file is installed 0664; should be 0644.
- **L3** — `Maintainer: noreply@localhost`, no `Homepage`.
- **L4** — tiny helpers duplicated across packages (`orElse` in 10). Idiomatic in Go; no action recommended.
- **L5** — two `panic()` calls, both on embedded assets failing to load — a build-time invariant. Acceptable; worth a sentence in the code.
- **L6** — no static analysis beyond `go vet`; no `staticcheck`, no `lintian` in CI. Dead code has therefore **not been assessed** — stated plainly rather than guessed at.
- **L7** — `docs/ROADMAP.md` is stale (counts, pre-rename wording).

## Integration status

Nothing here is claimed on the strength of a name appearing in the code.

| Integration | Status | Evidence |
|---|---|---|
| Ollama / local models | VERIFIED | used for every turn on this machine |
| Anthropic (API) | IMPLEMENTED — NOT VERIFIED | no key configured here |
| OpenAI, OpenRouter, Google, Groq, DeepSeek, Mistral, xAI, Together, Fireworks, Cerebras, Perplexity | IMPLEMENTED — NOT VERIFIED | one OpenAI-compatible client; none configured |
| Claude Code | IMPLEMENTED — VERIFIED FAILING | detected; signed out; its auth failure is read correctly and fails over |
| Codex | IMPLEMENTED — VERIFIED FAILING | detected; out of allowance until 15 Oct |
| Cursor agent | PARTIALLY IMPLEMENTED | detected, never driven, by decision |
| VS Code / Cursor / IntelliJ | IMPLEMENTED (detection only) | listed as editors, never given work |
| Godot | VERIFIED | real project, check, run, picture, build |
| three.js | VERIFIED | real check, run, picture |
| Unity | IMPLEMENTED — BLOCKED | installed, not licensed for batch use |
| Unreal, Phaser, Babylon, PlayCanvas, Bevy, Defold, GameMaker | IMPLEMENTED — NOT VERIFIED | adapters exist, not exercised |
| MCP servers | IMPLEMENTED — NOT VERIFIED | six known, none approved here |
| Node, npm, Go, cargo, uv, Chrome, Godot templates | IMPLEMENTED (install recipes) | `provision` |
| **Git** | **NOT IMPLEMENTED** | no git tool; git is used only by build scripts |
| **Docker** | **NOT IMPLEMENTED** | deliberate |
| **PHP, Python** | **NOT IMPLEMENTED** | no integration |

Tool detection reports path, version, version-compatibility, licence state and
installability, with a sentence for each problem. It had no distinct
**permission-error** state (your list asks for one): an unreadable executable
was reported as absent. **Fixed in Phase 4** — `Status.Blocked`, with the path
and the remedy.

## Testing gaps

- No installation test, no packaging test, no end-to-end test of the installed
  application. This is the largest gap and the reason for Phases 7–8.
- No test asserts that the desktop entry and icons land where Ubuntu expects.
- Hosted AI providers have unit tests against fakes; none has been exercised
  against a real endpoint on this machine.

## Exit criteria

The complete technical audit is done: every area in the brief was examined,
every claim above is backed by something that was run, and what could not be
run is named.

**PHASE 1 COMPLETE**
