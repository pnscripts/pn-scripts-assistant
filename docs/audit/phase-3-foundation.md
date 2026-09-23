# Phase 3 — foundation and blocker fixes

**Date:** 2026-09-23 · **Machine:** Ubuntu 24.04.5 · Everything below was run.

## Problems fixed

### H1 — two copies could open one brain

`server.Claim` was taken by the windowed app and not by `serve`, so two
`pn-scripts-assistant serve` processes on different ports shared a data root:
two sets of tasks, two learning workers, one database.

`runServe` now claims the same lock and releases it on the way out. A copy that
holds a brain also **writes where it is answering** into the lock file
(`Lock.Answering`), so the second copy can say something true rather than guess
from the address it was given itself.

Verified on this machine:

```
$ pn-scripts-assistant serve            # first copy, port 8896
$ pn-scripts-assistant serve            # second, port 8895, same brain
  Assistant is already running on this brain (…/data) and answering on http://127.0.0.1:8896.
  Stop it first, or serve another brain with PN_SCRIPTS_ASSISTANT_DATA_ROOT=<folder>
  (exit 1)
```

### H3 — nothing to read when it will not start

Every logger wrote to standard error only, which goes nowhere when the program
is started from the applications menu — so "it did not start" could not be
answered.

New package `internal/brain/logs`: the same lines go to the terminal **and** to
`$XDG_STATE_HOME/pn-scripts-assistant/logs/pn-scripts-assistant.log`
(`~/.local/state/…` by default), 0600, rotated by hand at 5 MB keeping three.
Not the data root, because that can be a removable disk that is not plugged in,
and a log that disappears with the disk cannot explain why the disk is missing.
A log that cannot be opened costs the file and not the run: the program says so
and carries on with the terminal only.

Verified: the file exists after a run, holds the startup lines, and is 0600.

### M1 — the two failures a new user meets first

| Before | Now |
|---|---|
| `listen tcp 127.0.0.1:8896: bind: address already in use` | `something is already answering on 127.0.0.1:8896 — if it is Assistant, open http://127.0.0.1:8896; otherwise start this one elsewhere with BRAIN_ADDR=127.0.0.1:<port>` |
| `reaching ollama at http://127.0.0.1:1: Post "…": dial tcp …: connection refused` | `Ollama is not answering at http://127.0.0.1:1 — start it (ollama serve), or point this at another one with OLLAMA_BASE_URL: …` |

Both reproduced before and after.

### M7 — the microphone is off until it is asked for

`config.Default()` had `AlwaysListen: true`, so a fresh install opened the
microphone at start, before anybody had agreed to it in the interface. Now
false, by the owner's decision. Speaking aloud (`AlwaysSpeak`) is unchanged and
still on.

Verified on a new brain: `always_listen: false`.

### L1 — module metadata

`go mod tidy`: three real dependencies (`modernc.org/sqlite`,
`onnxruntime_go`, `golang.org/x/sys`) were marked `// indirect`. Nothing about
the build changes; the file now says what is true.

## Problems remaining

Carried forward deliberately, in their own phases: M2 retries, M3 the window's
WM class, M4 scrubbing transcripts and logs, M5 computed package dependencies,
M6 version source and CI, M9 duplicate desktop entry, H2 copyright, H4 the
README, and everything in Phases 6–12.

## Tests executed

| Test | Result |
|---|---|
| `gofmt -l` | 0 |
| `go vet ./...` | clean |
| `go test ./... -p 2` | **exit 0 · 61 packages ok** (one new: `logs`) |
| New: `logs` — writes and says where, rotates keeping three, survives nowhere to write | pass |
| New: `server` — the holder records its address; a second copy is still refused | pass |
| New: `llm` — the Ollama message names the address, `ollama serve` and `OLLAMA_BASE_URL` | pass |
| Real machine: two `serve` copies on one brain | refused, exit 1, correct address |
| Real machine: port taken by something else | actionable message |
| Real machine: log file after a run | present, 0600, correct lines |
| Real machine: microphone default on a new brain | off |

## Known limitations

- The log file records what the program logs; it does not yet scrub secrets
  (M4, Phase 5).
- `bindProblem` is verified on this machine rather than by a unit test —
  simulating `EADDRINUSE` in a test would test the simulation.
- Nothing here has been packaged or installed yet; that is Phases 7 and 8.

## Exit criteria

The application builds and starts reliably, refuses to run twice on one brain,
says something useful when it cannot start, and leaves a record behind.

**PHASE 3 COMPLETE**
