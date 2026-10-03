# PN Scripts Assistant

[![CI](https://github.com/pnscripts/pn-scripts-assistant/actions/workflows/ci.yml/badge.svg)](https://github.com/pnscripts/pn-scripts-assistant/actions/workflows/ci.yml)
[![Licence: MIT](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8.svg)](clients/go.mod)

PN Scripts Assistant is a [PN Scripts](https://pnscripts.com) product ([product page](https://pnscripts.com/products/pn-scripts-assistant)).

A personal, self-learning AI assistant that runs entirely on your own machine.

One binary. No database server, no container runtime, no cloud account. It
remembers what it learns about you, and what it learns stays here.

## What it does

- **Answers with memory.** It recalls what it already knows about your projects
  and documents, and uses that to answer — so "where does that project live"
  gets a real path rather than a guess.
- **Learns on its own.** It scans your code and documents, and proposes things
  worth remembering after conversations. Anything it inferred waits for you to
  approve; anything it can verify against the disk promotes itself.
- **Acts.** It can read files, list directories, run commands, search and read
  the web, and control a smart home. Out of the box it does what you ask
  without stopping to confirm each step — and everything it does is written
  down, and a file it changed can be put back. If you would rather be asked,
  the Permissions panel has three settings: ask every time, ask about what you
  have not already allowed, or act freely.
- **Draws what it knows.** The interface renders a live map of memory, built
  from real embedding similarity — not decoration.
- **Listens and speaks.** Conversation mode hears you until you stop talking,
  answers, reads the answer aloud, and listens again. Both halves are local.
- **Hires and equips experts.** "Hire a game-development expert", "hire an
  electrician to help me plan this installation": it proposes who, what they
  may and may not touch, what the machine needs (with size, source, licence and
  cost), and who else is needed — and hires nobody until you say so. Advisory
  and regulated work never touches anything, and says which licensed person
  takes over.
- **Starts and builds projects.** "Make me a Tetris game" asks where, chooses
  the engine from what you asked and what this machine can really do, and shows
  the whole plan. Once you approve, the work is confined to that folder — by the
  kernel, not only by checking arguments — its settings live in `.pn-assistant/`
  beside the code, and it is finished only on fresh evidence: the latest check
  passed after the last change, a run that did not fail, a picture that shows
  something.
- **Chooses how work is done.** Who writes the code — Claude Code or Codex on
  your subscription, or a model on this machine — is decided per piece of work,
  subscriptions first, switching before a limit is hit and falling back when a
  service is signed out or out of allowance, with every decision and switch
  recorded. How much of a subscription is left is shown only when its service
  says. Checking, running and building are this program's own work with the
  engine, never a model's word. See
  [ADR 0005](docs/decisions/0005-choose-how-work-is-done.md).
- **Integrations, gated.** MCP servers, approved one by one, granted per agent
  and per project, confined to their own folders, with every call recorded. See
  [ADR 0004](docs/decisions/0004-hire-equip-and-confine.md).

## Privacy

This is the part that is enforced in code rather than promised in a prompt.

| mode | what leaves the machine |
|---|---|
| `private` (default) | **nothing** — local model, local embeddings, no web |
| `research` | search queries only; conversations and memory stay here |
| `open` | your messages and the memory recalled for that turn go to the hosted provider you chose |

**In `open` mode, your messages and the memory recalled for that turn are sent
to the hosted provider you chose; the memory database itself stays on your
machine.** In `private` and `research` mode no memory leaves at all. Recall is
what a turn needs, not your whole memory, but it is assembled from your disk
rather than typed by you, so choose `open` knowing that.

An unrecognised privacy value is treated as `private`, because the safe reading
of a typo is the strict one.

**Everywhere it can connect to is written down**, with a reason beside each
entry, in [`clients/internal/outside/outside.go`](clients/internal/outside/outside.go)
— and a test reads the whole program and fails if an address turns up that is
not on the list. There is no telemetry and nothing reporting to this project.
Every host on it is something you asked it to install, a documentation site, a
web search, or a model provider you gave your own key to.

## Requirements

- [Ollama](https://ollama.com) with a chat model and `nomic-embed-text`
- For the native window: `libwebkit2gtk-4.1-dev` and `libgtk-3-dev` (Linux)
- For speech: any of `speech-dispatcher`, `espeak-ng`, or `piper`
- For listening: [whisper.cpp](https://github.com/ggml-org/whisper.cpp) with
  `whisper-cli` on your PATH and a model of 50MB or more

None of it is required. The assistant reports what it cannot do rather than
refusing to start, and controls for capabilities that are missing are not shown
at all. The first run checks for all of this and offers to install what it can
— the package does not carry a separate checker, because the program is one.

### Two Ollama settings worth checking

Ollama's own defaults work against this, and both need root to change:

```
OLLAMA_HOST=127.0.0.1:11434   # not 0.0.0.0 — see Privacy below
OLLAMA_MAX_LOADED_MODELS=2    # or every reply reloads the chat model
```

The second is not a preference. Recall embeds your question before every reply,
and with only one model permitted in memory, loading the embedder evicts the
chat model — which is then read back from disk to answer. Measured on a 7B
model: 6.8 seconds to reply when it is resident, 15.8 after an embedding
evicted it.

The first matters more. This brain binds to loopback and refuses to do
otherwise, but it sends every conversation and every recalled memory to Ollama.
An Ollama on `0.0.0.0` is unauthenticated and reachable by anything on your
network, which undoes the careful part.

## Installing it

On Ubuntu it installs as a package and draws its own window. On macOS and
Windows it installs the way those systems expect and shows its interface in
your browser — the native window is Linux-only so far, and that is the only
difference between them.

| System | Download | |
|---|---|---|
| Ubuntu / Debian | `pn-scripts-assistant_<version>_amd64.deb` | `sudo apt install ./…deb` |
| macOS 11+ | `…-macos-arm64.dmg` (or `amd64` for Intel) | drag it to Applications |
| Windows 10+ | `…-windows-setup.exe` | run it; no administrator password |

Nothing is signed — there is no Apple developer account and no Windows
certificate behind this — so macOS and Windows both refuse the first open and
both take one extra click to pass. [installing.md](docs/guide/installing.md)
says exactly which click, for each.

### On Ubuntu

Ubuntu 24.04 or newer, on a 64-bit machine. Download
`pn-scripts-assistant_<version>_amd64.deb` from the
[releases](https://github.com/pnscripts/pn-scripts-assistant/releases) and:

```bash
sudo apt install ./pn-scripts-assistant_*.deb
```

`apt` pulls in the libraries the window needs — the package says which ones,
having been asked of the binary rather than remembered — and puts **PN Scripts
Assistant** in the applications menu with its icon.

To check what you downloaded is what was built:

```bash
sha256sum -c SHA256SUMS
```

### Checking that a download is genuinely ours

A checksum proves a file matches a published number. It does not prove the
number came from here — so every released file also carries a **provenance
attestation**: a statement, signed by GitHub with a key nobody here holds,
binding that file to this repository, the workflow that built it and the commit
it was built from. Check any download with nothing installed but the GitHub
CLI:

```bash
gh attestation verify pn-scripts-assistant_*_amd64.deb --repo pnscripts/pn-scripts-assistant
```

Nothing is signed with a code-signing certificate — an Apple one and a Windows
one cost money every year and identify a company, and this project has neither.
That is why the attestation is here: it is free, it needs no secret, and it is
the one form of "this really is the file that workflow built" that can honestly
be offered. [SECURITY.md](SECURITY.md) says the same and what else is not there
yet.

### Building the packages yourself

```bash
make release
```

That formats, vets, runs every test, builds the `.deb`, checks it from the
outside with `lintian`, and writes `SHA256SUMS` beside it in `build/packages`.
It needs `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`, `dpkg-dev`, `fakeroot` and
(for the check) `lintian`.

Or just the binary, without a package:

```bash
CGO_ENABLED=1 go build -C clients -trimpath -o ../build/pn-scripts-assistant ./cmd/pn-scripts-assistant
```

`CGO_ENABLED=0` builds too — you get the assistant without a native window,
reachable in any browser at `127.0.0.1:8790`.

## Running it

Press the Ubuntu key, type the first few letters of its name, and press Return.
The first run opens setup: it finds Ollama, offers to install a model, and asks
where the brain should live.

From a terminal, the same thing:

```bash
pn-scripts-assistant
```

If it is already running, that brings the existing window forward rather than
starting a second copy — one assistant per brain, and the second says where the
first is answering. `pn-scripts-assistant serve` runs it without a window, for a
machine with no desktop; the address is printed when it starts.

A few things worth knowing:

| | |
|---|---|
| Which copy is this | `pn-scripts-assistant version` |
| Everything it can do from a terminal | `man pn-scripts-assistant` |
| Where the data is, and how much | `pn-scripts-assistant status` |
| Why it did not start | `~/.local/state/pn-scripts-assistant/logs/pn-scripts-assistant.log` |
| Somewhere else | `BRAIN_ADDR=127.0.0.1:9000 pn-scripts-assistant` |

Everything else is in the window. The terminal is for people who prefer one.

## Updating and removing it

A newer package installs over the old one and keeps everything learned:

```bash
sudo apt install ./pn-scripts-assistant_<newer>_amd64.deb
```

Removing the program leaves the brain alone, deliberately — years of somebody's
memory should not go with an `apt remove`:

```bash
sudo apt remove pn-scripts-assistant          # the program
rm -rf ~/.local/share/pn-scripts-assistant    # and the memory, if you mean it
rm -rf ~/.config/pn-scripts-assistant ~/.local/state/pn-scripts-assistant
```

If the brain was moved to another drive, `pn-scripts-assistant status` says
where it is before you remove anything.

## Where the data lives

The brain finds itself. It looks for a `.brain-root.json` marker across mounted
drives, so the data can live on an external disk, be unplugged, reattached at a
different path, or moved to another computer — nothing is pinned to a machine.
With no marker anywhere, it creates one under
`~/.local/share/pn-scripts-assistant/data`. A brain made before the program was
renamed from PN Brain stays where it was, in `~/.local/share/pn-brain`, and is
still found there.

Settings live beside the data in `brain.conf`, not beside the program, because
they describe *this brain* rather than this installation.

## Talking to it

Type in the box, or use your voice. Three controls appear in the Engine panel,
and only when the machine can actually do them:

- **Microphone** — which input to listen through. Worth setting: the system
  default is often an empty analog jack rather than the microphone you own.
- **Read replies aloud** — answers are spoken as well as shown.
- **Conversation mode** — listens until you stop talking, answers, speaks, and
  listens again, until you switch it off.

Silence detection calibrates to your room at the start of every turn, because a
fixed threshold is either deaf in a quiet room or triggered by a fan in a noisy
one.

**Spoken turns can use the same tools as typed ones, under the same permission
setting.** What was said decides whether tools are offered, not how it arrived:
small talk is answered without them either way. Under the default *act freely*
setting a spoken request can change something without stopping to ask, and is
recorded like any other; set Permissions to ask if you would rather confirm.

If it does not hear you, `pn-scripts-assistant mic-test` reports the three numbers that make
that answerable: your room's noise floor, what therefore counted as speech, and
how loud you actually got.

## Speed, honestly

On a CPU with no GPU this is slow, and the shape of the slowness is worth
knowing. Measured on four cores with a 7B model: prompts are processed at about
ten tokens a second and answers generated at two and a half. Prompt size is
therefore very nearly the whole reply time.

A turn answered without tools, which is most small talk and much of what is
said aloud, takes around 40 seconds. A turn that needs tools takes longer,
spoken or typed, because tool schemas add roughly 530 tokens to the prompt. Trimming what is sent
helps and has limits; the floor is the hardware. A GPU, or `open` mode with a
hosted model, are the only things that change the order of magnitude. In
`open` mode your messages and the memory recalled for that turn are sent to the
hosted provider you chose; the memory database itself stays on your machine.

## Commands

The interface does everything; these exist for people who prefer a terminal.

```
pn-scripts-assistant                    run the app: serve, and open the window
pn-scripts-assistant serve              serve only, without a window
pn-scripts-assistant status             where the data lives and what is in it
pn-scripts-assistant ingest <dir>...    learn about the projects and documents in a folder
pn-scripts-assistant ingest --browser   learn which websites you use (domains only, never URLs)
pn-scripts-assistant promote            turn validated lessons into durable knowledge
pn-scripts-assistant tidy               clear what is not worth remembering out of the queue
pn-scripts-assistant mic-test           listen once and report what the microphone heard
pn-scripts-assistant drives             where the brain could live, and how much room is left
pn-scripts-assistant move <dir>         move the brain to another drive, verifying every byte
pn-scripts-assistant rewrite-paths      repair stored paths after a move, then re-embed
pn-scripts-assistant copies             where copies of the brain are kept, and copy now
pn-scripts-assistant places             the drives and folders it learns from  (--read)
pn-scripts-assistant menu               put it in the applications menu  (--remove to take it out)
pn-scripts-assistant setup              choose the drive, the model and the keys again
pn-scripts-assistant version            which copy of the program this is
```

`man pn-scripts-assistant` has all of them, with the files and the environment
variables.

## How it is built

Go, and a SQLite file. Vectors are stored as float32 blobs and similarity is
computed in process — exact, and fast enough at this size that an index
would be a dependency bought for nothing.

Nothing else has to be running for it to start: no database server, no
container runtime, no background stack. The program, one file of memory, and
whichever model you point it at. How it came to be built this way is in
[docs/ROADMAP.md](docs/ROADMAP.md).

## The rest of it

| | |
|---|---|
| [Installing, updating, removing](docs/guide/installing.md) | the package and what it puts where |
| [Setting it up](docs/guide/configuring.md) | the first run, privacy, environment |
| [Models](docs/guide/how-it-thinks.md) | which one answers, and why the second question is faster |
| [When something is wrong](docs/guide/troubleshooting.md) | the log first |
| [Working on it](docs/guide/developing.md) | building, tests, expectations |
| [Making a release](docs/guide/releasing.md) | `make release` |
| [What changed](CHANGELOG.md) | between versions, in the terms you would notice |
| [Decisions](docs/decisions/) | why things are the way they are |
| [Audits](docs/audit/) | what was measured, and when |
| [Security](SECURITY.md) | the trust model, how to report a fault, and what is honestly not there yet |
| [Contributing](CONTRIBUTING.md) | what a change is expected to carry |
| [Code of conduct](CODE_OF_CONDUCT.md) | short, because a long one would not be read |

## How this project checks itself

Stated here because "trust me" is not a thing a program that reads your files
gets to say.

- **Every push and every pull request** runs formatting, `go vet`, the whole
  test suite under the race detector, a build for each system it claims to
  build for, `govulncheck`, and the `.deb` put through `lintian` —
  [ci.yml](.github/workflows/ci.yml).
- **Every released file** is built by a workflow on the system it is for, is
  published with its SHA-256, and carries a signed provenance attestation
  anybody can check.
- **The package is reproducible.** The same commit built on a GitHub runner and
  on the maintainer's desktop produces a byte-identical `.deb`, so you can
  check out that commit, run `make package`, and compare — rather than trusting
  a published number.
- **Every action in CI is pinned to a commit**, not to a tag somebody could
  move, and only the job that publishes a release can write anything.
- **`main` requires those checks to have passed**, and refuses force-pushes and
  deletion.
- **The audits in [docs/audit/](docs/audit/)** use a fixed vocabulary —
  VERIFIED, IMPLEMENTED — NOT VERIFIED, BROKEN — so what was measured is
  distinguishable from what was merely written. They are the maintainer's own;
  nobody independent has audited this, and that is said rather than left to be
  assumed.
- **Three dependencies**, each with a reason written down, and no new one
  without another.

## Licence

MIT — see [LICENSE](LICENSE). Yours to read, run, fork and sell.
