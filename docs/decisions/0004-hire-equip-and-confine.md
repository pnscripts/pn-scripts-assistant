# ADR 0004 — Hire, equip, and confine

**Status:** accepted · **Date:** 2026-09-18

## The real question

ADR 0002 made the assistant an organisation: jobs, seats and agents, hundreds
of professions, a shortlist for the planner. What it could not yet do was the
thing the owner actually asks for — "hire a game developer", "make me a Tetris
game", "help me plan the electrical work for this room" — and have the result
be *equipped*: the engine installed at a version that works, the documentation
for that version, the tools the work needs and none it does not, a folder it is
confined to, a record of what it did, and — for an electrician — the line past
which an AI stops and a licensed person takes over.

The decision this records is that **all of that is declared, not coded, and
nothing material happens without a proposal its owner approved.** Game
development is the first complete family, not the shape of the system.

## The seams, as they were

Found by reading the code before changing it:

| Seam | Where | Used for |
|---|---|---|
| The approval gate | `agent.Loop.MayI`, `permits.Book.DecideAt`, `brain.Decide` | every change; approving executes the *stored* arguments |
| The narrowing rule | `tools.Allowed`, `team.Fit.Only`, `tasks.held` | an agent may never do more than its owner |
| Optional tool interfaces | `Cued`, `Serves`, `Weighed`, `Replaceable` | the pattern every new capability follows |
| The one download door | `preflight.download` (allowed hosts, size bound, part file) | every install |
| Machine parts | `preflight.Requirements` | the assistant's own dependencies |
| The conductor | `tasks.Conductor` prepare → think → settle | work that outlives a sentence |
| Hiring | `team.Hire` | one step: a sentence in, a file written |

## What was added, and where it lives

| Concept | What it is | Where |
|---|---|---|
| **Capability package** | what a role needs to work safely: role, tools and denials, skills, knowledge, requirements, integrations, workspace, evidence, risk, limits, escalation | `capability/`; 11 ship as JSON, owner's own in `<root>/packages/*.json` |
| **Recipe** | one thing a package needs on this machine: detect + version, source, licence, cost, size, install location, the one fixed install, verify, smoke test | `provision/`; the machine parts are recipes too |
| **Pinned release** | a download fixed by version *and* checksum | `preflight/pinned.go` (Godot 4.7.2 SHA-512, Node 24.21.0 SHA-256) |
| **Hire proposal** | a hire worked out and not made | `team/proposal.go`; kept whole in the `proposals` table |
| **Project proposal** | "make me X": where, what with, who, what it needs, the plan, what finished means | `bootstrap/`; kept whole in `proposals` |
| **Project settings** | roots, outputs, commands, conventions, acceptance, integrations, memory, skills, evidence | `<project>/.pn-assistant/` — source-controllable, the project's, never the owner's memory |
| **Scope** | where work on a project may write and run, resolved through links | `workspace.Scope`, enforced in the loop (`Brief.Within`) |
| **Evidence** | what a task can prove it did: exact argv and exit, files, checks, builds, pictures, versions, approvals | `evidence` table; `tools.Perform` gathers it from every call |
| **Engine adapter** | detect, docs for the version, scaffold, validate, check, build, smoke + picture, diagnostics | `engines/`: Godot, three.js, Unity, Unreal; Phaser, Babylon.js, PlayCanvas, Bevy; Defold and GameMaker recognised |
| **Integration** | an MCP server: approved, granted per agent and per project, audited | `mcp/`; `<root>/integrations/servers.json` and `secrets.json` (0600) |

## How a request becomes work

```
"Make me a Tetris game"
   │
   ├─ no folder ──────────────► "Where should I create or work on this project?"
   ├─ folder inspected ───────► refused (system, home, hidden, the brain's own) → ask again
   │                            not empty, not a project → a folder of its own inside it
   ├─ no engine named ────────► "Which engine?" with what each would take here
   ▼
proposal ── files (never overwriting), package, engine + version, who (somebody
   │         here under the package, or a hire for this project only), what the
   │         machine needs and what would be installed, the steps, what finished
   │         means as evidence, limits, who else is needed
   ▼
"yes" ──► an approval, always asked (start_project is Consenting)
   ▼
set up ── install what it listed (pinned, checksummed, proved by version and a
   │      smoke run) → lay the files (all or nothing, each recorded for undo) →
   │      take on the hire → start the task with the plan it showed
   ▼
every step ── confined to the project's folders (refused, not asked, outside
   │          them) · told the project, its conventions, skills and memory, and
   │          the package's know-how and limits · evidence kept from every call
   ▼
finished only on evidence ── the package's list (a check, a run, a picture…);
                              steps done without it → blocked, saying what is missing
```

Hiring in conversation is the same shape: `hire_agent` proposes and writes
nothing; the owner is asked permanent or one task (and which task); making a
permanent hire is always asked; a task-only hire starts its task and is let go
when it ends.

## Decisions, and what each costs

**Declared, not coded.** No `if job == electrician`. A package says what an
electrician's role may use (reading and writing documents) and must not
(`set_device`, `run_command`…), that the work needs a professional, and who.
The cost: a package is only as good as whoever wrote it — so rules that must
hold are checked in code when it is read, and a package that breaks one is left
out *whole*: advisory work may not carry an operating tool; work needing a
professional must be advisory and must name who; a role must list its tools.

**A hire is never unrestricted.** Its tools are always a written list; with
nothing to derive them from, the list is `none`. A job that keeps a person in
the loop gets nothing that acts on the world from what its capabilities map to.

**Always asked, whatever the setting.** Installing, starting a project in the
owner's folders, switching an integration on, and a permanent hire implement
`tools.Consenting` and are put to the owner even on "never stop". A refusal
still wins. *This narrows what "never stop" meant before; see Open questions.*

**Nothing an agent writes becomes an install.** Installs are recipes shipped in
code; the model can name a recipe, never a URL or a command. Unity, Unreal,
Rust and Chrome are detected and described — account, licence, cost — and never
installed or agreed to on anybody's behalf.

**Project work is confined, and the refusal is not a question.** A write or a
command outside the project's roots is refused before the gate, so no approval
can be asked for — or given — for it. Paths are judged where they really are.

**Complete means evidence.** "I wrote the game" is a claim; a `game_check` that
passed, a smoke run that threw nothing and a picture are evidence. Tasks under a
package finish only when its evidence exists.

**MCP is an integration, not a bypass.** Each server tool becomes one of this
program's tools: `Mutating` unless its owner marks it read-only (a server's
`readOnlyHint` is shown, not trusted), offered only to agents it is granted to
and on projects that allow it, hidden while privacy forbids what it would do,
its description marked as the server's, its answer framed as information. Every
activation, call and read is recorded as sizes and digests, never content.
Secrets live in their own 0600 file and are scrubbed from anything shown. Both
protocol eras are spoken: `server/discover` first (2026-07-28, stateless), the
`initialize` handshake as fallback — the reference server in the catalogue
answers the old way today.

**Godot is the first adapter, not a special case.** `godot_build` now runs
through `engines.Godot`; `godot_status` and `godot_docs` keep their names. The
Godot installer is pinned to 4.7.2 and checked against the release's SHA-512
instead of fetching whatever is newest.

## Verified

- Unit and integration tests across `capability`, `provision`, `team`,
  `bootstrap`, `workspace`, `engines`, `mcp`, `agent`, `tasks`, `store`,
  `server` and `brain` (the conversation flows end to end, and every shipped
  package loading on a real brain).
- A real three.js Tetris: node parsed both modules, the page ran in this
  program's own WebKit and was photographed, and a runtime error was reported
  with its file and line.
- The real MCP reference server (`@modelcontextprotocol/server-everything`
  2026.8.31) through the gateway: connected (legacy 2025-11-25), tools listed and
  called, a resource read, health checked, audit written.
- On screen, in an isolated scratch brain: the hire proposal and hiring for one
  task, the project flow from question to started task with its evidence,
  packages, the machine, integrations started from their card, and the chat
  shortcut ending in an approval.

## Not done, and why

- **Unity and Unreal were not run.** *Corrected 2026-09-18:* this first said
  neither was installed. Unreal is not; **Unity is** — Hub 3.14.3 and editor
  6000.5.4f1 under `~/Unity/Hub/Editor` — and its batch mode refuses to start
  ("No valid Unity Editor license found", no Hub session). The claim was made
  without running detection on the machine; see ADR 0005 for what is now
  probed and reported.
- **Godot screenshots need a virtual display** (`xvfb`, a recipe); without it
  the smoke run is headless and says why there is no picture.
- **Approved project commands still ask.** `.pn-assistant/project.json` records
  them; granting them a standing yes scoped to the project would need
  argument-level grants in `permits`, which this does not add.
- **Updating a recipe in place** is not offered; installing a newer pinned
  release is an edit to `pinned.go`, and removing is `Take`.
- **An agent cannot hire mid-turn**, as in ADR 0002: hiring stays a proposal
  its owner sees, or the conductor's existing free, temporary hiring.

## Open questions for the owner

1. Should "never stop" also skip the always-asked approvals (install, start a
   project, switch an integration on, permanent hire)? Today it does not.
2. The conductor still hires temporarily and freely for a plan's roles, as
   chosen in ADR 0002. Should those hires be proposals too?
3. Which integrations, beyond the six reference servers, belong in the
   catalogue — and which agents should get them by default (today: nobody but
   the owner's own assistant)?
