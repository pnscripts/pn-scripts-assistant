# PN Scripts Assistant 0.1.0

The first release that can be installed rather than built. Ubuntu 24.04 or
newer, 64-bit.

```bash
sudo apt install ./pn-scripts-assistant_0.1.0_amd64.deb
```

Then press the Ubuntu key and type the first few letters of its name.

## What it is

A personal assistant that keeps what it learns in one folder on your own
machine, runs its language model locally, and by default sends nothing
anywhere. One binary: no database server, no container runtime, no account.

## What is in this release that was not working before

**The model now reads all of its instructions.** A conversation turn is about
6,300 tokens — who the assistant is, the tools it may use, what it remembers.
Ollama's default window is 4,096 and it discards the overflow silently, from
the beginning, which is where the instructions are. The visible symptom was a
model that called the same tool over and over and never answered. Each request
now asks for a window that fits it, so "say hello" is answered rather than
investigated.

**A web page can no longer drive your assistant.** Any site you had open could
post to the assistant on `127.0.0.1` and be obeyed — it looked like a request
from your own machine, because it was. Requests must now name this computer and
come from the assistant's own page. Programs and the command line are
unaffected.

**Your database is yours.** It was created world-readable; on a shared machine
that is every account reading every conversation. It is 0600 now, along with
its journal, and an existing one is tightened the next time it opens.

**Secrets stay out of the record.** A token printed by a command you approved
was kept in the conversation and written to the log. Both are scrubbed now.

**It knows which copy it is.** `pn-scripts-assistant version`, which did not
exist.

**It says where to look when it will not start.** A log at
`~/.local/state/pn-scripts-assistant/logs/`, rotated, readable only by you, and
named by `pn-scripts-assistant status`.

**One assistant per brain.** Two copies on one data root shared a database
without either of them saying so; the second now refuses and says where the
first is answering.

**The microphone is off until you ask for it.**

**Ollama is found when it is running**, not only when its command happens to be
on your PATH.

## Known, and not fixed in this release

- **Slow on a machine with no graphics card.** Measured here, four cores: the
  first question in a conversation takes about ten minutes, the next about one
  — the model keeps what it has already read. Ask it *"why are you so slow"*
  and it answers with your own numbers.
- **A greeting pays for every tool.** About 5,100 of those 6,300 tokens are
  tool descriptions.
- **A transient network failure ends a chat turn.** Tasks survive it;
  conversations do not.
- **Nothing is signed**, and the build has not been checked for
  bit-for-bit reproducibility.

## Verifying what you downloaded

```bash
sha256sum -c SHA256SUMS
```

## Removing it

`sudo apt remove pn-scripts-assistant` takes away the program and leaves
everything it learned. [The guide](guide/installing.md) says what to delete
when you mean it.

---

Full audit of this release: [docs/audit/final-release-audit.md](audit/final-release-audit.md).
