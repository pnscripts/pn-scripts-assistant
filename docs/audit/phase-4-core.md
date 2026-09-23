# Phase 4 — core functionality stabilization

**Date:** 2026-09-23 · **Machine:** Ubuntu 24.04.5, 4 cores, 31 GB, no graphics
acceleration · **Model:** qwen2.5-coder:7b through Ollama.
Everything below was run. Where a number appears, it was measured here.

## The finding this phase turned on

**C1 — the local model was given a third of its prompt, and nothing said so.**

- Status: **IMPLEMENTED — BROKEN** · Severity: **CRITICAL** (now fixed)
- Where: `internal/brain/llm/ollama.go` — no `num_ctx` was ever sent, so Ollama
  used its default window of 4,096 tokens.
- Measured, by recording the exact request through a relay in front of Ollama
  and reading the model server's own log:

  | | Tokens |
  |---|---|
  | The turn as the brain sends it ("say hello", 39 tools offered) | **6,338** |
  | What the model was given at Ollama's default window | **2,050** |
  | Discarded, silently, before the model read a word | **4,288 (68%)** |

  The 5,754 characters of persona — who it is, when to use a tool, when to
  answer in words — are at the *start* of the prompt, which is the part
  dropped. The model was left with the tail of a tool list and a question.

- What it looked like from outside: asked **"hello, who are you?"** the brain
  called `what_can_you_do` three times and `what_am_i_hearing` five times and
  then answered *"I stopped after 8 steps without reaching an answer."* —
  25 minutes of work and no reply. It read like a weak model and was a
  truncated prompt. Nothing failed: Ollama truncates silently and returns 200.

- Fix: every request now asks for a window sized to itself, quantised to
  4,096-token steps (`llm.room`), never smaller than Ollama's default, capped
  by how much memory the machine has (`llm.RoomFor` — 16,384 tokens at 16 GB
  or more, 8,192 at 8 GB, otherwise 4,096), and **grow-only within a run**
  (`Ollama.withRoom`), because Ollama reloads the model whenever the window
  changes and a small housekeeping call between two turns would otherwise
  reload it twice.

- Verified on this machine, same question, same model:

  | | Before | After |
  |---|---|---|
  | Prompt the model received | 2,050 of 6,338 tokens | **6,338 of 6,338** |
  | Answer to "say hello" | 8 tool calls, no reply | **"Hello! How can I assist you today?"** |
  | Tools called | 8 | **0** |

  `docs/decisions/0005-choose-how-work-is-done.md` recorded the 4,096-token
  window as a known and accepted limit, on the evidence that task steps fitted
  in 1.8–2.4 k tokens. That was true of steps and had never been measured on a
  conversation turn. The record has been corrected rather than left standing.

## Also fixed in this phase

### A program that is here and unusable is no longer reported as missing

The audit noted that tool detection had no permission-error state: a file
without its executable bit was reported as *"not on this machine"*, so somebody
would be told to install what they already had.

`provision.Recipe.Check` now answers three ways instead of two — here, absent,
or **here and unusable** (`Status.Blocked`), with the path and the remedy:
*"found at /opt/godot/godot, but it is not executable — chmod +x
/opt/godot/godot"*, or *"it may be in /opt/godot, which this user may not
read"*. Every place that acts on a status was taught the difference, so a
blocked program is named as a blocker rather than queued for a reinstall that
would not fix it and might not be allowed either.

### `menu` no longer writes over a packaged entry (M9)

Installed from the `.deb`, the program is already in the applications menu.
Writing a second entry into `~/.local/share/applications` would shadow the
packaged one, would not be updated with the package, and would keep pointing at
a path that an uninstall takes away. `desktop.SystemWide()` looks for an entry
under `XDG_DATA_DIRS` whose `Exec` names *this* program; when there is one,
`Install` writes nothing and says so, the System tab shows "installed with the
package" and offers no Remove button, and the command line refuses to remove it
and names `sudo apt remove pn-scripts-assistant` instead. A packaged entry for
a *different* copy of the program is correctly not treated as this one's.

### The window now says what it is (M3)

The desktop entry declares `StartupWMClass=pn-scripts-assistant`; the window set
no class at all, so GNOME matched it to whatever the binary happened to be
called. Measured with `xprop` on this machine:

| | `WM_CLASS` |
|---|---|
| Before | `"Before-The-Fix", "Before-The-Fix"` (the file's name) |
| After | `"pn-scripts-assistant", "Pn-scripts-assistant"` — whatever the file is called |

`g_set_prgname` is called before `gtk_init`, which is also the application id
GNOME matches on Wayland.

### `status` says where the log is

Phase 3 added the log file; `status` did not mention it, which is the one place
somebody looks when the program did not start.

## Real workflows run on this machine

A scratch brain — its own `HOME`, XDG directories and data root, never the live
one — configured for the local model, started with `serve`.

| Workflow | Result |
|---|---|
| Start on a fresh data root, no setup wizard | VERIFIED — serves, logs, creates only XDG paths |
| `status` against that brain | VERIFIED — data root, database, log, counts |
| `GET /api/v1/version` | VERIFIED — `{"product":"PN Scripts Assistant","protocol":1,…}` |
| `GET /api/desktop` | VERIFIED — reports `packaged: false` here, entry path correct |
| Greeting, spoken in the model's own words | VERIFIED — 8.0 s, grounded in the facts it was given |
| A typed conversation turn | VERIFIED — see below |
| Two `serve` copies on one brain | VERIFIED — the second refuses and names the first |
| Microphone default on a new brain | VERIFIED — off |

## What a turn costs on this machine, measured

Two turns in one conversation, timed end to end, with Ollama's own log read
beside them. Nothing else was changed between them.

| | Turn A ("say hello") | Turn B ("and what is two plus two?") |
|---|---|---|
| Prompt | 6,338 tokens | 6,343 tokens |
| Taken from the model's cache | 0 | **6,331** |
| Processed | 6,338 | **12** |
| Wall clock | **598 s** | **58 s** |
| Answer | "Hello! How can I assist you today?" | "2 + 2 is 4." |
| Tools called | 0 | 0 |

The first turn after the model loads pays for the whole prompt at about eight
tokens a second — this machine has four cores, no graphics card, and a browser
open. Every turn after it pays for the difference only, which is why the window
is grow-only: it was a housekeeping call at a smaller window, between two
turns, that made Ollama load the model again and threw the cache away.

The interface was checked on screen rather than through the API: the core, the
conversation with both questions and both answers, what it is doing now and how
long each part took. No blank panels.

## What is still true, and not fixed here

- **The prompt is mostly tool descriptions.** Of 6,338 tokens, the persona is
  1,224 and the 39 tool schemas are about 5,100. A greeting pays for every
  tool the assistant has. `agent.relevant` already drops the obvious
  mismatches and is deliberately cautious — a tool wrongly withheld is the
  worse failure — so this is recorded rather than tightened here.
  **Status: NOT IMPLEMENTED** (a size question, not a correctness one).
- **One slot, shared.** The fact extractor runs between turns on the same
  model and evicts the conversation from the model's slot; Ollama's prompt
  cache restores it, which is what made turn B fast. On a machine where that
  cache does not fit, the turn is paid for again.
- **Tasks were not re-run in this phase.** The orchestrator's end-to-end runs
  (Godot, three.js, Unity, failover, local-model install) were verified
  earlier this month and are unchanged by this work. The window fix applies to
  them too and can only enlarge what a step is given.
- **`bindProblem` and the window class** are verified on this machine rather
  than by unit tests, for the same reason as in Phase 3: simulating them would
  test the simulation.

## Tests executed

| Test | Result |
|---|---|
| `gofmt -l cmd internal` | 0 |
| `go vet ./...` | clean |
| `go test ./... -p 2` | **exit 0 · 61 packages ok** |
| New: `llm` — the window is sized to the turn, fits the machine, does not shrink back, stops at the cap | pass |
| New: `provision` — here-but-not-executable, an unreadable folder, and absence is still absence | pass |
| New: `desktop` — a packaged entry is left alone; another copy's entry is not mistaken for this one | pass |
| Real machine: the same question before and after the window fix | 8 tool calls and no answer → answered, no tools |
| Real machine: `xprop` on the window, before and after | file name → `pn-scripts-assistant` |
| Real machine: interface in a headless browser | renders, conversation correct |

## Exit criteria

Core implemented functionality has been tested on this machine and stabilized:
the local model now receives the whole of its prompt and answers instead of
looping, a program that is present but unusable is reported as such, the window
is matched to its launcher, the menu entry is not duplicated over a packaged
one, and `status` says where the log is.

**PHASE 4 COMPLETE**
