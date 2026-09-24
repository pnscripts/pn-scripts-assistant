# Phase 2 — the capability registry

**Date:** 2026-09-24 · **Machine:** Ubuntu 24.04.5 · Additive only: nothing was
removed and nothing is wired into the assistant yet. That is Phases 3–5.

## What was built

`internal/brain/environs` — one list of what is on this machine, one type, one
state vocabulary, one cache.

### The states, which are the point

Four answers where the program had two. "Not configured" is not "not
permitted" and neither is "not installed": each wants a different sentence
from the assistant and a different action from its owner.

| State | Means |
|---|---|
| `available` | here and usable now |
| `needs_configuration` | here, wants a key, a mailbox, an address |
| `needs_sign_in` | here and configured, nobody signed in |
| `needs_licence` | here, not licensed for this use |
| `not_installed` | not on the machine, and could be |
| `blocked` | here and unusable — no executable bit, a folder nobody may read |
| `no_capacity` | here, allowance spent |
| `unknown` | could not be established, said rather than guessed |

### What it reads

42 programs by name and, where one exists, by the finder that already knows
better — Godot is a downloaded file named for its own version, and
`godot.FindAll` has known where those live for months. The rest of what the
assistant can reach (models, integrations, services, its own abilities) is
supplied through `Sources.More` by whoever builds the world, because a package
that discovers a machine must not depend on the program running on it.

Measured on this machine: **30 of 42 found**, a full reading in about a second
and a half, kept for ten minutes, re-readable on demand for the moment after
something is installed.

### The block the model will be given

Generated from that list, ordered so it never reshuffles — a prefix that
changes cannot be reused, and on this hardware that costs minutes:

```
On this machine, now:
Thinks with: Ollama 0.34.0
Writes code with: Claude Code 2.1.215, Codex 0.155, Cursor's agent
Languages: .NET 9.0.121, Go 1.26.6, Java 21.0.6, Node.js 22.23.2, PHP 8.4.25, Python 3.12.3
Game engines: Godot 4.7.1, Unity
Editors: Cursor, IntelliJ IDEA, Visual Studio Code
Tools: Blender, CMake, Composer, espeak-ng, FFmpeg, GCC, Git, GitHub CLI, ImageMagick, jq, make, npm, rsync, xdotool
Browsers: Chrome or Chromium, Firefox
Not installed: GitHub Copilot CLI, Godot, Ruby, Rust, Unreal Engine — you can say so, and offer to install what this program installs.
```

**576 characters — about 145 tokens**, against a turn that is already 6,338.
The budget is enforced in code and tested: over it, the least useful line goes
first.

Something here and not ready is named with its reason —
`Claude Code 2.1.215 (not signed in)`, `Unity (not licensed)`,
`Anthropic (needs setting up)` — because a model that is not told a thing
exists cannot suggest signing in to it, and that silence is what made the
assistant unable to explain itself.

### What a tool cannot work without

`tools.Needing`: `godot_build` needs Godot, `type_text` needs xdotool,
`make_subtitles` needs ffmpeg and whisper, `read_a_page` needs a browser.
Written as data so the selection code, the help answer and the explanation can
all read it, instead of each tool discovering it by failing — which on this
machine costs a minute of a model's turn to learn something that was on a
list.

**Deliberately not a gate.** A tool whose requirement is missing stays
registered and callable: the machine changes between the reading and the call,
and a tool that refuses on the strength of a stale list is worse than one that
tries and says what happened.

## Tests

| Test | What it proves |
|---|---|
| `TestItFindsWhatIsInstalledAndAsksItsVersion` | a real file on a real PATH, version pulled out of its own output |
| `TestSomethingAbsentSaysWhatItWouldTake` | absence carries the remedy |
| `TestAProgramThatCannotRunIsNotCalledMissing` | 0600 file → `blocked`, with `chmod` in the sentence |
| `TestThingsThisPackageCannotSeeAreAddedBySources` | injected sources land in the same shape |
| `TestTwoAnswersAboutOneThingSettleOnTheBetterOne` | the better-informed source wins |
| `TestSomethingTurnedOffIsNotInTheList` | an override removes one thing in one place |
| `TestTheReadingIsKeptAndCanBeTakenAgain` | three questions, one reading; `Again` re-reads |
| `TestAnOldReadingIsRefreshed` | a stale reading is not used |
| `TestUsableAnswersAboutOneThing` | an unknown id is not usable |
| `TestItReadsThisMachine` | a real scan of this machine, 30 found, Go present with a version |
| `TestTheBlockSaysWhatIsHereAndStaysSmall` | the budget holds against the real machine |
| `TestABlockOverBudgetLosesItsLeastUsefulLine` | what is cut is the least useful |
| `TestSomethingHereButNotReadyIsStillSaid` | states reach the words |
| `TestTheBlockDoesNotReshuffleItself` | two readings, one block |
| `TestToolsSayWhatTheyCannotWorkWithout` | requirements are readable per tool and in bulk |
| `TestSomethingAddedLaterSaysWhatItNeeds` | a skill answers for itself |

**63 packages pass**, `gofmt` and `go vet` clean.

## Not done in this phase, on purpose

- Nothing consumes `environs` yet: the prompt, the help answer, the selection
  code and the interface are untouched. Phases 3–6.
- `orchestrator`, `provision` and `preflight` still hold their own discovery.
  They stay until the new one is proved in place — two systems briefly, by
  plan, and one of them dies in Phase 10 rather than both living on.
- The assistant's own abilities are not yet in the list; they arrive when the
  brain supplies them, in Phase 4.

**PHASE 2 COMPLETE**
