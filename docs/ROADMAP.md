# Roadmap

**Where this actually is, as of 23 September 2026.** The program is released as
an Ubuntu package: `0.1.0`, installable with `apt`, in the applications menu,
with a manual page and an audit behind it. What that release contains and what
it does not is in [the release notes](RELEASE-NOTES-0.1.0.md); what was
measured to get there is in [docs/audit/](audit/); how to install, configure
and build it is in [docs/guide/](guide/).

The rest of this file is the reasoning: which tools were chosen and why, which
choices were reversed, and what measurement changed. It is kept because the
arguments are still the arguments — not as a statement of what is done. The
"what comes next" below predates the release and is read as a list of
intentions rather than a plan with dates.

## Choosing tools

Switch approach when one is genuinely blocked; keep it when it works. Both halves
matter, and the second is the easier one to get wrong — churn feels like progress.

Switched, for reasons that were real:

| From | To | Because |
|---|---|---|
| NativePHP / Electron | Go | needs Laravel ≤12; a window is not worth a framework downgrade |
| `webview_go`, Wails | hand-written cgo | both pin webkit2gtk-4.0, which Ubuntu 24.04 does not ship |
| Chromium `--app=` | GTK + WebKitGTK | it was a browser in a costume, not an app |
| hand-written extension list | `spc dump-extensions` | the list rotted immediately, omitting ext-intl |

**Replaced: the Laravel core, by Go and SQLite.** This entry used to argue the
opposite — that the friction was in distribution rather than the application, and
that rewriting a working brain would cost weeks and buy nothing. The reasoning was
sound and the conclusion was still wrong, because it answered the wrong question.

What it missed: the cost was never PHP, it was the four processes around it. The
brain needed Docker running Postgres, pgvector and Redis alongside it, so it could
not start on a machine without a container stack — and "install Docker first" is
not an answer for a personal assistant. Every distribution failure in the table
above traces back to that. The static build existed only to escape it, and the
static build is what kept failing.

Go removes the question rather than answering it. One binary, one SQLite file, no
server, no daemon, no cgo. Vectors are float32 blobs and similarity is computed in
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
brain simply has access — one less daemon, one less token to protect, and one less
process that can be left running when it should not be.

**Found by the move: every memory held a container path.** All 79 facts recorded
paths under `/mnt/scan`, which is where the owner's disk appeared *inside* the
container. Outside it, those paths point nowhere, so the knowledge was accurate
and unusable at the same time. `brain rewrite-paths` repairs them and re-embeds
what it changed — rewriting the text alone would leave each vector describing the
old wording, and nothing would ever surface that mismatch.

Where other languages still earn their place: platform-native code for Windows
WebView2 and macOS WKWebView, and Python if PDF text extraction or OCR is ever
added.

## Where it stands

The Laravel application is gone; everything it did runs in Go. Memory and
recall, the Extractor/Validator/Curator pipeline, the project and document
scanners, the agent loop with its approval gate, filesystem and web tools, Home
Assistant, lesson review, and the memory map.

One binary, one SQLite file. 163,000 lines of Go, 44,000 of them tests, across
1,307 test functions and 71 packages.

## Capabilities today

*A selection — the assistant now carries far more than this, and says so
itself: ask it what it can do and the answer is built from what is actually
installed rather than from a list written here. These are the ones whose
guards are worth explaining.*

| Tool | Risk | Notes |
|---|---|---|
| `read_file`, `list_directory` | Safe | credential paths refused outright |
| `fetch_url` | Safe | every resolved address checked; redirects re-checked |
| `web_search` | Safe | Brave API if keyed, else DuckDuckGo HTML |
| `list_devices` | Safe | only when a smart home is configured |
| `write_file` | **Mutating** | approval required; overwrite says so |
| `run_command` | **Mutating** | argv array, no shell |
| `set_device` | **Mutating** | approval required; unlocking says so in capitals |

Web tools are not registered at all in `private` mode. A tool the model can see
is a tool it will try, and an assistant that keeps proposing something it may
never do is worse than one that simply cannot.

### What the guards are actually for

**Credential paths are refused, on read and on write.** Reading a file and
fetching a URL are both Safe, and reasonably so — neither changes anything.
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

**A Mutating tool is never executed by the loop.** It records the call, stops
the turn, and reports what it is waiting for. Decisions are final — the update
is conditional on the row still being pending, so a replayed request cannot turn
a denial into an approval or run something twice. Approval executes the *stored*
arguments, never anything sent alongside the approval, or the summary the owner
read would describe something that did not happen.

This matters more than it sounds. Measured against `qwen2.5-coder:7b` and
`llama3.2:3b`, small local models are unreliable at deciding when to use a tool
— `llama3.2:3b` called `write_file` in response to "Hello, how are you?". The
gate is what makes a badly behaved model annoying rather than dangerous.

## Done since this list was written

- **The native window**, on Linux. GTK3 and WebKitGTK 4.1, built with cgo, and
  the `.deb` declares the libraries so `apt` brings them in.
- **Voice**, both halves. It hears the room until you stop talking, answers,
  reads the answer aloud and listens again. The voice it answers in is
  chosen in the program from every one the machine can speak in.
- **An installer for every system.** A `.deb`, a macOS `.dmg`, a Windows
  installer, each built on the system it belongs to, and the package is
  byte-for-byte reproducible between two machines.

## Still to do

- **Windows and macOS windows.** WebView2 and WKWebView, each needing
  platform-native code — which is where another language genuinely earns its
  place. On both systems today it runs and serves its interface to the
  browser. A mobile client would talk to the same HTTP API.
- **Multi-drive expansion.** A drive registry in the data root, so the brain can
  grow across several disks instead of being rebuilt when one fills.
- **More ingestion.** Browser history and email are the obvious next sources.
  Email needs OAuth, which is not something to set up silently.
- **Hosted providers, tested against something real.** Every one of the twelve
  is tested against a fake. Nobody has watched one answer.
- **The tunnel and paired devices, across the internet.** Both work on one
  network. Neither has been used the way they are meant to be used.

## Two things measurement changed

**The model choice.** A benchmark that warmed each model with the same prompt it
then timed was measuring cache hits, not performance. Corrected, `llama3.2:3b`
is twice as fast as `qwen2.5-coder:7b` and equally accurate at recall — and
scores 2/5 on deciding when to use a tool, against 4/5. Speed was the wrong axis;
a fast wrong answer costs more than a slow right one.

**The self-description guard.** Measured against the twelve lessons actually
waiting in this brain, the ported guard caught one of six self-descriptions. It
checked "I am", "I can", "I will" and missed "I should" purely because that verb
was not enumerated. Enumerating verbs was the wrong shape: the extractor is asked
for one sentence about the owner, and a sentence about somebody else does not
begin with "I". Six of six now, with zero false positives across all 79 facts.
