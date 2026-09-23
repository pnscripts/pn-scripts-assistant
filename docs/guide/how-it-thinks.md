# Models, and which one answers

## On this machine, by default

The assistant talks to [Ollama](https://ollama.com) on `127.0.0.1:11434`. Two
models are used for different things: one that answers and does the work, and a
small one (`nomic-embed-text`) that turns text into vectors so that memory can
be searched by meaning rather than by keyword.

What it chooses depends on what it measured about the machine — cores, memory,
whether there is a graphics card. On four cores with no card it picks a 7B
model and tells you it will be slow, rather than picking something large and
leaving you to discover it.

## How much it is given to read

A conversation turn is bigger than it looks: who the assistant is, the tools it
may call, what it remembers that is relevant, and the conversation so far.
Measured here: **6,338 tokens** for "say hello" with 39 tools offered.

Ollama's own window is 4,096 tokens and it discards what does not fit, silently
and from the beginning — which is where the instructions are. Until 0.1.0 that
was happening on every turn: the model was handed the tail of a tool list and
a question, and behaved exactly as something would that had never been told
what it was.

Each request now asks for a window sized to itself, rounded up to a step,
capped by how much memory the machine has, and never shrunk back within a run —
because Ollama reloads the model whenever the window changes, and the reload
throws away everything it had already read.

That last part is why the second question in a conversation is ten times
faster than the first.

## Hosted models

Keys for Anthropic, OpenAI, OpenRouter and the rest can be set in the System
tab, and none is required. Which one answers a given turn depends on the
privacy mode and on what is signed in; the local model is always the fallback,
so a service being out of allowance degrades rather than fails.

**What the assistant has learned about you never goes to a hosted model**, in
any mode. Conversations are typed deliberately; memory is assembled from your
disk without you composing it, so it is not the program's to forward. There is
no setting that changes this.

## Who writes code

For real work — building a project, writing a game — the choice is made per
piece of work: Claude Code or Codex on your own subscription first, then a
model on this machine. It switches before a limit is hit and falls back when a
service is signed out, and every decision and switch is recorded as evidence
you can read afterwards.

Checking, running and building are never a model's word: the program runs the
engine itself and reports what happened. See
[ADR 0005](../decisions/0005-choose-how-work-is-done.md).
