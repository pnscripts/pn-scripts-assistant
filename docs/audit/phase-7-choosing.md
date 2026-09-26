# Phase 7 — the tools a turn is given

**Date:** 2026-09-26 · **Machine:** Ubuntu 24.04.5, 4 cores, 31 GB, no
graphics acceleration · Model: qwen2.5-coder:7b through Ollama.

Everything the assistant has is available. This phase is about what fits in
one question — a different thing, and on this hardware the difference is
measured in minutes.

## What replaced what

`agent.relevant()` — a keyword table that dropped "housekeeping" tools unless
the message mentioned them — is gone. `agent.Choosing.choose()` decides the
list from four things:

1. **What every turn needs**, offered regardless: asking rather than guessing,
   saying what it can do, what it remembers, reading a file, listing a folder,
   searching. Nothing that changes anything — see below.
2. **What this conversation has already been shown**, kept whether or not this
   message mentions it.
3. **What this message plausibly needs**: a tool named outright, a cue
   matched, a skill's own cues, the web when a website is named.
4. **A budget** — 9,000 characters of names, descriptions and schemas — spent
   on the strongest claims first.

And two rules that were not there before:

- **A tool whose requirement is missing is not offered.** `godot_build` needs
  Godot; on a machine without it the tool is left out and the *inventory* says
  Godot is not installed and what it would take. Before, the only signal was a
  tool that failed when called — a minute of a model's turn to learn something
  that was already on a list.
- **The list only grows within a conversation.** Ollama keeps what it has
  already read only while the prompt begins the same way, and the tools are
  part of that beginning. A set that changed with every message would throw
  the reading away every time.

## What the measured lesson caught

The first version of this put `write_file`, `edit_file` and `run_command` in
the always-offered set. A test written months ago against a real failure
refused it:

> "Say hello in four words" made the small model propose creating
> /home/Petar/greetings.txt, and the turn ended in an approval prompt instead
> of an answer.

The same thing happened here on Tuesday: asked to say hello in five words, the
model reached for `write_document` and stopped for approval. So the tools that
change things are offered when a message asks for them, and reading — which
cannot go wrong — is always there.

## Measured, on this machine

Same question, same model, before and after:

| | Before | After |
|---|---|---|
| Tools offered | 39 | **18** |
| Tool schemas | ~23,900 characters | **10,600** |
| Whole prompt | **6,338 tokens** | **3,905** |
| Window asked for | 12,288 | **8,192** |
| Answer to "say hello in five words" | `write_document`, an approval prompt, no answer | **"Hello, how can I assist you today?"**, no tools called |

And the shape of a conversation, timed end to end:

| Turn | Prompt | Reused from cache | Wall clock |
|---|---|---|---|
| First, cold | 3,905 | 0 | 13 min |
| A **new** conversation | 3,879 | 5 | 9 min 24 s |
| The **next turn** in that conversation | 3,902 | **3,887** | **17 s** |

Seventeen seconds is the number that matters: it is what using the assistant
feels like once a conversation is under way. The first turn is still minutes
on this hardware, and a new conversation starts cold — the fact extractor runs
between turns on the same model and evicts what was read. That is Ollama's
cache rather than this program's, and it is recorded here rather than
explained away.

## Tests

The rules that were measured lessons are still tested, now against the new
chooser: a website question is not offered the screen; housekeeping appears
when asked for; the tools that only look are always offered; nothing offers to
type unless asked; a conversation is not offered the tools for changing
things. Six new tests cover the budget, the always-offered set, requirements,
stickiness, determinism, and a tool named outright.

**63 packages pass**, `gofmt` and `go vet` clean.

**PHASE 7 COMPLETE**
