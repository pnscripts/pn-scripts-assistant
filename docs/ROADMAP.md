# Roadmap

**Where this actually is, as of 8 November 2026.** The program is released
for two systems: an Ubuntu/Debian package, installable with `apt`, in the
applications menu, with a manual page; and a Windows installer and zip, with a
window of its own. What each release contains is in the
[changelog](../CHANGELOG.md); what it does not do yet is in
[known limits](guide/known-limits.md); what was measured to get there is in
[docs/audit/](audit/); how to install, configure and build it is in
[docs/guide/](guide/).

The rest of this file is the reasoning: which tools were chosen and why, which
choices were reversed, and what measurement changed. It is kept because the
arguments are still the arguments, not as a statement of what is done. The
"what comes next" below predates the release and is read as a list of
intentions rather than a plan with dates.

## Choosing tools

Switch approach when one is genuinely blocked; keep it when it works. Both halves
matter, and the second is the easier one to get wrong: churn feels like progress.

Switched, for reasons that were real:

| From | To | Because |
|---|---|---|
| NativePHP / Electron | Go | needs Laravel ≤12; a window is not worth a framework downgrade |
| `webview_go`, Wails | hand-written cgo | both pin webkit2gtk-4.0, which Ubuntu 24.04 does not ship |
| Chromium `--app=` | GTK + WebKitGTK | it was a browser in a costume, not an app |
| hand-written extension list | `spc dump-extensions` | the list rotted immediately, omitting ext-intl |

**Replaced: the Laravel core, by Go and SQLite.** This entry used to argue the
opposite: that the friction was in distribution rather than the application, and
that rewriting a working brain would cost weeks and buy nothing. The reasoning was
sound and the conclusion was still wrong, because it answered the wrong question.

What it missed: the cost was never PHP, it was the four processes around it. The
brain needed Docker running Postgres, pgvector and Redis alongside it, so it could
not start on a machine without a container stack, and "install Docker first" is
not an answer for a personal assistant. Every distribution failure in the table
above traces back to that. The static build existed only to escape it, and the
static build is what kept failing.

Go removes the question rather than answering it. One binary, one SQLite file, no
server, no daemon. (The one cgo part since is the Linux window, GTK and
WebKitGTK; the Windows window and everything else are plain Go.) Vectors are float32 blobs and similarity is computed in
process, which at this size is exact and cheaper than the network hop it replaced.

Verified equivalent on the real data before the old system was removed: 79 facts,
145 lessons, 26 conversations, 79 messages and 15 tool calls carried across, and
the memory map rebuilt to the same 79 nodes and 177 links with the same strongest
pair, 6↔8 at 0.949.

The lesson worth keeping is not "rewrite sooner". It is that "keep what works" has
to be checked against what the thing is *for*. The Laravel core worked; the product
it added up to could not be downloaded and run, which was the entire point.

**Dropped along the way: the host agent.** It existed only because Laravel ran
inside a container and could not see the real filesystem. Running natively, the
brain simply has access: one less daemon, one less token to protect, and one less
process that can be left running when it should not be.

**Found by the move: every memory held a container path.** All 79 facts recorded
paths under `/mnt/scan`, which is where the owner's disk appeared *inside* the
container. Outside it, those paths point nowhere, so the knowledge was accurate
and unusable at the same time. `brain rewrite-paths` repairs them and re-embeds
what it changed, because rewriting the text alone would leave each vector describing the
old wording, and nothing would ever surface that mismatch.

Where other languages still earn their place: none so far. The Windows window
reaches WebView2 through its COM interfaces from Go, PDF text and forms are
read and written by Go code written from the format, and scans are read by
Tesseract or a local model that can see, called as programs.

## Where it stands

The Laravel application is gone; everything it did runs in Go. Memory and
recall, the Extractor/Validator/Curator pipeline, the project and document
scanners, the agent loop with its approval gate, filesystem and web tools, Home
Assistant, lesson review, and the memory map.

One binary, one SQLite file. 217,000 lines of Go, 58,000 of them tests, across
1,653 test functions and 91 packages (counted 31 October 2026).

## Capabilities today

*A selection: the assistant now carries far more than this, and says so
itself: ask it what it can do and the answer is built from what is actually
installed rather than from a list written here. These are the ones whose
guards are worth explaining.*

| Tool | Risk | Notes |
|---|---|---|
| `read_file`, `list_directory` | Safe | keys, passwords and other protected paths are asked about first, goal mode included |
| `fetch_url` | Safe | every resolved address checked, redirects re-checked, and the address actually dialled checked again |
| `web_search` | Safe | Brave API if keyed, else DuckDuckGo HTML |
| `list_devices` | Safe | only when a smart home is configured |
| `write_file` | **Mutating** | approval as the Permissions setting says; overwrite says so, and the old file is kept for undo |
| `run_command` | **Mutating** | argv array, no shell; a shell handed a program as text (`bash -c`, `cmd /c`, PowerShell) is judged by what it runs |
| `set_device` | **Mutating** | approval as the Permissions setting says; unlocking says so in capitals |

Whether a Mutating tool stops for approval depends on the Permissions setting.
The default is *act freely* (`everything`): it runs without asking. *Ask about
what I have not allowed* (`granted`) runs what has been granted and asks about
the rest, and always asks about anything rated critical. *Ask every time*
(`ask`) stops before every one. In every setting each tool call is recorded,
and a file the brain overwrites is kept so it can be put back: the last 60
copies, for up to 14 days.

Web tools are not registered at all in `private` mode. A tool the model can see
is a tool it will try, and an assistant that keeps proposing something it may
never do is worse than one that simply cannot.

### What the guards are actually for

**Credential paths are refused, on read and on write.** Reading a file and
fetching a URL are both Safe, and reasonably so: neither changes anything.
Together they are an exfiltration route: a page can contain text telling the
model to read a credentials file and fetch a URL with the contents attached, and
because neither step needs approval, nobody sees it happen.

**`fetch_url` checks every resolved address, not the hostname.** A public name
can point at a private one; that is the whole trick behind server-side request
forgery. Here it would turn "fetch a web page" into "read whatever is listening
on this machine", including this brain's own unauthenticated API. Redirects are
re-checked on every hop, since passing the first check and then redirecting is
the standard way around a guard like this.

**Anything fetched is untrusted.** A model reading a page cannot reliably tell
page text from instructions. That remains a real risk for any agent with tools,
and no amount of framing makes it a guarantee.

**`run_command` takes an argv array and there is no shell.** A semicolon is an
argument, not a second command. That is also what makes the approval summary
trustworthy: what is shown is exactly what will run, with no expansion happening
afterwards.

**When a Mutating tool needs approval, the loop does not execute it.** It
records the call, stops the turn, and reports what it is waiting for. Under the
default *act freely* setting no approval is needed and the call runs, still
recorded. Decisions are final: the update
is conditional on the row still being pending, so a replayed request cannot turn
a denial into an approval or run something twice. Approval executes the *stored*
arguments, never anything sent alongside the approval, or the summary the owner
read would describe something that did not happen.

This matters more than it sounds. Measured against `qwen2.5-coder:7b` and
`llama3.2:3b`, small local models are unreliable at deciding when to use a tool:
`llama3.2:3b` called `write_file` in response to "Hello, how are you?". The
gate is what makes a badly behaved model annoying rather than dangerous.

## Done since this list was written

- **The native window**, on Linux. GTK3 and WebKitGTK 4.1, built with cgo, and
  the `.deb` declares the libraries so `apt` brings them in.
- **The native window on Windows** (October 2026). A Win32 window holding
  Microsoft Edge WebView2, reached through syscall and the COM interfaces,
  still no cgo in the Windows build. The loader ships beside the program,
  taken from Microsoft's NuGet package at a pinned version and checked against
  its SHA-256; where the runtime is missing it falls back to the browser. A
  Windows runner opens a real window on every push (`window-check`).
- **Voice**, both halves. It hears the room until you stop talking, answers,
  reads the answer aloud and listens again. The voice it answers in is
  chosen in the program from every one the machine can speak in.
- **An installer for each released system.** A `.deb` and a Windows
  installer, each built on the system it belongs to, and the package is
  byte-for-byte reproducible between two machines. (A macOS bundle was
  published until 0.9; from 1.0 the releases are Linux and Windows only.)
- **Its own icon on Windows**, on the window, the shortcuts, the installer and
  the uninstall entry.
- **A security review of everything added since 0.4**, by a reviewer who had
  not written it, with every finding fixed (October 2026).
- **Updating keeps everything.** `scripts/upgrade-check.sh` builds an earlier
  release, fills a sealed scratch brain through it, opens it with the current
  code and reads every record back.
- **Moving the Windows window between screens of different scaling** (0.11),
  proved on a Windows runner with the messages Windows sends.
- **Light on the machine** (0.11). Untouched for a minute, the window rests at
  about 2 % of one core instead of more than a whole one; minimised, about
  0.5 %. `scripts/load-check.sh` measures it and CI holds it there.
- **A stranger's install, proved on the published files** (0.11): a clean
  Ubuntu runner installs the released `.deb` with `apt`, a clean Windows runner
  the released installer, after every release.

## Still to do

- **macOS.** Not planned: releases are Linux and Windows only. The source
  still compiles for macOS and serves its interface to the browser there,
  untested. A mobile client would talk to the same HTTP API.
- **Multi-drive expansion.** A drive registry in the data root, so the brain can
  grow across several disks instead of being rebuilt when one fills.
- **More ingestion.** Browser history is the obvious next source. Mail is read
  with an app password over IMAP; OAuth sign-in is not offered.
- **Hosted providers, tested against something real.** Every one of the twelve
  is tested against a fake. Nobody has watched one answer.
- **The tunnel and paired devices, across the internet.** Both work on one
  network. Neither has been used the way they are meant to be used.

## After 1.0: working the way its user works

General abilities, for whoever uses the assistant, following their
requirements and goals; nothing in them belongs to one person or one
company. Three waves were built on 8 November 2026 (unreleased at the time
of writing); the rest is specified below and not started.

**Built:**

- **Standing rules learned from conversation.** "Always…", "never…", "from
  now on…", "ask me before…" and complaints that mean "stop doing that" are
  proposed as rules in the user's own words, with the message they came from,
  and kept only with a yes (package `rules`, the Rules page). Kept rules are
  read into every turn and every step of a goal, and checked before and after
  every outward or lasting action (sending, posting, commits, files, paying,
  deleting): a precise "never" stops the action and names itself, whatever
  else is allowed.
- **The last look before anything goes out** (package `polish`): what the
  reader already sees, filler, repetition, sure slips, placeholders, wrong
  weekdays, applied to every email before the user is asked, and on the
  Writing page; a sent message is shown again with where it went.
- **Other coding agents as tools.** Claude Code, Codex and Cursor's agent,
  signed in by the user, asked for one task in a chosen folder: read, folder
  or full (always asked); what changed is read from the folder; the user's
  rules travel with the task; the allowance's pace is shown before each task.
  MCP servers the user adds were already tools under the same approval; the
  rules now apply to them too.
- **Looking after the machine**: leftover processes (listed, stopped only on
  a yes, never the user's programs), how the last session ended (a power cut
  is noticed, interrupted work is parked with the reason), and "can I
  restart?" (a waiting kernel and its DKMS drivers, installs, running work,
  unpushed work). Linux first.

**Next, specified:**

- **Work moved to quiet hours.** A user-set window for heavy work (goals,
  coding agents, learning), with each paid allowance paced to its renewal:
  work that would run an allowance out early waits for the window, says so,
  and can be started anyway with one press.
- **Replies on the user's own channels.** Comments on pages the user connects
  (through the official API where there is one, else a browser the user signs
  in), each checked for scams and answered from facts the assistant can show;
  a policy per channel and kind of message: answer alone and show afterwards
  (the exact text, where and to whom, translated if needed), or show the
  exact text first and wait. Personal messages, prices, offers and new
  promises always wait.
- **Promises kept.** Anything said publicly that commits to doing something
  becomes a task with the words quoted, and a follow-up reply when it is done.
- **An approval policy per channel and kind of action**, beyond today's per
  tool permissions: what it may do alone, what it does and then shows, what
  waits for an explicit yes on the exact words.

## Two things measurement changed

**The model choice.** A benchmark that warmed each model with the same prompt it
then timed was measuring cache hits, not performance. Corrected, `llama3.2:3b`
is twice as fast as `qwen2.5-coder:7b` and equally accurate at recall, and
scores 2/5 on deciding when to use a tool, against 4/5. Speed was the wrong axis;
a fast wrong answer costs more than a slow right one.

**The self-description guard.** Measured against the twelve lessons actually
waiting in this brain, the ported guard caught one of six self-descriptions. It
checked "I am", "I can", "I will" and missed "I should" purely because that verb
was not enumerated. Enumerating verbs was the wrong shape: the extractor is asked
for one sentence about the owner, and a sentence about somebody else does not
begin with "I". Six of six now, with zero false positives across all 79 facts.
