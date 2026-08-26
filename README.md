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

Run `pn-brain-doctor` to see what is missing and install it.

## Running it

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

## Commands

The interface does everything; these exist for people who prefer a terminal.

```
pn-brain                    run the app: serve, and open the window
pn-brain serve              serve only, without a window
pn-brain status             where the data lives and what is in it
pn-brain ingest <dir>...    learn about the projects and documents in a folder
pn-brain promote            turn validated lessons into durable knowledge
pn-brain tidy               clear self-descriptions out of the review queue
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
