# Phases 3–6 — the assistant can see its own machine

**Date:** 2026-09-24 · Everything below was run on this machine with the local
model (qwen2.5-coder:7b through Ollama, four cores, no graphics card).

## Phase 3 — artificial restrictions

The audit found fewer than the brief assumed, and said so: there was no
enable-this-tool wall to remove. `run_command` already took any program with
any arguments; a task already hired for itself without asking. What existed
was **silence** — capabilities that were absent rather than explained — and
that is what changed.

| Restriction | Verdict | What was done |
|---|---|---|
| Mail and smart-home tools unregistered until configured | keep | they are now **named in the inventory** as waiting for setting up, so the assistant can say what it would take |
| Privacy hides the web tools | keep | now **named as switched off**, with the setting that turns them on |
| Mutating tools stop for approval | keep | untouched |
| Landlock, credential paths, argv-only, SSRF, pairing | keep | untouched |
| Hired agents get a narrowed toolbox | keep | least privilege for delegated work, not a wall in front of the owner |
| Nothing at all was switched off by a setting | — | `TurnedOff` added: **empty by default**, and it takes things *away* |

`TurnedOff` is the shape the brief asked for: everything available, one list of
exceptions. It works by id — `godot`, `docker`, `service:openai`,
`tool:run_command` — and reaches the tools as well as the inventory, so
switching off `docker` also withholds the tools that cannot work without it.

## Phase 4 — the model is told what is on the machine

The persona used to end with where the folders are and said nothing about
what is in them. It now carries this, from the one reading of the machine:

```
On this machine, now:
Thinks with: Ollama 0.34.0
Writes code with: Claude Code 2.1.215, Codex 0.155, Cursor's agent
Languages: .NET 9.0.121, Go 1.26.6, Java 21.0.6, Node.js 22.23.2, PHP 8.4.25, Python 3.12.3
Game engines: Godot 4.7.1, Unity
Editors: Cursor, IntelliJ IDEA, Visual Studio Code
Tools: Blender, CMake, Composer, espeak-ng, FFmpeg, GCC, Git, GitHub CLI, ImageMagick, jq, make, npm, rsync, xdotool
Browsers: Chrome or Chromium, Firefox
Not installed: GitHub Copilot CLI, Ruby, Rust, Unreal Engine — you can say so, and offer to install what this program installs.
```

**576 characters, about 145 tokens**, in a fixed order so the prompt prefix is
still reusable — which on this machine is the difference between a 58-second
second turn and a ten-minute one.

It says nothing at all until the machine has been read once, which takes a
second or two at start-up: a turn that stalls in front of somebody in order to
describe their own machine has made a poor trade.

## Phases 5 and 6 — the introduction and the help

### What was removed

`tools/introduce.go` held `canDoList`: eighteen entries, each with fixed prose
and a fixed example — *"read a drive or folder and remember what is in it —
say \"learn everything\""*. The same paragraph for every person on every
machine, describing eighteen tools of seventy-three and never mentioning what
was installed.

### What replaced it

`what_can_you_do` now reports **what is loaded**, grouped under headings a
person would recognise, each tool described in its own words, followed by the
machine block. The groups come from the capability table that already existed,
so a tool added with its capabilities named lands in the right group without
anybody editing this file.

Two readers, one set of facts: the model is told to say it in its own words,
and a person — when there is no model to phrase it — sees the facts without a
note addressed to somebody else.

### The introduction is written by the model

Measured on this machine, from a fresh brain:

**Before** (what every person on every machine saw):
> read a drive or folder and remember what is in it — say "learn everything" ·
> read one folder or document now — say "learn the documents in that folder" · …

**After** (written by the local model, from the facts of this machine):
> I'm an AI assistant with 73 tools, including remembering and learning, file
> management, writing documents, coding, game development, web access, and
> more. On this machine, I have Ollama 0.34.0 and qwen2.5-coder:7b for
> thinking, and tools like FFmpeg and CMake for various tasks.

It is written from a **shape** rather than the full list — four thousand
characters of tool descriptions is two minutes of reading on this machine
before a word is written, and an introduction needs the shape, not the
wording. The full list is what somebody gets when they actually ask.

Written once at start-up, in the background, and kept against the facts it was
written from — so installing something and opening the program again gives a
different introduction. While it is being written, the facts are shown; if
there is no model at all, the facts are what there is. Neither is a failure,
and neither is a paragraph pretending to be one.

## What is still hardcoded, honestly

- The **persona** — who the assistant is and how it behaves — is still written
  in Go, and should be: it is the instruction, not the introduction.
- The **setup wizard's prose** (2,857 lines) is untouched. Approved to become
  model-written; not done in this phase.
- The **interface's 88 explanatory paragraphs** are untouched. Same.
- The fallback sentences behind every phrased line remain, by design: a
  machine with no model says exactly what the program said before.

## Tests

`environs` 16, `tools` 5 new, `brain` 4 new, and one existing test rewritten —
it asserted the hardcoded phrases this change removes, and now asserts that
the introduction is read off the registry and that the old list is gone.

**63 packages pass**, `gofmt` and `go vet` clean, versions order correctly.

**PHASES 3, 4, 5 AND 6 COMPLETE**
