# PN Scripts Assistant

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
- **Acts, with permission.** It can read files, list directories, search and
  read the web, and control a smart home. Anything that *changes* something
  stops and asks first, showing exactly what will happen.
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
| `open` | what you type may go to a third-party model |

**What the brain has learned about you never leaves, in any mode.** Not in
`open`, not with any setting. Conversations are typed deliberately; memory is
assembled from your disk without you composing it, so it is not ours to forward.
There is no configuration that changes this.

An unrecognised privacy value is treated as `private`, because the safe reading
of a typo is the strict one.

## Requirements

- [Ollama](https://ollama.com) with a chat model and `nomic-embed-text`
- For the native window: `libwebkit2gtk-4.1-dev` and `libgtk-3-dev` (Linux)
- For speech: any of `speech-dispatcher`, `espeak-ng`, or `piper`
- For listening: [whisper.cpp](https://github.com/ggml-org/whisper.cpp) with
  `whisper-cli` on your PATH and a model of 50MB or more

Run `pn-scripts-assistant-doctor` to see what is missing and install it. Nothing here is
required — the brain reports what it cannot do rather than refusing to start,
and controls for absent capabilities are not shown at all.

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

## Running it

One command, from anywhere (rebuilds if this tree is newer, then opens the window):

```bash
pn-scripts-assistant
```

The app menu entry is **PN Scripts Assistant**. If it is already running, that opens `http://127.0.0.1:8790` instead of starting a second copy.

Download the AppImage, make it executable, run it:

```bash
chmod +x PN-Scripts-Assistant-x86_64.AppImage
./PN-Scripts-Assistant-x86_64.AppImage
```

Or build from source:

```bash
CGO_ENABLED=1 go build -C clients -o ../dist/pn-scripts-assistant ./cmd/pn-scripts-assistant
```

`CGO_ENABLED=0` builds fine too — you get the brain without a native window,
reachable in any browser at `127.0.0.1:8790`.

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

**Voice can ask but cannot act.** Spoken turns are sent without tools: their
schemas cost more time than the answer does, and nobody talking to an assistant
is asking it to write a file. It also means a misheard sentence has nothing to
reach. Anything that changes something is typed, and still stops for approval.

If it does not hear you, `pn-scripts-assistant mic-test` reports the three numbers that make
that answerable: your room's noise floor, what therefore counted as speech, and
how loud you actually got.

## Speed, honestly

On a CPU with no GPU this is slow, and the shape of the slowness is worth
knowing. Measured on four cores with a 7B model: prompts are processed at about
ten tokens a second and answers generated at two and a half. Prompt size is
therefore very nearly the whole reply time.

A spoken turn takes around 40 seconds. A typed one takes longer, because tool
schemas are roughly 530 tokens sent with every message. Trimming what is sent
helps and has limits; the floor is the hardware. A GPU, or `open` mode with a
hosted model, are the only things that change the order of magnitude — and in
`open` mode what the brain has learned about you still never leaves.

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
```

## How it is built

Go, and a SQLite file. Vectors are stored as float32 blobs and similarity is
computed in process — exact, and fast enough at this size that an index
would be a dependency bought for nothing.

Nothing else has to be running for it to start: no database server, no
container runtime, no background stack. The program, one file of memory, and
whichever model you point it at. How it came to be built this way is in
[docs/ROADMAP.md](docs/ROADMAP.md).

## Licence

See [LICENSE](LICENSE).
