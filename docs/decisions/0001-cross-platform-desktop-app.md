# ADR 0001 — Cross-platform desktop app

**Status:** accepted · **Date:** 2026-08-25 · **Settled:** 2026-09-01

## The real question

"Make it a desktop app" looks like a UI question. It isn't. There are two separable
problems and only one of them is about windows:

1. **A native window instead of a browser tab** — easy, cosmetic.
2. **Running on a machine with nothing installed first** — the actual problem.

Solving only (1) produces a browser wrapper that still demands a stack of services
before it will start. For personal use that is tolerable. For the stated goal — open
source on GitHub, installable by other people — it is fatal: nobody assembles a
server to try a personal assistant. Ollama, LM Studio and Obsidian are all one
download, and that is the bar.

So the desktop requirement forces a decision about the **backend runtime**, and that
is what this ADR is really about.

## What was decided

One program. No services beside it, nothing to start first, and no step between
downloading it and talking to it.

Concretely: a single Go binary, a native window over the system webview, and one
SQLite file holding everything the brain knows. Vectors are float32 blobs and
similarity is computed in process — exact at this size, and no index to install.

## The route not taken, and why it is recorded

The first answer to this was to keep the existing core and embed its runtime in the
app, so nothing would have to be rewritten. It was a reasonable answer to the wrong
question: the weight was never the language, it was the four processes around it —
a database, a vector extension and a queue, each of which had to be running before
the assistant existed at all. Embedding the runtime would have carried every one of
them along.

That is worth keeping written down because the reasoning that produced it was sound.
"Keep what works" is usually right, and it was not right here — the thing being kept
worked, and the product it added up to could not be downloaded and run, which was the
entire point.

## What it cost, measured

The rewrite was verified against the old system on the real data before anything was
removed: 79 facts, 145 lessons, 26 conversations, 79 messages and 15 tool calls
carried across, and the memory map rebuilt to the same 79 nodes and 177 links with
the same strongest pair, 6↔8 at 0.949.

## Consequences

- Vector search is brute-force cosine, and that is a deliberate ceiling rather than
  an oversight. It is exact and sub-millisecond at personal scale; somewhere in the
  tens of thousands of facts it needs a real index, and that is the point at which
  this decision gets revisited rather than patched.
- Windows and macOS builds happen where those systems are, not on this Linux
  machine — the window layer is the only platform-specific part.
- Everything the brain owns lives in one folder, which is what makes it possible to
  carry it on a drive and to keep copies of it. See ADR 0002 if that ever needs its
  own record.
