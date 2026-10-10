# ADR 0007 — Everything allowed, and what it already knows how to do

**Status:** accepted · **Date:** 2026-10-10 · **Supersedes:** parts of ADR 0004
and ADR 0005, listed below

## The request

Its owner, in his words (translated): "make it able to do everything we have
done so far, in this and the other sessions. Also remove the restriction with
private, and everything should always be allowed there."

Two things, then. The limits this program put on itself — privacy defaulting to
private, the approvals asked whatever the setting, confinement to a project,
the coding agents' sandboxes — are lifted by default. And the work done for him
by hand across many sessions — games built and published, a website shipped, a
plugin taken through review, profiles brought into line, mail read — is written
down as playbooks the program ships with, with the tools to carry them out.

## What "private" was

`llm.Mode`, three values, enforced in code rather than in a prompt:

| mode | what it allowed |
|---|---|
| `private` (was the default) | the local model only, local embeddings, no web; hosted providers refused by `Mode.GuardProvider`; web tools and off-machine integrations hidden by `OffLimits` |
| `research` | the web, with the model kept local |
| `open` | hosted models, with the memory recalled for a turn |

Beside it sat a second privacy axis for project work
(`orchestrator.Policy.Privacy`, default `balanced`), a `ProfileToHosted` switch
(off) and a stack of limits that were refusals or always-asked questions rather
than privacy as such.

## Decision

1. **Defaults are the most open settings.** Privacy `open`; freedom
   `everything`; profile sent with a hosted turn; work privacy
   `cloud_preferred`. The strict values still exist and are one line away.
2. **One new switch, `BRAIN_CONFINE`, default `0`.** With freedom `everything`
   and confine off (`Brain.Unbounded`), the agent loop (`agent.Loop.Unbounded`)
   lifts every limit that was not its owner's own decision:

   | limit | where | now |
   |---|---|---|
   | always-asked approvals (install, model download, removal, sending mail, destructive commands, starting a project, a permanent hire, switching an integration on, off-machine integration calls) | `tools.Consenting`, `brain.askToApprove` | carried out, and recorded as approved |
   | protected files (keys, `.env`, mail folders) — question and last-line refusal | `protect.InArguments`, `tools.GuardSensitive` | read without asking |
   | project confinement — refusal and the Landlock sandbox | `agent.outside`, `agent.Confined`, `brain.Decide` | not applied |
   | a hire's tool list and never-list | `Brief.Only`, `Brief.Never` | every tool offered |
   | integration grants per agent and per project | `tools.MayUse` | every integration offered |
   | integration approval, and privacy refusing a remote server | `mcp.Gateway.Activate` | switching on approves it, recorded |
   | a task's hire that would reach further than the task | `team.Widens` | hired (`Wish.Free`) |
   | addresses on this machine and the local network | `FetchURL.GuardTarget`, `read_a_page` | fetched |
   | Claude Code's file-only tool list | `orchestrator.ClaudeCodeExecutor` | `--dangerously-skip-permissions`, every tool, the owner's own settings |
   | Codex's workspace-write sandbox | `orchestrator.CodexExecutor` | `--dangerously-bypass-approvals-and-sandbox` |
   | an agent writing outside the project stops the work | `orchestrator.run` | recorded as a warning; the work goes on |

   `BRAIN_CONFINE=1` puts every one of them back exactly as ADR 0004 and 0005
   built them; no code path was removed.
3. **What is kept, whatever the setting.** A tool its owner refused by name
   (`permits.Refuse`) stays refused. What he switched off (`BRAIN_TURNED_OFF`)
   stays off. Every call is still written down — the invocation, the step with
   what it was handed and what came back, the evidence, the MCP audit — and
   files it overwrites are still kept for undo. The audit costs nothing, and it
   is how he sees what happened.
4. **Existing installs switch over once.** `config.LoadFrom` rewrites an older
   `brain.conf` in place, line by line — only `BRAIN_PRIVACY`,
   `BRAIN_FREEDOM`, `BRAIN_CONFINE` and `BRAIN_PROFILE_TO_HOSTED`, keeping every
   other line, its comments, its secrets and its file mode, and never writing a
   value that came from the environment — and adds `BRAIN_EVERYTHING_ALLOWED=1`.
   `orchestrator.LoadPolicy` does the same for `resources/policy.json`
   (`everything_allowed`). After that, whatever its owner sets is left alone.
   Standing refusals in `permissions.json` are not touched: those are his.
5. **The interface says it.** The privacy select lists `open` first and marks
   it as the default; setup falls back to `open` and `everything`; the
   permissions panel shows every capability as allowed while everything is,
   and reports `everything_allowed` and `confine`.

### The outbound-host test

`internal/outside` lists every host written into the source, and its test fails
when one appears that is not listed. That was asked about: does "everything
allowed" mean it should go? It does not conflict, so it stays as it is. It never
blocked anything at run time — nothing consults the list when connecting — and
it never covered addresses its owner asks for (a page, a mail server, an
integration). It documents what the program contacts *on its own*, which is the
proof that there is no telemetry; a playbook naming itch.io or Game Jolt is an
instruction to the model, not an address in the program. Its comments and the
README now say plainly that it documents rather than blocks.

The one real run-time host check, `preflight`'s download door, stays: it is
what makes an install fetch the pinned file from the project that publishes it,
an integrity check like the checksum beside it, not a permission. The model
still cannot name an install URL at all.

## Playbooks: what it already knows how to do

The program already modelled "how a job is done here" as **skills**
(`internal/brain/skills`): Markdown files whose words decide when they are
offered (`tools.Cued`), read as a Safe tool, never able to act themselves.
Playbooks are the same thing shipped in the binary:

- `internal/brain/skills/playbooks/*.md`, embedded with `go:embed`; each has
  `name`, `version`, `when` and `tools` in its header and *Before you start*,
  *Steps*, *Verify* and *Never* sections.
- `skills.Builtin()` loads them; `skills.All()` merges them with the owner's,
  his winning on a name clash, so a playbook can be corrected without a
  release. `Brain.ReloadSkills` registers both.
- Reading one says its version, so the record of a task says which version of
  the procedure was followed.

Fourteen ship at version 1: `build_html5_games`, `publish_itch_io`,
`publish_game_jolt`, `build_products`, `ship_pnscripts_com`,
`wordpress_org_plugin_review`, `update_social_profiles`, `brand_naming`,
`sales_hold`, `github_cost_control`, `machine_throttling`, `update_ai_brain`,
`email_and_complaints`, `planning_loans_strategy`. They were extracted from the
workspace brain (profiles, lessons, `.skills`), the site's own `.ai` rules, the
social kit and the product-factory specification. They carry no secrets,
passwords, account numbers or personal data, and no path on anybody's machine;
a test checks for the shapes of those.

Every playbook keeps the outward habits that are about other people rather than
about this machine: its owner signs in himself (no password is ever typed), and
publishing, sending and paying wait for his yes in the conversation. Those are
instructions to the model, not gates in the code.

### The tools they need

| need | tool |
|---|---|
| shell, git, gh, php, node | `run_command` (as before; `bash -lc` for pipes; `do_in_background` past five minutes) |
| file system | `read_file`, `write_file`, `edit_file`, `list_directory`, `search_files` (as before) |
| a browser it can drive, signed in | **new** `drive_browser`: Chrome over the DevTools protocol — tabs, open, navigate, eval, text, screenshot, close — in a profile of its own under the program's machine folder, on loopback port 9333, started on first use. Chrome refuses remote debugging on its default profile, so its owner signs in once in that window. Written on the standard library: a WebSocket client is two hundred lines, not a fourth dependency. |
| email | `read_email`/`send_email` (IMAP/SMTP, as before), **new** `read_mailbox` for Thunderbird's local mbox folders (every account, no password, deleted messages skipped), and Gmail through `drive_browser` |
| MCP | the existing gateway. The Claude-in-Chrome extension's MCP is bound to Claude's own apps and is not reachable from here; `drive_browser` is the equivalent. |

## The first meeting is a conversation

Asked for in the same breath ("the cliché at first meeting should be gone, it
should be a real conversation"). The first meeting was a card over the window —
a heading, two fields and a Begin button — and then a composed greeting:
"Good evening.", a recital of what it can do, a count of what it knows. All of
it is gone from the first meeting (`brain/firstmeeting.go`):

- **No form.** The naming card is removed from the page; nothing waits on it.
- **No composed opener.** `Greet` adds no time-of-day line, no introduction
  and no knowledge count while `FirstMeeting()` holds (no settings yet, or no
  owner's name and no conversation). The opener is the model's own, written
  from facts with a brief that forbids formulas, offers of help, feature lists
  and exclamation marks and allows one question, what to call him. With no
  model to write it there is no fallback sentence: nothing is said until he
  speaks. A real fact — a drive filling up — is still said.
- **The model is told how to talk, not what to say.** While its owner's name is
  unknown the system prompt, written and spoken, carries a note: you have just
  met; react to what was said; ask one thing only when you need it; answer in
  the language they write in.
- **Setup is filled from the conversation.** Every message of a first meeting
  goes through `LearnFromWords`: "I'm Petar", "call me…", "казвам се…",
  "аз съм…" give the owner's name; "your name is…", "ще се казваш…" the
  assistant's; a message mostly in Cyrillic sets the language to Bulgarian
  when none is set. Narrow on purpose — "I'm tired" is not a name — and
  written to `brain.conf` the moment it is said.

## Costs, said plainly

- **Prompt injection is no longer stopped by a question.** A web page the
  assistant reads can tell it to read a key and post it somewhere; with nothing
  confined, nothing asks. The record shows it afterwards. `BRAIN_CONFINE=1` or
  privacy `private` is the answer on a machine where that matters.
- **Open privacy sends the profile and recalled memory** to the chosen hosted
  provider with every hosted turn.
- **The coding agents run with their own checks off.** A mistake by Claude Code
  or Codex can now reach outside the project; it is recorded as a warning
  rather than stopping the work.

## What this supersedes

- ADR 0004, "Always asked, whatever the setting" — now only with
  `BRAIN_CONFINE=1`. Open question 1 is answered: yes, "never stop" skips them.
- ADR 0004, "Project work is confined", "A hire is never unrestricted" for the
  tool list in force, and "MCP is an integration, not a bypass" for grants,
  approval and privacy hiding — all now only with `BRAIN_CONFINE=1`. The audit
  of every activation and call is unchanged.
- ADR 0005, decision 1 (work privacy default `balanced`) — now
  `cloud_preferred` — and decision 4 ("Claude Code and Codex never run with
  their bypass flags") — now they do, unless confined.
- The README's privacy default.

## Verified

- Unit tests: the loop runs an always-asked tool, a call outside the project and
  a tool off a hire's list without asking, records each as a step, and still
  refuses a refused tool (`agent/unbounded_test.go`); the agents' exact argv in
  both modes and the policy upgrade (`orchestrator/unbounded_test.go`); the
  settings upgrade keeps every other line and the file mode, never writes the
  environment down, and happens once (`config/config_test.go`); an integration
  is approved as it is switched on (`mcp`); the last-line guard steps aside only
  for a call that carries the flag (`tools`); fourteen playbooks load with
  versions and sections, carry nothing private, are cued by their words, and
  lose to the owner's own skill of the same name (`skills/playbooks_test.go`);
  the DevTools client against a real WebSocket endpoint and Thunderbird folders
  laid out as Thunderbird lays them out (`tools`).
- A real headless Chrome on this machine started by `drive_browser`, a page
  opened, a button clicked through `eval`, its text read and a PNG taken
  (`PN_LIVE_BROWSER=1`).
- A scratch instance of the CGO build (linked against WebKitGTK 4.1), with its
  own home, settings and data root and no model reachable:
  - an older `brain.conf` (`private`, `ask`, profile off) came up `open`,
    `everything`, `BRAIN_CONFINE=0`, profile on, with its other lines kept;
  - `/api/permissions` reported freedom `everything`, `everything_allowed`
    true, and all fourteen playbooks plus `drive_browser` and `read_mailbox`
    registered; the log said "loaded what it has been taught, skills=14";
  - a brand new brain greeted with no composed line (only "This drive is
    filling up.", a real fact on that machine), the page had no naming form,
    and one message — "Здрасти, казвам се Тестов. Ще те наричам Искра." —
    wrote `BRAIN_OWNER=Тестов`, `BRAIN_NAME=Искра`, `BRAIN_LANGUAGE=bg` and
    cleared `first_run`.
- **Not verified:** a model-written first opener and a whole first
  conversation with a real model — the scratch instance had none reachable, by
  design, so its answers are IMPLEMENTED — NOT VERIFIED. Publishing through
  `drive_browser` on the real sites was not attempted.
