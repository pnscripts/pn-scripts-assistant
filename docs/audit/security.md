# Security

**Date:** 2026-09-23 · **Machine:** Ubuntu 24.04.5 · Written for Phase 5 of the
production audit. Everything claimed here was either run on this machine or is
covered by a test named below. Where something is not covered, it says so.

## What this program is, in security terms

A single-user desktop application that keeps a private database, runs local and
hosted AI models, and — with its owner's approval — executes commands and edits
files on the machine it runs on. It listens on loopback. It can be opened to
other devices deliberately, one device at a time, by pairing them in person.

The thing worth protecting, in order: **what it knows about its owner** (the
database), **what it can do on their behalf** (tools), and **the credentials it
holds** (model keys, mail password, integration tokens).

## Trust model

| Who | What they are trusted with | How that is enforced |
|---|---|---|
| The person at the keyboard | Everything, through the interface | The window and `http://127.0.0.1:<port>` |
| A program on this machine (a script, the CLI) | Everything the API offers | It sends no `Origin`; nothing else distinguishes it from the owner, and it is already running as them |
| **A web page the owner has open** | **Nothing** | `Origin` must be this server; the `Host` must be a loopback name. Both checked before the request is routed |
| A paired device, full | Everything except nothing — it is a second keyboard | Token given in person, stored hashed; cookie `HttpOnly`/`Secure`/`SameSite=Strict`; TLS ≥ 1.2 |
| A paired device, limited | Ask and watch; no decisions | `atTheDeskOnly` route prefixes refuse approvals, settings, pairing and files |
| Anything else on the network | `/health` and the pairing claim, nothing more | The listener binds loopback unless reach is on; the guard refuses unpaired callers |
| A model, local or hosted | Nothing by itself | Every mutating tool stops for approval and runs the *stored* arguments, not the model's next message |

The model is **not** trusted. Nothing it writes becomes a command, a URL to
fetch, or an install: the tools it may call are a fixed list in this program,
each with its own rules, and the dangerous ones stop and ask.

## Fixed in this phase

### S1 — a web page could drive the assistant (CRITICAL)

- Status before: **IMPLEMENTED — BROKEN**. The guard trusted any caller on the
  loopback interface and returned before its same-origin check, which applied
  only to paired remote devices.
- Demonstrated against the running program, not reasoned about: a
  `POST /api/desktop` carrying `Origin: http://evil.test` and
  `Content-Type: text/plain;charset=UTF-8` — a request any page may send across
  origins with no preflight — was accepted and **took the program out of the
  applications menu**. The state changed.
- Reach: any site the owner has open, for as long as the assistant is running.
  Settings, tasks, conversations, approvals.
- Fixed: a request that arrives from this machine must now also (a) name this
  machine in its `Host` header and (b) carry either no `Origin` or this
  server's own. Verified afterwards, same requests: `403 that request came from
  somewhere else`.

### S2 — a name pointed at this computer (CRITICAL, same fix)

Anyone may make their own domain resolve to `127.0.0.1`. The browser then
treats the assistant as that site's own origin: no `Origin` header to catch,
and **every answer readable by the page** — the whole memory, not just the
ability to act. The name in the request is the part a page cannot choose, so
the `Host` header is now checked against loopback names for every local
request, reads included. Verified: `Host: evil.test:8899` → `403 this assistant
answers to localhost, not to evil.test`.

### S3 — the database was readable by every account on the machine (HIGH)

`brain.sqlite` was created 0644 by SQLite's default. It holds every
conversation, everything remembered, and the exact output of every command
approved. The settings file beside it has been 0600 since the first week and is
the less revealing of the two. Now 0600 on open — including the `-wal` file,
where the newest writes live in WAL mode and nowhere else — and the folder
0700. An existing database is tightened the next time it opens; verified on one
that was already 0644.

### S4 — secrets reached the transcript and the log (MEDIUM)

`redact.Text` was applied to evidence only. Two places kept text it never saw:

- **The conversation.** A tool's output — `env`, a config file read out, a
  `curl` that echoes its header — was stored exactly, read back into every
  later turn, and could be sent to a hosted model. Now scrubbed in
  `store.AddMessage`, which is the one place every message passes through.
- **The log.** The file a person attaches to a support message without reading
  it. Now behind a `slog.Handler` that scrubs the message, the attributes, and
  attributes carried on a child logger. The standard library's logger is routed
  through the same handler, because one line in the speech server used it and
  went to the terminal only, unscrubbed.

## What was verified on this machine today

| Check | Result |
|---|---|
| What it listens on | `127.0.0.1:8899` only (`ss -ltnp`) |
| `POST` from another site's origin | **403** |
| `GET` with a foreign `Host` (rebinding) | **403** |
| The assistant's own page (`Origin: http://127.0.0.1:8899`) | 200 |
| A program with no `Origin` | 200 |
| `brain.sqlite`, `-wal`, `-shm` after start | 0600 |
| `brain.conf` | 0600 |
| The log file and its folder | 0600 / 0700 |
| Secrets in the log | none — scrubbed, tested against real token shapes |
| Secrets in the conversation | none — tested with an `env` dump holding a key |

## What holds from the Phase 1 audit

Each was checked then and is unchanged by this phase: no shell anywhere (argv
arrays only); mutating tools stored and re-run from storage after approval;
Landlock confinement for project work; credential paths refused in code;
SSRF checks on the *resolved* address with redirects re-checked; API keys never
returned to the page beyond their last four characters.

## Not covered, and worth saying plainly

- **No penetration test.** Everything here was tested by someone who knew where
  to look.
- **A program running as this user is this user.** Nothing here defends against
  malware already running in the owner's account: it can read the database
  directly, whatever the server does.
- **`/health` and the pairing claim are open by design.** `/health` says only
  that something is listening; the claim needs a token shown in person.
- **The tunnel and paired devices** were tested on this machine, never across
  the internet.
- **The `.deb` has not been installed yet** (Phase 8), so nothing here says
  anything about file ownership after a packaged install.
- **Dependencies are not scanned.** Three direct dependencies, no
  `govulncheck` in CI yet.

**PHASE 5 COMPLETE**
