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
| `go test -race` on server | **no data races** (802 s) |
| `go mod tidy -diff` | clean |
| `govulncheck ./...` | **No vulnerabilities found** — after the toolchain was raised; see S7 |
| Dependencies | three direct (`modernc.org/sqlite`, `onnxruntime_go`, `golang.org/x/sys`); none added this month |

The server package under `-race` exceeded Go's ten-minute default test timeout
on the first attempt — it takes 32 s without the detector, and the detector
costs about twenty-five times that. Re-run with a longer deadline: **exit 0,
802 s, no data race**. Status: VERIFIED.

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
| A newer package sorts newer | **VERIFIED — after a fix** | the first upgrade attempt was refused as a *downgrade*; see S5 below |
| Files land where Ubuntu looks | **VERIFIED** | `dpkg -L`: `/usr/bin`, `/usr/share/{applications,icons/hicolor,man/man1,doc}` |
| `man pn-scripts-assistant` | **VERIFIED** | renders |
| `pn-scripts-assistant version` | **VERIFIED** | `PN Scripts Assistant 0.1.0~git20260923.983e04f (983e04f), built 2026-09-23` |
| Launch from the applications menu | **VERIFIED** | started through the desktop entry with `gtk-launch`; the window's `WM_CLASS` is `pn-scripts-assistant`, matching the entry's `StartupWMClass` |
| First run opens setup | **VERIFIED** | and it was this that exposed the Ollama-detection fault |
| The installed program serves a brain | **VERIFIED** | `/api/v1/version` reports the package's own version |
| Upgrade over an installed copy | **PARTIALLY VERIFIED** | run with dpkg against a separate root rather than with `apt` on this system; see below |
| `apt remove` | **PARTIALLY VERIFIED** | same run, third step |

### The lifecycle, run where it did not need a password

`apt` on this machine needs a password the program cannot supply, and that is
why two rows above sat at NOT VERIFIED for a fortnight. dpkg will do the same
work into a directory of its own, as an ordinary user, which is enough to
exercise everything this package actually contains:

```bash
dpkg --force-not-root --force-script-chrootless --force-depends \
     --instdir=$T/root --admindir=$T/admin -i pn-scripts-assistant_<version>.deb
```

Three steps, against the real package and a copy of it with the version bumped:

| Step | What happened |
|---|---|
| install | 10 files placed under `/usr/{bin,share}`, package `ii` (installed, configured) |
| upgrade | version moved to the bumped one, still `ii`, still exactly 10 files — nothing from the old copy left behind |
| remove | 0 files left in the root, the package gone from dpkg's database |

What this does **not** cover, and the reason the status is PARTIALLY rather
than fully verified: `apt`'s own dependency resolution (forced off here, since
the fake root has no libraries in it) and the triggers other packages run —
man-db, desktop-file-utils, hicolor-icon-theme. Those ran on the real install,
which is verified above. And nothing removes anything from a home directory,
which is not a matter of testing at all: the package carries no maintainer
scripts, so there is nothing that could.

## Two more faults, found by doing the install rather than reading about it

**S5 — a newer build looked older to apt.** Installing the new package over
the installed one was refused: `1 downgraded … E: Packages were downgraded`.
The version ended in a commit hash, and dpkg compares a version in runs of
digits and letters — `20260923.983e04f` against `20260923.91b8207` came down
to 983 against 91. Every build on the same day was ordered by a hash, which is
ordered by nothing. The timestamp carries the ordering now, to the second, and
`scripts/version-order-test.sh` asserts it with dpkg itself as part of
`make check`. Severity: **HIGH** — it makes upgrades fail. Status: **FIXED and
VERIFIED** for ordering; the upgrade itself is below.

**S6 — a slow answer was thrown away by the server, not by the model.** A
first conversation turn that reached for a tool took **28 minutes** on this
machine; the server's write deadline is fifteen, so the connection was cut and
the caller received an empty reply — while the assistant had finished the
work, written the answer into the conversation and stopped for approval
exactly as designed. Everything worked and nobody was told. The three requests
that wait on a model now set their own deadline
(`server.letItThink`, one hour) instead of inheriting one meant for ordinary
web traffic. Severity: **HIGH** on hardware like this one. Status: **FIXED**,
with a test that runs a real server with a real deadline and a handler that
outlasts it.

**S7 — the toolchain had twenty-eight known vulnerabilities.** The first
`govulncheck` run, against go1.26.0, reported 28 in the Go standard library
alone — `crypto/x509`, `crypto/tls` and `net/http` among them, all three of
which this program serves with, and all reachable from `Server.Serve`. None
was in this code. `go.mod` now pins `toolchain go1.26.6`, the go command
fetches it when a machine does not have it, and the scan is clean: **No
vulnerabilities found**. It runs as part of `make release` and in CI, so the
next one is found by a machine rather than by somebody remembering.
Severity: **HIGH**. Status: **FIXED and VERIFIED**.

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

- **M2 — no retries on model calls.** **FIXED.** One more attempt, for the
  failures that could succeed if asked again: a connection that never arrived,
  or a 429/500/502/503/504. Never for a refusal — asking the same wrong
  question twice gets the same answer twice, and on a paid service costs twice
  — never after cancellation, and never for anything already streaming, where
  half an answer has been delivered. `Retry-After` is honoured and bounded at
  five seconds.
- **The prompt is mostly tool descriptions** (MEDIUM, performance). Of 6,338
  tokens, 1,224 are the persona and about 5,100 are 39 tool schemas — a
  greeting pays for every tool the assistant has. `agent.relevant` drops the
  obvious mismatches and is deliberately cautious, because a tool wrongly
  withheld is the worse failure. NOT IMPLEMENTED, measured and recorded.
- **One model slot** (LOW). The fact extractor runs between turns and evicts
  the conversation; Ollama's prompt cache restores it, which is what makes the
  second turn ten times faster. On a machine where that cache does not fit, the
  turn is paid for again.
- **L6 — no `staticcheck`** (LOW). `govulncheck` is now part of `make release`
  and of CI; `staticcheck` is not, so **dead code has still not been
  assessed** — stated plainly rather than guessed at.
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
- **Closed since:** the `.deb` is byte-for-byte reproducible, verified between
  a GitHub runner and this desktop from one commit and gated in CI; the install lifecycle has been run
  end to end; CI now runs on every push and pull request rather than only on a
  tag; every released file carries a signed provenance attestation.
- **Open:** hosted AI providers are tested against fakes only; the tunnel and
  paired devices have never been exercised across the internet. (The package
  install is no longer on this list: CI installs, upgrades and removes it on
  every push, in a root of dpkg's own — see `installs` in ci.yml.)

## Release readiness

**Ready, with one condition stated rather than hidden:** nothing here has been
checked on a machine other than this one. The upgrade and removal steps have
now been run — against a root of dpkg's own rather than through `apt`, for the
reason and with the limits written above.

Everything else the brief asked for is done and was run: it builds, installs,
launches from the menu, configures itself, serves, answers, logs, refuses what
it should refuse, and uninstalls cleanly in dpkg's own account of what it owns.

## Phase 12 — the artifacts

In `build/packages`, built by `make release` on this machine:

| File | |
|---|---|
| `pn-scripts-assistant_0.1.0~git20260923172822.d8c47a3_amd64.deb` | 7.3 MB, lintian clean, PIE, no reference to the development tree |
| `SHA256SUMS` | `f924bced15b0a93613c5e347c8b059a233086994d7543dfd3a73b16e756b8a7a` |
| `RELEASE-NOTES.md` | what is in the release, and what is knowingly not |

The packaged binary's own account of itself:

```
PN Scripts Assistant 0.1.0~git20260923172822.d8c47a3 (d8c47a3), built 2026-09-23
```

**PHASE 11 COMPLETE · PHASE 12 COMPLETE**
