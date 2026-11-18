# Other programs asking the assistant

Claude Code, Claude Desktop, Cursor and other programs that speak the Model
Context Protocol (MCP) can ask the assistant running on this computer. You
set it up once in that program; after that it starts
`pn-scripts-assistant mcp` by itself whenever it needs it.

## What they can ask
Four things. None of them changes anything:

- **Is the assistant running**: its version, and whether its terms of use
  are accepted. Always answered.
- **Which programs are on this computer**: by name, with version and where
  each came from (menu, package, snap, Flatpak; on Windows the Start menu,
  installed programs, app packages). One program can be searched for by part
  of its name, and removed programs the assistant remembers can be listed
  with the day they were removed. **Off until you allow it** (below).
- **A question to the assistant itself**, answered in words by the model on
  this computer. It does nothing for the program that asks: no files, no
  commands, no settings, no messages, and it is not a conversation of yours
  (nothing is learned from it and no task starts). It knows nothing about
  you unless the next one is allowed too. One question at a time, and your
  own question first: while you are being answered a program is told to ask
  again in a moment, and a program's question the model is still working on
  is given up the moment you ask something. **Off until you allow it.**
- **A search of what the assistant remembers about you and your work**: the
  few remembered things closest to what was searched for (5, at most 10),
  each cut short. Never the whole memory. What is remembered was learned
  from your files and conversations, and a remembered thing can name the
  file it came from and where that file is. **Off until you allow it**, and the one to think about before you do: see
  "Where the answer goes".

No file, mail, conversation or setting of yours is handed over as such, and
nothing that changes anything. What the assistant remembered from them can
be, with the memory switch.

A question to the assistant is answered only by the model on this computer,
never by a service you connected, so another program's questions cost you
nothing and go nowhere. That model can be slow. When an answer takes longer
than most of a minute, the program is given a ticket and fetches the answer
with it a little later; you do not have to do anything.

## You decide what they may ask
On the Programs page, the card "What other programs may ask" has one switch
for each thing that can be allowed: "Which programs are installed on this
computer", "Questions to me, answered in words" and "Searching what I
remember about you". Each is off on a new installation and after an update,
and allowing one never allows another. Under the last two the card says what
allowing them means before you tick them. While a switch is off, a program
that asks is told that you have not allowed it and where you can; it gets
nothing else. Turning one of the last two off also takes back what is in
hand: a question the model is still working on is given up, and an answer
the program has not fetched yet is no longer handed over.

With both of the last two on, the assistant's answers to other programs'
questions use what it remembers, as its answers to you do.

The switch is set at this computer: not from a paired device and not by a
web page, and the asking program is given no way to change it. Every change
of it is written in "What I did", so a change you did not make would show
there.

## You see who asked
Every question is written in "What I did" with "Asked by" and the name the
program gave, whether it was answered or not: a question with the start of
its answer (the first 600 characters), a search with what was searched for
and how many things were handed over. The
last five are also listed under the switches. If something another program asks for ever needs your
yes, its card says "Asked by" the same way, and offers yes or no for that
once: no "always" is offered for what another program asks, and another
program can never give the yes itself.

At most 30 questions a minute are answered, from all programs together. The
ones over that are refused, and written as one line, so a program that asks
in a loop is held to that many lines a minute on the record. Whether the
assistant is running counts too.

The name is what the program says about itself. Nothing can check it, so it
is shown to you and never decides anything.

The switch and the record are for programs that ask the way they are meant
to, through `pn-scripts-assistant mcp`. A program running under your own
account on this computer can do what you can do at this computer, as with
your files; see "Known limits".

## Setting it up
The assistant must be installed so that `pn-scripts-assistant` is a command
(the Linux package does that).

- **Claude Code**, in a terminal:
  `claude mcp add pn-scripts-assistant -- pn-scripts-assistant mcp`
- **Claude Desktop** (`claude_desktop_config.json`) and **Cursor**
  (`~/.cursor/mcp.json`), inside `"mcpServers"`:
  `"pn-scripts-assistant": {"command": "pn-scripts-assistant", "args": ["mcp"]}`

Then allow the question on the Programs page and ask there, for example:
"ask PN Scripts Assistant whether GIMP is installed and which version", or
"search PN Scripts Assistant's memory for what I decided about the invoice
template".

## What to know
- **The assistant must be running.** `pn-scripts-assistant mcp` holds
  nothing itself: it passes each question to the running assistant on this
  computer and says so plainly when none is running, or when the terms of
  use are not accepted yet.
- **Only this computer.** It asks the assistant on this computer's own
  address and refuses any other. If you changed the port the assistant
  serves on, add `--api http://127.0.0.1:<port>` after `mcp`.
- **Where the answer goes.** What a tool answers is given to the program
  that asked, and that program passes it to whatever model it uses, which
  for Claude and Cursor is a service on the internet. The list of your
  installed programs, the assistant's answer or what it found in its memory
  then leaves this computer with it. What the memory holds was learned from
  your files and conversations, so allow searching it only for programs you
  would show those to. That is your choice when you allow it; switch it off
  again on the Programs page, or remove the entry there, to stop it.
- **The assistant sees its own entry.** Under Organisation, Integrations,
  "Found in your other programs", the entry you added is listed as the
  assistant itself, with nothing to import.
