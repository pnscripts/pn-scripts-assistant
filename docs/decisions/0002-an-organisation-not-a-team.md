# ADR 0002 — An organisation, not a team

**Status:** accepted · **Date:** 2026-09-13

## The real question

"Give the assistant hundreds of professions" sounds like a question about a
bigger list. It is not one. What existed was a team: six agents in a Go slice,
each of which *was* its job, its tool list and its model size at once. Making
that list longer makes every planning prompt longer, and the program had already
measured what that costs — a small model choosing between thirty-one tools
"is a harder problem than the model can reliably solve". The same is true of
people.

So the decision this records is not how many jobs there are. It is that four
things which were one thing become separate, and that **however large the
organisation grows, what any model is shown stays small.**

## The concepts, and where each one lives

The codebase already had a rule for where things go: a record of what happened
is a SQLite row; something a person edits is a file in the brain's folder. Every
concept below follows it.

| Concept | What it is | Where it lives |
|---|---|---|
| **Organisation** | everything below, as its owner arranges it | the files in `organisation/` |
| **Company, division, department, team** | a *unit*: a kind a file says it is, with a parent | one file per unit, `organisation/<name>.md` |
| **Job** | a definition — what a backend engineer is for, knows, produces, is judged on | `occupations` table: 427 shipped, thousands imported |
| **Position** (seat) | a responsibility inside this organisation: a job, a seniority, whom it reports to, what it must never do | inside its unit's file |
| **Agent** | who actually turns up: a name, a seat, a manner, a model policy, tools | `agents/<name>.md` |
| **Template** | an agent with no name yet | built in (16), or `templates/<name>.md` |
| **Capability** | something somebody is good at; not a job, not a tool | `capabilities`, linked to jobs in `job_capabilities` |
| **Tool** | something this program can do; declares Safe or Mutating | Go, in the tool registry |
| **Permission** | a standing yes, no, or yes-for-this-run, about everybody or one agent | `permissions.json` |
| **Risk** | how serious a piece of work is: low, medium, high, critical | on every task and step |
| **Model** | one component of an agent, chosen per step | resolved from the agent's policy and what is installed |
| **Knowledge** | what a job knows, shared by everyone who holds it | the taxonomy |
| **Memory** | what one agent remembers from its own work | `agent_memories` |
| **Task** | work that outlives the sentence that asked for it | `tasks`, `task_steps` |
| **Workflow** | the plan a task works through — made per job, never fixed | the steps of a task |
| **Orchestrator** | the conductor: plans, routes, runs, checks, hands on | `internal/brain/tasks` |
| **Delegation** | a step handed to a specialist as a task of its own | a child task, `parent_step_id` |
| **Escalation** | a failed step put to the agent's manager as the next step | a step with `escalated_from` |

Nothing about the hierarchy is in code. A unit names its parent; a seat names
the seat it reports to; a chart that loops or loses a parent is shown rather than
dropped. There is no `if department == engineering` anywhere.

## Job ≠ position ≠ agent

```
JOB        software.backend_engineer      a row, shared
POSITION   backend_engineer, Engineering  a seat: job, seniority, reports to lead_developer
AGENT      alex    — sits in that seat, remembers what alex did, thinks with the working model
AGENT      maria   — the same seat, remembers what maria did, pinned to a hosted model
```

An agent takes its capabilities from its job, adds its seat's and its own, and
may hold further jobs (`also:`) whose knowledge it gains and whose authority it
does not. Its tools come in three states — no limit, exactly these, or none —
because an empty list means "everything" elsewhere and a bookkeeper whose
capabilities map to no tool must not be handed all of them.

## How a request becomes work

```
request
  │
  ▼
planner ── shown a shortlist of ≤ 6, picked in code from the whole organisation
  │         (capability overlap, words, state; the record only breaks ties)
  ▼
plan ── steps: what, done when, kind, who, and whether it stands alone
  │
  ├─ a role nobody holds ──► hire one from the catalogue, for this job only
  ├─ each step weighed ──► low / medium / high / critical
  ▼
for each step (or group of steps that stand alone, side by side)
  │
  prepare ── who (validated), which service (through the privacy router),
  │          which model (the agent's policy), what tools (narrowed, never widened)
  │
  think ──── one turn of the agent loop, its model calls queued in a lane
  │          every call weighed; critical asks through a grant, except on never stop
  │
  settle ─── checked honestly in code
             ├─ met ──────► remembered (if verified); reviewed by someone else if high+
             ├─ unmet ────► retried once, told why
             └─ still ────► handed to a specialist (or one hired)
                            ► else sent up to the manager
                            ► else the plan reconsidered, once
                            ► else stopped, saying why
  ▼
account ── verified and claimed counted apart; what ran without asking; who was
           hired; temporary hires let go
```

## Decisions, and what each costs

**The planner is shown a shortlist, not the organisation.** The single change
that makes an organisation of any size possible. Tested with three hundred extra
agents. The cost: the right person must be among the six, so the shortlist is
wide in what it matches and the generalist is always last.

**Delegation, escalation and review are the conductor's, not tools.** A tool is
optional, and "it said it would ask the specialist and it did not" is the failure
this program has fought hardest. The cost: an agent cannot decide mid-turn that it
needs somebody; the code decides after the step fails its check.

**Authority is a narrowing, everywhere.** An agent's tools narrow the assistant's.
A child task may use only what the agent that handed it on could, narrowed again at
every level. A hire made by a task may use no more than whoever it works for. A
second job adds knowledge and never tools. Refusals come first at every level. So
a new agent, a chain of them, or a hire can never be a way round the gate.

**One gate, one router.** However many agents, there is one approval gate
(`MayI`) and one choke point for what leaves the machine (`Router.Provider`), both
wired once and shared by the conversation and the tasks.

**One switch for asking and privacy.** Its owner's choice: ask me first and nothing
leaves; do what I have allowed and the web is open; never stop, never refuse. On the
last, nothing asks — critical actions and protected files included — and every
high or critical action that ran is kept on its step and named in the task's
account, which is what makes that choice reviewable.

**Lanes, not a number.** This machine's model is one lane, one call at a time,
because two calls on a processor take longer than twice as long. Hosted services
are a pool sized by the machine, never fewer than two. Held per call, not per step.
The program runs on more than one machine, so the answer comes from the machine.

**Hiring is free, temporary, and reported.** Its owner's choice. Three hires per job
at most; each written on the task as it happens, named at the end, and let go when
the job ends. Asked for in a sentence, hiring is permanent and seated, and refuses
to hire a copy of somebody already there.

**Jobs come from real classifications, by a button.** 427 written by hand ship.
ESCO (with the occupation–skill links and every language's names) and O*NET (tasks,
knowledge, other titles, hot technologies) import from a downloaded folder or zip —
never the network — enriching what is there before adding, with what shipped or was
written by hand always winning.

**Knowledge is the job's, memory is the agent's.** The brief carries what the job is
for, from the taxonomy, and up to three things this agent remembers that bear on the
step — lessons first. Memory is its own table, not the owner's facts, which are what
the assistant knows about the person it works for.

**The record is a tie-break.** Verified against claimed, time, hand-ons, escalations
and reviews found wrong, per agent. It decides between two equally suited people once
both have done enough for a rate to mean something, and never outvotes suitability.

## Memory, as the specification layers it

| Layer | Here |
|---|---|
| Short-term context | the conversation, or a step's brief |
| Working memory | what earlier steps of this task established — verified only |
| Episodic memory | `agent_memories`: what each agent found and learned |
| Semantic memory | the owner's facts, recalled by embedding |
| Procedural memory | skills its owner has taught |
| Project memory | a goal's tasks and their accounts |
| Organisational knowledge | the owner's profile, under the privacy switch |
| User preferences | the profile and the facts |

## Not done, and why

- **User satisfaction** is not measured. There is no signal for it that does not
  mean asking somebody to rate their own assistant.
- **Agent-initiated delegation mid-turn** is not offered, deliberately; see above.
- **The importers have been run against release-shaped samples, not the full ESCO
  and O*NET releases**, which were not downloaded on the machine this was built on.
  Both read by column name and refuse a file missing a column by naming it.

## Consequences

- Every concept is a file somebody can open or a row somebody can query; the
  Organisation and Tasks views read them, and nothing about the organisation lives
  only in memory.
- Adding a profession is a row; a department, a file; an agent, a file; a template,
  a file; a tool, a Go type declaring its risk. None needs the core changed.
- Steps now have three parts (prepare, think, settle), and tasks carry a risk, a
  parent, a tool limit and a hire list. Migrations 8 to 15, all additive: a brain
  from before this reads as it did.
