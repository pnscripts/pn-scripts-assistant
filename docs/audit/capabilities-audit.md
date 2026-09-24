# Capabilities, permissions and the introduction — audit

**Date:** 2026-09-24 · **Commit:** `0c29bc5` · **Machine:** Ubuntu 24.04.5
**Method:** every count and quotation below was read out of this repository
today. Nothing is inferred from memory of how it "probably" works.

This is Phase 1 of the change you asked for: understand before touching.
No code has been changed.

## 1. How capability works today

### The tool registry

`tools.Registry` holds **74 tools**, constructed once in `brain.New`
(`internal/brain/brain/brain.go:227–618`) and handed to the agent loop. Each
tool declares `Name`, `Description`, `Parameters` (JSON Schema), `Risk`,
`Summarize` and `Execute`. Two optional interfaces exist: `Cued` (offer only
when the message mentions something) and `Serves` (which professions this tool
belongs to).

By risk: **43 Safe** (run without asking) and **31 Mutating** (stop for
approval). There is no allowlist inside the dangerous ones — `run_command`
takes any program with any arguments, as an argv array with no shell:

> "The brain runs on the machine it is asked about, so a command it is given
> simply runs. There is nothing between it and the computer."
> — `internal/brain/tools/command.go:26`

That is already the model you are asking for: **the capability is not
restricted; the action is confirmed.**

### What stops a tool from being offered

Four mechanisms, and they are not the same kind of thing:

| # | Mechanism | Where | What it does |
|---|---|---|---|
| 1 | **Not registered** | `brain.go:259–305` | Mail and smart-home tools are only added when a mailbox or a Home Assistant address is configured |
| 2 | **`OffLimits`** | `brain.go:648–670` | Hides `fetch_url`, `web_search`, `read_a_page` and any integration that leaves the machine, unless privacy allows the web |
| 3 | **`relevant()`** | `internal/brain/agent/relevance.go` | Per turn, drops tools whose subject is not mentioned — `list_models`, `set_appearance`, `run_command`, `type_text`, the organisation's own tools, and anything `Cued` |
| 4 | **`ForCapabilities`** | `tools/capabilities.go:157` | For a **hired agent**, narrows its toolbox to what its profession needs. Not applied to the conversation |

Mechanism 1 is *configuration* expressed as *absence*. Mechanism 2 is
*privacy*. Mechanism 3 is *prompt economy*. Mechanism 4 is *least privilege for
delegated work*. Only the second is a permission rule, and it is about what
leaves the machine rather than what may be done on it.

### The approval gate

`permits` (425 lines) is a grant book: each decision is `ask`, `allow` or
`refuse`, optionally scoped to one agent, with the sentence the owner was shown
when they decided. Over it sits one setting with three values —
`ask` (default), `granted`, `everything` — and everything that runs is
recorded either way.

A Mutating tool never executes inside the model's turn: the call is stored, the
turn stops, and approval runs the **stored arguments**, not whatever the model
says next.

### Agents

`occupations` seeds about a hundred professions (backend engineer, DevOps
engineer, designers, researchers…) as rows, not running things. An agent is
hired into the organisation from one of them. A task **may hire for itself
without asking**; approval is required only when the hire *widens* what the
organisation can reach — permanent, a hosted provider, or an MCP integration
(`team/proposal.go:1077`, `Widens`). So agents are not behind a manual switch;
some hires are.

### Models

`llm.Router` picks between the local model and configured services by privacy
mode and availability, with automatic selection on by default (`AutoModel:
true`) and a manual override. The orchestrator decides who *codes* — Claude
Code, Codex, or a local model — with failover and recorded evidence.

### Discovery that already exists

Four separate discoverers, none of them aware of the others:

| Where | Discovers | Used by |
|---|---|---|
| `orchestrator/discover.go` | coding agents (Claude Code, Codex, Cursor), editors (VS Code, Cursor, IntelliJ), local models, engines, runtimes, integrations, services | **tasks only** |
| `provision/recipes.go` | 10 recipes: npm, Go, cargo, Chrome, uv, Godot, Godot templates, Node, Unity, Unreal | project requirements |
| `preflight/requirements_list.go` | Ollama, chat model, embedding model, speech, listening, window libraries | the setup wizard |
| `engines` | game engines this program can drive | game tasks |

`orchestrator.Resource` is already close to the registry you describe: id,
kind, title, version, path, **state** (`available`, `auth_required`,
`not_installed`, `not_licensed`, `low_capacity`, `limit_reached`, `offline`,
`failed`, `unknown`), evidence level, observation time, `Local`, and a
capability score map. It is cached for a minute.

**Not discovered anywhere:** git, docker, php, laravel, python (only `uv`),
browsers in general, GitHub CLI, system services, arbitrary desktop
applications.

## 2. What actually limits the assistant

### F1 — the model is never told what is on this machine · **artificial**

The system prompt contains the persona, a list of where things are (home
folder, data root, mounted drives), what is remembered, and the JSON schemas of
the tools it may call. **It contains no inventory of the machine.** Godot,
Unity, Claude Code, Codex, VS Code, Go, Node, docker, git — all discovered
elsewhere in the program, none of it reaches the conversation. The
orchestrator's resource list is built for tasks and never given to the chat
model (`brain/orchestrate.go`; no call from the prompt builders).

Asked "can you make a Godot game?", the assistant answers from its tool
schemas, not from the fact that Godot 4.3 is at a known path.

### F2 — "not configured" is indistinguishable from "cannot" · **artificial**

Mail and smart-home tools are simply absent until configured. The reasoning in
the code is sound as far as it goes —

> "a tool the model can see is a tool it will try, and offering to read mail it
> cannot reach produces a brain that promises to check and then reports an
> error every time" — `brain.go:276`

— but the consequence is the exact conflation you named: the assistant cannot
say *"I can read your mail once you add the mailbox"*, because it does not know
the capability exists. The fix is not to register broken tools; it is to give
the model the **inventory** with states, separately from the callable schemas.

### F3 — privacy hides the web tools, silently · **genuine rule, artificial silence**

Default privacy is `private`, so `fetch_url`, `web_search` and `read_a_page`
are hidden. That is a deliberate product decision about what leaves the
machine and it should stay. What should not stay is the silence: the model is
not told these exist and are off, so it cannot say *"I could look that up if
you switch privacy to research."*

### F4 — the capability list shown to people is hardcoded · **artificial**

`tools/introduce.go` holds `canDoList`: **18 fixed entries**, each with fixed
prose and a fixed example sentence ("learn everything", "make the core green").
It is filtered against the registry, so it cannot promise a tool that is not
loaded — but it describes 18 of 74 tools, says nothing about the machine, and
its wording is written in Go. This is what `what_can_you_do` returns, and what
the first-run greeting shows (`greeting.go:206` → `WhatItCanDo()`).

### F5 — the first-run experience is written in Go · **artificial**

- `internal/setup/page.go` — **2,857 lines**, the whole wizard including its
  prose.
- `internal/brain/server/assets/index.html` — **2,280 lines**, with **88**
  hardcoded explanatory paragraphs.
- `internal/brain/bootstrap/text.go` — 162 lines of project-proposal prose.
- `team/proposal.go` — `Text()`, the hiring proposal written out in Go.

The *spoken* greeting is already AI-phrased from facts (`greeting.go`,
`wording` package, added yesterday). The written introduction beside it is not.

### F6 — per-turn tool narrowing is keyword-based · **quality measure, now a limit**

`relevant()` drops tools by matching words in the message. It exists for a
real, measured reason: a 3B model asked about a website called `list_models`;
asked to "say hello in four words" it proposed creating a file. But it means
capability selection is a static keyword table rather than a decision informed
by the task — which is what your §15 asks for.

### F7 — four registries, no single source of truth · **architectural**

Discovery is split four ways (table above) with different types and different
states. Your §22 asks for one. This is the largest structural change.

### F8 — hired agents get a narrowed toolbox · **genuine, and worth keeping**

`ForCapabilities` hands a hired agent only the tools its profession needs, so a
writer cannot be talked into fetching a page. This is least privilege for
delegated work, not a wall in front of the owner. It should stay, and the
assistant itself is not subject to it.

### F9 — integrations need approval · **genuine**

An MCP server is somebody else's code, often reaching off the machine, granted
per agent and per project. Keep.

## 3. What is genuinely load-bearing and must not be removed

| Boundary | Why |
|---|---|
| Mutating tools stop for approval, and approval runs the **stored** arguments | The model cannot change what it asked for between the question and the answer |
| No shell, ever — argv arrays only | There is nothing to inject into |
| Landlock confinement for project work | The kernel, not a string check, keeps work inside the project |
| Credential paths refused on read and write | In code, not in a prompt |
| Privacy modes controlling what *leaves* | The product's central promise |
| SSRF checks on resolved addresses | A page cannot redirect the assistant onto the loopback interface |
| Pairing, TLS, the `Host`/`Origin` rules added yesterday | A web page must not be able to drive this |
| Everything recorded as evidence | What makes any of the above reviewable |

None of these restricts *which capabilities exist*. Every one of them governs
*what happens when one is used*. That distinction is already the architecture
here; it is not something that has to be introduced.

## 4. The honest constraint nobody can design around

Measured yesterday on this machine, with the packaged build:

| | |
|---|---|
| A conversation turn, 39 tools offered | **6,338 tokens** |
| Of which the persona | 1,224 |
| Of which the tool schemas | **~5,100** |
| Prompt processing, 4 cores, no graphics card | **~8 tokens/second** |
| First turn in a conversation | **598 s** |
| Second turn (prefix reused from cache) | **58 s** |

"All tools available" cannot mean "every tool's schema in every prompt". At
74 tools the prompt would be roughly 9,500 tokens and the first answer would
take twenty minutes on this hardware. Whatever replaces `relevant()` must
**select** what is presented per turn while leaving everything reachable — and
must keep the shared prefix stable, because that is what makes the second turn
ten times faster than the first.

This is the single most important design constraint in the whole change, and
it is a measurement rather than an opinion.

## 5. What has to change

1. **One capability registry**, built from one discovery layer, with the
   states you named: available / unavailable / needs configuration / needs
   confirmation. `orchestrator.Resource` is 80% of it already.
2. **Discovery widened** to the things people actually have: git, docker,
   python, php, composer, node, browsers, the GitHub CLI, editors, engines,
   coding agents, and arbitrary programs found on the PATH.
3. **An environment block in the prompt**, generated from the registry,
   compact, stable, and placed where it does not break the model's cache.
4. **Tools declare what they require**, so "this tool needs Godot" is data
   rather than a guess, and a tool whose requirement is missing can be
   described rather than hidden.
5. **`canDoList` deleted**; `what_can_you_do` answers from the registry and
   the environment; the written introduction is phrased by the model from
   facts, as the spoken one already is.
6. **`relevant()` replaced** by selection that uses the registry and the task,
   with a hard budget on how much schema goes into a turn.
7. **Overrides instead of switches**: a `disabled` list in settings, empty by
   default.

## 6. What this audit does **not** recommend

- Removing the approval gate, the sandbox, the credential rules or the privacy
  modes. You asked for capability, not for unrestricted action, and those are
  the mechanisms that make the difference real.
- Registering tools that cannot work. A tool that is offered and always fails
  teaches the model to distrust its own tools; the inventory is the right place
  to say "this exists but needs a mailbox".
- Rewriting the setup wizard's mechanics. Its *prose* can become AI-generated;
  its steps are a flow, not a personality.

**PHASE 1 (AUDIT) COMPLETE — awaiting approval before any code changes.**
