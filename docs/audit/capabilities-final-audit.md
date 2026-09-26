# Capabilities and the introduction — final audit

**Date:** 2026-09-26 · **Machine:** Ubuntu 24.04.5, 4 cores, 31 GB, no
graphics acceleration · **Model:** qwen2.5-coder:7b through Ollama
**Method:** every line was run here. What could not be run says so.

## The checklist you set

| | Status | Evidence |
|---|---|---|
| Everything available by default | **VERIFIED** | No capability requires switching on. There is no `Enable*` setting in the program; `TurnedOff` is empty on every brain and takes things *away* |
| No unnecessary permission restrictions | **VERIFIED** | The audit found none to remove: `run_command` already took any program with any argument, and a task already hired for itself. What was artificial was silence, and that is what changed |
| No hardcoded introduction | **VERIFIED** | `canDoList` is gone. The introduction, the greeting and the wizard's opening are written by the model from facts |
| Dynamic capability discovery | **VERIFIED** | `internal/brain/environs`: 42 programs looked for, **31 found** on this machine, cached ten minutes, re-read on demand |
| Dynamic AI context | **VERIFIED** | Every turn carries the inventory; 576 characters, fixed order |
| Dynamic tool selection | **VERIFIED** | `agent.Choosing`: 39 tools became 18, the prompt 6,338 tokens became 3,905 |
| Dynamic agent selection | **PARTIALLY VERIFIED** | It existed before this work and is unchanged: a task hires for itself without asking, and only a hire that *widens* what the organisation can reach needs approval. Not re-exercised end to end in this phase |
| Dynamic model selection | **PARTIALLY VERIFIED** | Also existing and unchanged: the router picks by privacy and availability, the orchestrator decides who codes, automatic selection is on by default with a manual override. Not re-exercised in this phase |
| Proper safety boundaries | **VERIFIED** | Approval gate, stored-argument execution, Landlock, credential paths, argv-only, privacy modes, SSRF, the `Host`/`Origin` rules — all untouched and all still tested |
| Tests passing | **VERIFIED** | 63 packages, `gofmt` and `go vet` clean |
| Documentation updated | **VERIFIED** | The guide describes the panel, the override and what the assistant knows about the machine |

## What the assistant can see

```
On this machine, now:
Thinks with: Ollama 0.34.0
Writes code with: Claude Code 2.1.215, Codex 0.155, Cursor's agent
Languages: .NET 9.0.121, Go 1.26.6, Java 21.0.6, Node.js 22.23.2, PHP 8.4.25, Python 3.12.3
Game engines: Godot 4.7.1, Unity
Editors: Cursor, IntelliJ IDEA, Visual Studio Code
Tools: Blender, CMake, Composer, espeak-ng, FFmpeg, GCC, Git, GitHub CLI, ImageMagick, jq, make, npm, rsync, xdotool
Browsers: Chrome or Chromium, Firefox
Not installed: GitHub Copilot CLI, Ruby, Rust, Unreal Engine — you can say so, and offer to install what this program installs
```

Four states where the program had two: here, needs configuring, needs signing
in, needs licensing, not installed, here-but-unusable, spent, unknown. "Not
configured" is not "not permitted" and neither is "not installed".

## Before and after, measured

| | Before | After |
|---|---|---|
| What the model knew about the machine | nothing | 31 things, with versions and states |
| "what can you do" | 18 sentences written in Go, same on every machine | read off the registry, grouped, with the machine |
| The introduction | one paragraph, identical everywhere | written by the model from this machine's facts |
| The greeting | one phrase, identical at every launch | decided from the state — "Good morning, Petar." on a quiet morning; "three actions waiting for approval" on a busy evening |
| The wizard's opening | generic | names .NET, Java, Python, Git, VS Code, Blender — all found here |
| Tools offered per turn | 39 | 18, chosen and budgeted |
| Prompt | 6,338 tokens | 3,905 |
| A follow-up turn in a conversation | 58 s (when the cache held) | **17 s**, 3,887 of 3,902 tokens reused |
| Switching something off | nothing existed | one list, empty by default, reaching the tools that depend on it |

## Two competing systems, and what happened to them

`orchestrator.Editors()` kept its own list of three editors beside the one in
`environs` that already found those three and two more. It reads `environs`
now.

Three discoverers remain, and deliberately: `provision` owns what can be
*installed* (recipes with installers), `preflight` owns what setup needs, and
the orchestrator owns who *codes* (sign-in state, allowances, failover). Those
are different questions from "what is on this machine", and folding them in
would trade one clear answer for one large one. **Status: PARTIALLY
CONSOLIDATED**, stated rather than claimed.

## What is still written in Go, and should be

- **The persona.** It is the instruction, not the introduction.
- **The wizard's procedural steps** — "Where to keep it", what a drive costs.
  Its *opening* is written by the model; the steps are a flow.
- **The interface's labels and notes** about what a button does. The brief
  allows static technical metadata; a label explaining a control is not the
  assistant's account of itself.
- **Every fallback sentence** behind every phrased line. A machine with no
  model says exactly what the program said before, and that is the only
  honest answer when there is nothing to write with.

## Known limits

- **The first turn of a conversation is minutes on this hardware.** 3,905
  tokens at about eight a second. Follow-ups are seconds.
- **A new conversation starts cold.** The fact extractor runs between turns on
  the same model and evicts what was read; Ollama's cache sometimes restores
  it and sometimes does not.
- **Writing takes a minute or two.** The greeting, the introduction and the
  wizard's opening are written in the background and shown when they arrive;
  until then the composed words stand.
- **A 7B model writes like a 7B model.** Given facts it stays on them; the
  prose is plain and occasionally clumsy. That is the model, not the plumbing.

**PHASE 11 COMPLETE**
