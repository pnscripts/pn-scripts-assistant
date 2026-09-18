# ADR 0005 — Choose how work is done, and hold it to its boundaries

**Status:** accepted · **Date:** 2026-09-18

## The question

ADR 0004 made the assistant able to hire, equip and confine. Two audits that
day found where that fell short in practice — confinement a command could walk
out of, "finished" decided on stale evidence, a Godot template that real
Godot rejected, Unity offered as ready while unlicensed — and asked for the
next step: its owner says **what** they want, and the assistant decides
**how** — which engine, which coding agent, which model, what takes over when
that fails — preferring subscriptions already paid for, switching before a
limit is hit, installing a local model when its policy allows, and never
crossing an approval, privacy, confinement or evidence boundary to do it.

The owner approved both audits ("make the best and approve"): fix the
audit's findings first, then build the orchestration on what exists.

## The real machine, as found

| Resource | State found | How it is known |
|---|---|---|
| Claude Code 2.1.215 (Claude Max) | **signed out** — "OAuth session expired" | `claude auth status` → `loggedIn: false`; a headless run said so |
| Codex 0.154 (ChatGPT app, free plan) | **out of allowance until 15 Oct 15:31** | its own `--json` error and session log |
| Cursor agent 2026.01.28 | not usable from here; sign-in not checked | checking starts a login and rewrites its settings |
| VS Code, Cursor, IntelliJ | editors — never executors | installed |
| Ollama 0.34, qwen2.5-coder:7b | **available**, CPU only | `/api/show`: tools, insert, 7.6B, 32k |
| Unity 6000.5.4f1 (Personal) | **installed, not licensed to work unattended** | batch mode: "No valid Unity Editor license found" |
| Godot 4.7.1 (desktop copy) | usable | real check, run and picture |
| Unreal | not installed | searched |
| mutter 46.2, Landlock ABI 4 | a private display; kernel confinement | used |

So today every piece of coding is chosen to go to the local model — and says
why the others are not used.

## What was built

### First, the audit's findings (Phase 0)

| Finding | Fix |
|---|---|
| `cp a ../x`, `bash -c`, `python -c` wrote outside a project | **Landlock**: commands, engines and MCP servers on a project run in a copy of this program that confines itself to the project, the temp folder and tool caches (`sandbox`) |
| tools that did not say where they wrote were allowed | confinement **fails closed**; `make_subtitles`, `put_back` declare their paths |
| an agent could edit its own `project.json` (and its integration list) | `.pn-assistant/` is not writable by the work; settings **widened** mid-task stop it |
| a failed latest check still counted | only the **latest** result of each kind counts, and only **after the last change** |
| tasks without a project needed no evidence | every task **with packages** is judged |
| the untouched template passed as a game | a **one-colour picture fails** the run |
| Godot's template failed its own check | preset fixed; tested against **real Godot** |
| no pictures without Xvfb | **mutter headless** on a private, silent bus (4.9 s) |
| Unity "ready now" while unlicensed | a **licence probe** (stops Unity once licensing decides); proposals block with the reason |
| never-stop sent mail and installed without asking | email, every install path, destructive commands and off-machine integration calls are **always asked** |
| one-click permanent hire with every tool | the Organisation view **proposes**; an empty tool list is refused |
| "hire a doctor" proposed a Physician | jobs keeping a person in the loop are **advisory** and name the professional |
| false matches (financial advisor → academic) | the **role word** decides; catalogue gaps filled |
| an owner file could strip the electrician's limits | shipped safety fields cannot be **weakened**; the job's oversight is the floor |
| agent memory crossed projects | recall is **per project** |
| secrets in evidence | known values and token shapes **scrubbed** before recording |

### Then, the orchestrator

```
request ─► task understanding (bootstrap, packages, team.Needs)
        ─► discovery (orchestrator.Resources: machine, agents, local models,
           services, engines — each from the package that knows it; cached 1 min)
        ─► selection (orchestrator.Select: unusable set aside with reasons;
           suitability band ▸ subscription first ▸ capacity ▸ history)
        ─► policy (privacy mode, thresholds, budgets, autonomy; project and task
           overrides only narrow)
        ─► approval gate (unchanged; always-asked classes)
        ─► execution (conductor: engine actions by this program; writing by the
           chosen agent or model; failover down the fallbacks)
        ─► evidence (decision, switch, install, files, checks, pictures)
        ─► history (resource_runs: ties broken by what worked here)
```

- **One decider.** `orchestrator.Select`/`Decide` is the only place a resource
  is chosen; the unwired `orchestrator/` another writer left was replaced by it.
- **Nothing invented.** Capacity is a number only when a service says it
  (Codex's log and events; Claude Code's rate-limit events); otherwise
  "unknown", and said so.
- **Agents as themselves.** Claude Code runs with file tools and no shell,
  Codex in its own sandbox; neither gets this program's environment, and no
  credential file is opened. A file an agent writes outside the project stops
  the work.
- **Checking is not a model's word.** Check, run and build are *actions* this
  program performs with the engine; a failure goes back to the writer with
  the error, file and line, up to three times.
- **Engines chosen, not asked** — from what the request says (browser, 2D, 3D,
  phone, high-end) and what each engine has proven here.
- **Local models** judged by `/api/show` capabilities; installed within the
  approved policy after checking the registry manifest (size, licence,
  digest), then proved by id, capabilities and an answer.

## Decisions

1. **Privacy for work is its own axis** (cloud preferred, balanced, local
   preferred, local only; default balanced, as approved); the conversation
   router's own guard is unchanged.
2. **Autonomy was approved once**: local models and pinned tools install
   without asking within 10 GB, keeping 20 GB free; nothing paid per use
   without a budget (none set).
3. **Cursor's agent is detected, not driven**, until it can be checked without
   starting a login.
4. **Claude Code and Codex never run with their bypass flags.**

## Verified

On this machine, not only in tests:

- **Kernel confinement**: `cp a ../x`, `bash -c "echo > ../x"` and
  `python3 -c "open('../x','w')"` under the sandbox write nothing outside the
  project; writing inside works; stopping a command stops what it started.
- **Real Godot 4.7.1** accepts a new project; a runtime error is placed at
  `main.gd:5`, a parse error at `main.gd:4`, once each; the empty template's
  picture is refused as one colour and a drawing game's picture passes — taken
  on a private mutter display with no Xvfb installed.
- **Unity's licence probe** on the installed 6000.5.4f1: *inactive* in 0.34 s,
  with Unity's own words ("Access token is unavailable", "'com.unity.editor.headless'
  was not found"); the licence file untouched.
- **Discovery on the real machine**: Claude Code signed out (`auth status`),
  Codex out of allowance until 15 Oct 15:31 (its own log), Cursor's agent not
  driven, three editors as editors, qwen2.5-coder:7b by its capabilities — and
  coding therefore given to the local model, with each rejection's reason.
- **The real agents' failures read correctly**: Claude Code's
  `authentication_failed` and Codex's "You've hit your usage limit … try again
  at Oct 15th" become *signed out* and *out of allowance until*.
- **On screen, in an isolated scratch brain**: the "How work is done" card, the
  policy form, a project proposal that chooses and explains its stack, and
  starting a project asked even on "never stop".
- **Tests**: failover past a signed-out agent keeps the step's work and
  records the switch; an agent writing outside the project stops the work; a
  failing check goes back to be fixed with its file and line and passes; every
  audited escape is refused; stale and missing evidence block; hiring near
  misses; package weakening refused; memory per project; secrets scrubbed.

## End to end, on this machine

Each run in its own scratch brain — its own home, settings, data and a
`PATH` of links — so nothing touched the owner's brain or settings, driven
through the program's own page.

- **Unity (T3).** "Create a Unity game where a ball rolls through a maze":
  Unity chosen because it was named; the proposal lists Unity's own licence
  errors as what it cannot start until, says the owner signs in to Unity Hub,
  and offers no approval. "yes, start it" is answered with that blocker and
  starts nothing.
- **three.js, first run (T2).** The engine was chosen and explained, the
  decision recorded ("writing: qwen2.5-coder:7b (local)", with why Claude Code
  and Codex were not), the local model wrote the game — and this program's own
  check found two real errors in it: an import of `./vendor/three.module.js`
  from inside `src/`, and a missing bracket at `main.js:21`. Both went back to
  the writer with file and line; its edits did not match the file, and the
  task stopped at its half hour as **blocked, not done**, saying so.

What that run changed:

| Seen | Changed |
|---|---|
| a small model retyped the broken line from memory and its edit was refused | a failing check now quotes the lines it points at, exactly |
| half an hour is not enough for a 7B model on four cores | an approved project is given 90 minutes when a local model writes it on a machine with no graphics card — said in the proposal, approved with it |
| "100 % of its work here has been verified" after every check failed | the record counts only work that later passed this program's check |
| "yes, start it" went to the model, which called the wrong tool twice | a run of plain yeses is recognised, and a blocked proposal answered directly |
| a decision said why its choice was made, not why the others were not | it says both |

- **The first choice unavailable (T4).** A `PATH` whose `claude` answers
  `auth status` with "signed in, max" and hands everything else to the real
  Claude Code — which, signed out, fails for real. The proposal: "Writes it:
  Claude Code … the max subscription already paid for; how much of it is left
  is not known … If that fails: qwen2.5-coder:7b (local)". On approval Claude
  Code was given the step and failed in 6 s with its own words ("OAuth session
  expired and could not be refreshed"); the switch was recorded with that
  reason, the owner told, the step's writer became the local model, and it
  took the same step over — 8 s from approval. (Stopped there: the local
  writing that follows is the same as T2's.)
- **A local model missing (T5).** An Ollama of its own with nothing in it,
  and a policy that installs models without asking up to 1.2 GB. The proposal
  said who would write it and on what terms — "qwen2.5-coder:1.5b (local),
  installed first … 940 MB, licence Apache — installed without asking, within
  your policy" — and the task, once approved, recorded each stage: *planned*
  (940 MB from registry.ollama.ai, manifest `sha256:d7372fd8…`, 40.6 GB free
  before), *pulled* (39.7 GB free after), *its id matches the planned
  manifest*, *capabilities: completion, tools, insert*, *answers: ready* —
  and only then chose it to write, in about a minute from approval. It chose
  the best coding model with tools that fits the 1.2 GB the policy installs
  without asking.

- **Godot (T1).** "Create a small Godot game where a paddle bounces a ball":
  Godot 4.7.1 (the copy on the desktop) chosen because it was named, 90
  minutes given. The local model twice answered without writing — judged so
  and tried again — and the step went to a specialist, who wrote `main.gd`.
  Real Godot then rejected it (statements outside functions, Godot 3's
  `.instance()`, two scenes that do not exist): 2 errors, with file and line.
  The task ended **blocked**: "its steps are done, but nothing yet shows a
  check that passed (the latest one failed: 2 errors), a run that did not
  fail". Nothing about the game was reported as working.

The runs after it found more, each seen on this machine before it was fixed:

| Seen | Changed |
|---|---|
| the local model only looked at the folder, said "the game's files are written", and its look was taken as proof — the untouched template went on to be checked | a project's writing, by an agent or a model, is judged on the files it changed and nothing else; a writing run that changed nothing is recorded as failed, so a later check is never credited to it |
| the check passed on the template; the **picture** did not: "one flat colour: nothing visible was drawn" — then the task stopped as blocked, "nothing yet shows a run that did not fail" | (the audit's rule, working as intended) |
| after a failover, the decision explained the local model with Claude Code's reasons ("the max subscription") | each fallback carries its own reasons |
| the step went on naming the member's usual model, not the one writing it | a writing step records who is writing it, and who took over |
| a 1.5B model, handed `game_build`, built the untouched template twice instead of writing | a model writing a project gets its files and nothing that runs, checks or builds them |
| stopping a task while its writer worked recorded the writer as failing and the task as blocked | a stopped step is ended, not failed: nothing goes on the writer's record and the task says "you stopped it" |
| both three.js runs imported three by a path from `src/`; one fix sent `from "./…"` for `from './…'`, three times | the package says how to import three; the check says "import it as 'three'"; an edit that nearly matches shows the line as it really is |
| install evidence was all written after the download | each stage is recorded as it happens |
| Godot (T1), first run: one answer from the local model with no tool call in it — 715 tokens in 23½ minutes, with the machine loaded — and the task stopped at once, because writing nothing had just been made an error (by this work, above) | writing nothing is judged by the step's own check and tried again, by the same writer if it is the only one; only "nobody can write it" stops a task |
| Godot (T1), second run: judged and retried as intended, then handed on — to the **three.js** developer hired for an earlier task, on the word "game" | a step is handed only to somebody free to take it: not another task's hire, and for a project, somebody who works under its packages |
| a Godot step handed on became a task with no project — so none of the project's confinement, orchestration or judging applied, and what the specialist wrote was not in the project's record | a handed-on step keeps its project and packages; the project is judged on its children's evidence too; a one-step child is not judged as the whole project |
| the Godot check and run, failing, were handed to specialists, asked to "check with game_check", and counted as done on their word | this program's own check, run and build are never handed on: after its fixing rounds a failing one fails, with the engine's words |
| a fix was asked with the failed step's own words — "run the game with game_build" — and a 7B model answered in six words and changed nothing, three times | a fix is asked as what it is: change the files so that it passes, with what was wanted; a round that changes nothing ends the fixing |

## Not done, and why

- **Claude Code is signed out on this machine**, so no piece of work here has
  yet been written by it; its adapter is proved by its real failure and by
  tests, not by a real success. Signing in is the owner's to do (`claude`,
  then `/login`); nothing here signs in for anyone.
- **Codex is out of allowance until 15 Oct**, so the same holds for it.
- **Cursor's agent is detected, not driven** (decision 3).
- **Unity cannot work unattended** until it is signed in to through Unity Hub;
  a Unity game is proposed with that as its blocker, never started.
- **Pay-per-use spending is not counted.** No service here says what a call
  cost, and a typed-in price list would be invented, so a paid service is
  never chosen on its own — only when a task names it and a budget is set.
- **Claude Code's remaining capacity** is known only from the rate-limit
  events it sends while working; signed out, it sends none, so it is shown as
  *unknown* rather than as a number.
- **Local models run with Ollama's default context here (4 096 tokens)**,
  though qwen2.5-coder is trained for 32 768. Ollama drops the oldest messages
  to fit — which can include a step's instruction on a long step. The steps
  run here fitted (1.8–2.4 k tokens). Raising it is a speed and memory
  trade-off on this processor, and changing it on a shared Ollama makes every
  other program using the same model reload it, so it is left as it was.
- **No graphics card**: a step written by the local model takes minutes (the
  first three.js step, 9 min 47 s), which is why a subscription is preferred
  whenever one is signed in and has capacity.
- **A stopped task keeps its temporary hires**, because it can be picked up
  again; they are no longer lent to other tasks, but they stay on the roster
  until their task finishes or is removed.
- **Two local-model projects at once share one lane**, and both clocks run:
  approved together on this machine, each gets half the model and the whole
  of its deadline counts. Approving them one after another avoids it; making
  a deadline pause while a step waits for the lane is not done.
- **The local model is the weak link.** In the three.js runs here,
  qwen2.5-coder:7b wrote the game's main file each time with an error in it —
  a missing bracket, a wrong import path — and could not then change it
  exactly; once it only looked and wrote nothing. The 1.5B model built instead
  of writing. The program's checks caught every one of these, and none was
  reported as done.
- **Integrations (MCP servers)** are shown with the other resources, with
  their honest state; which of them a piece of work may use is still decided
  by their grants, as before — the orchestrator never chooses one.
