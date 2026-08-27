# pn-brain

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

Run `pn-brain-doctor` to see what is missing and install it. Nothing here is
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

From this repo, one command (rebuilds if the source is newer, then opens the window):

```bash
./start
```

After the first run, the same thing is on your PATH and in the app menu as **PN Brain**:

```bash
pn-brain
# or
brain
```

If it is already running, that opens `http://127.0.0.1:8790` instead of starting a second copy.

Download the AppImage, make it executable, run it:

```bash
chmod +x PN-Brain-x86_64.AppImage
./PN-Brain-x86_64.AppImage
```

Or build from source:

```bash
CGO_ENABLED=1 go build -C clients -o ../dist/pn-brain ./cmd/brain
```

`CGO_ENABLED=0` builds fine too — you get the brain without a native window,
reachable in any browser at `127.0.0.1:8790`.

## Where the data lives

The brain finds itself. It looks for a `.brain-root.json` marker across mounted
drives, so the data can live on an external disk, be unplugged, reattached at a
different path, or moved to another computer — nothing is pinned to a machine.
With no marker anywhere, it creates one under `~/.local/share/pn-brain`.

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

If it does not hear you, `pn-brain mic-test` reports the three numbers that make
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
pn-brain                    run the app: serve, and open the window
pn-brain serve              serve only, without a window
pn-brain status             where the data lives and what is in it
pn-brain ingest <dir>...    learn about the projects and documents in a folder
pn-brain ingest --browser   learn which websites you use (domains only, never URLs)
pn-brain promote            turn validated lessons into durable knowledge
pn-brain tidy               clear what is not worth remembering out of the queue
pn-brain mic-test           listen once and report what the microphone heard
pn-brain drives             where the brain could live, and how much room is left
pn-brain move <dir>         move the brain to another drive, verifying every byte
pn-brain rewrite-paths      repair stored paths after a move, then re-embed
pn-brain import <dir>       load a Postgres export into a fresh database
```

## How it is built

Go, and a SQLite file. Vectors are stored as float32 blobs and similarity is
computed in process — exact, and cheaper than the network hop it replaced.

It used to be Laravel with Postgres, pgvector and Redis in Docker. That worked,
and it still could not be downloaded and run, which was the entire point. The
reasoning behind the switch — and the reasoning behind *not* switching, which
was wrong — is kept in [docs/ROADMAP.md](docs/ROADMAP.md).

## Licence

See [LICENSE](LICENSE).
