# Baseline — the project as found

**Date:** 2026-09-23 · **Commit:** `8d9c69f` on `dev` · **Machine:** Ubuntu 24.04.5, x86_64, Go 1.26.0

What was measured, not what the README says. Every line below was produced by
running something on this machine; where a thing was not run, it says so.

## Current architecture

One Go binary. No daemon beside it, no database server, no container. The
interface is a web page the program serves to itself and shows in a native
window (GTK + WebKitGTK through cgo). Everything it knows is one SQLite file
plus files in one data root. See ADR 0001.

```
one process
├── cmd/pn-scripts-assistant      app · serve · setup · status · menu · 20 more subcommands
├── internal/brain/server         HTTP API (159 routes) + the web interface (23k lines, embedded)
├── internal/brain/…              59 packages: tasks, orchestrator, agents, memory, llm, tools,
│                                 speech, engines, permits, store, pair, tunnel, sandbox…
├── internal/protocol, activity   typed events, numbered and kept (added this week)
├── internal/setup, preflight     first-run setup and what the machine is missing
└── internal/stage, window, away  three.js scene, the native window, the "brain is elsewhere" page
```

Two more commands exist: `cmd/doctor` (what this machine is missing, for
headless use) and `cmd/cli`.

| Measure | Value |
|---|---|
| Go, not tests | 112,364 lines |
| Go tests | 40,222 lines |
| Web interface | 23,055 lines (embedded in the binary) |
| Packages with tests | 60 |
| Packages without tests | 7 |
| HTTP routes | 159 |
| Tools the assistant can use | 75 |
| Module dependencies | 11, all indirect (SQLite, ONNX binding, uuid, humanize, isatty, strftime, bigfft, x/sys) |

## Current components

| Component | State |
|---|---|
| Web interface | Embedded (`go:embed`); served by the program itself |
| Native window | Linux only, cgo, needs GTK 3 + WebKitGTK 4.1 (both present here) |
| Database | SQLite (modernc, pure Go), 23 tables, 18 migrations |
| Data root | Found at run time by a marker file; never a remembered path |
| Devices | Pairing with hashed tokens, TLS for the network, WireGuard tunnel for outside |
| Realtime | None. The page polls, 32 timers. Events exist as of this week but are not yet streamed |

## Current build status — PASS

```
gofmt -l ./internal ./cmd        0 files
go vet ./...                     clean (exit 0)
go test ./... -count=1 -p 2      exit 0 — 60 packages ok, 7 with no tests
CGO_ENABLED=1 go build ./...     builds, with the window
scripts/build-packages.sh deb    builds a real .deb in 1m47s
```

Tests that need something this machine lacks skip with a reason (piper,
pdftotext, a TTY, a robot voice). Nothing in the suite requires the developer's
own directories.

## Current test status

60 packages pass. The suite covers the gates (approval, confinement, evidence),
the orchestrator, hiring, memory, engines against real Godot when it is
installed, and the packaging-adjacent parts of preflight. There is **no**
installation test, **no** packaging test, and **no** end-to-end test of the
installed application — that is the gap this work exists to close.

## Current installation status — NOT INSTALLED

- `dpkg -l` shows no `pn-scripts-assistant` package on this machine.
- Nothing on `PATH`; no desktop entry in `/usr/share/applications` or
  `~/.local/share/applications`.
- The program has only ever been run from the development tree, most recently
  through `scripts/pn-scripts-assistant-launch.sh`, which **rebuilds from the
  source tree** and runs `dist/pn-scripts-assistant` inside it. That script is
  a development convenience and must not be what an installed system uses.

Verified this session: the binary from the package stage runs **outside** the
repository with a fresh `HOME` — `help` and `status` work, and `serve` answers
on its port with the interface and the versioned API. It created
`~/.local/share/pn-scripts-assistant/data` and `~/.config/pn-scripts-assistant/last-root.json`
and nothing else.

## Current packaging status — a real .deb, with gaps

`scripts/build-packages.sh` builds a genuine Debian package (also a macOS
bundle and a Windows zip, neither buildable here). The .deb:

*Since this baseline was written:* the macOS side gained a `.dmg` — built with
`hdiutil` on a Mac and with `xorriso` anywhere else — and Windows gained an
Inno Setup installer, which can only be compiled on Windows and so is built by
CI. Both are in the release workflow. The sentence above describes what this
machine could produce on the day it was written.

```
pn-scripts-assistant_0.1.0~git20260921.8d9c69f_amd64.deb   7.0 MB
  /usr/bin/pn-scripts-assistant                            21.6 MB binary
  /usr/share/applications/pn-scripts-assistant.desktop
  /usr/share/icons/hicolor/{48,64,128,256,512}/apps/pn-scripts-assistant.png
Depends: libwebkit2gtk-4.1-0, libgtk-3-0t64 | libgtk-3-0
Recommends: xdotool, espeak-ng
```

Gaps found by inspection (to be judged in Phase 1):

1. No `/usr/share/doc/pn-scripts-assistant/copyright` — required by Debian policy.
2. No changelog.
3. The desktop file is installed group-writable (0664 rather than 0644).
4. `Maintainer: noreply@localhost` is a placeholder.
5. No `Homepage`, no `Installed-Size`.
6. No maintainer scripts; nothing verifies that the icon cache and desktop
   database are refreshed (Ubuntu's triggers usually do this — unverified).
7. Dependencies are hand-written rather than computed (`dpkg-shlibdeps`).
8. The package has never been installed or removed on this machine.
9. `lintian` is not installed here, so the package has not been checked against
   Debian policy.

## Current Ubuntu status

Works as a program on Ubuntu 24.04: builds, runs, serves, opens a window when
the libraries are present. Uses XDG-ish locations already
(`~/.local/share/pn-scripts-assistant`, `~/.config/pn-scripts-assistant`).
No systemd unit, and none obviously wanted: this is a desktop application that
the owner starts, not a service.

Not yet established: install → launch from the menu → configure → restart →
upgrade → remove. That is Phase 8.

## Current AI integrations

| Provider | State |
|---|---|
| Ollama (local) | Implemented; used for every turn on this machine; model roles chosen from what is installed |
| Anthropic | Implemented natively (key required; none set here) |
| OpenAI, OpenRouter, Google, Groq, DeepSeek, Mistral, xAI, Together, Fireworks, Cerebras, Perplexity, custom | Implemented through one OpenAI-compatible client; none configured here |
| Privacy router | Implemented: private / research / open decides what may leave the machine |
| Orchestrator | Implemented: chooses agent, model and engine per piece of work; records why |

Verified working on this machine: Ollama with `qwen2.5-coder:7b`; no hosted
provider is configured, so hosted paths are **IMPLEMENTED — NOT VERIFIED**.

## Current tool integrations

| Tool | State on this machine |
|---|---|
| Claude Code | Implemented as an executor; detected; **signed out**, so it fails at sign-in |
| Codex | Implemented as an executor; detected; **out of allowance until 15 Oct** |
| Cursor agent | Detected only, never driven (deliberate: checking sign-in starts a login) |
| VS Code, Cursor, IntelliJ | Detected as editors; never given work |
| Godot | Implemented and verified: new project, check, run, picture, build |
| Unity | Implemented; installed here but **not licensed for batch use** |
| Unreal | Adapter exists; not installed here — NOT VERIFIED |
| three.js, Phaser, Babylon, PlayCanvas, Bevy, Defold, GameMaker | Adapters exist; three.js verified, the rest NOT VERIFIED |
| Ollama | Verified |
| Node, npm, Go, cargo, uv, Chrome | Install recipes exist (`provision`) |
| MCP servers | Implemented (stdio, sandboxed, approval-gated); six known servers, none approved here |
| **Git** | **NOT IMPLEMENTED** as a tool (git is used only by the packaging scripts) |
| **Docker** | **NOT IMPLEMENTED**, deliberately |
| PHP, Python | **NOT IMPLEMENTED** as integrations |

## Known failures

None in the build or test suite. Known behavioural failures from last week's
end-to-end runs, unchanged: the local 7B model cannot reliably finish a game on
this processor; every such run ended *blocked* with its reason, never claimed as
done.

## Known warnings

- `libwebkit2gtk-4.1-dev`/`libgtk-3-dev` must be present to build the window; the
  packaging script refuses to build a windowless .deb, which is right.
- Ollama runs models with a 4,096-token context here; long steps can lose their
  instruction.
- Speech needs `espeak-ng` or piper for a voice; neither is installed here, so
  the program says it cannot speak.

## Known missing functionality

- No installation, upgrade or removal has ever been tested.
- No `make`-style entry point: the release process is a shell script nobody runs
  in CI. **CI never builds the .deb** — it builds loose binaries only.
- No version source: there are **no git tags**, so versions are derived as
  `0.1.0~git<date>.<hash>`.
- Logs go to standard error only. Launched from a desktop icon, they go nowhere.
- No streaming to clients (Phase 2 of the architecture plan).
- Windows and macOS windows do not exist; those packages serve to a browser.

## Known environment assumptions

| Assumption | Where | Risk |
|---|---|---|
| GTK 3 + WebKitGTK 4.1 present | window layer | Declared in the package's Depends |
| Ollama at `127.0.0.1:11434` | default config | Degrades with a clear message |
| Default port `127.0.0.1:8790` | default config | Untested against a port clash |
| `~/.local/share`, `~/.config` | paths package | Standard |
| External binaries when features are used: `xdotool`, `espeak-ng`, piper, whisper, `node`, `mutter`, Godot, Unity | tools, speech, engines | Each degrades with a message; Recommends covers two |
| No developer paths in shipped code | verified by grep: only one comment mentions `/media/petar`; tests use their own temporary directories | — |

## Exit criteria

The current state of the project is understood, measured, and written down.

**PHASE 0 COMPLETE**
