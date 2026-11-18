# Other programs asking the assistant

Claude Code, Claude Desktop, Cursor and other programs that speak the Model
Context Protocol (MCP) can ask the assistant running on this computer. You
set it up once in that program; after that it starts
`pn-scripts-assistant mcp` by itself whenever it needs it.

## What they can ask
Two things, and both only read:

- **Is the assistant running**: its version, and whether its terms of use
  are accepted. Always answered.
- **Which programs are on this computer**: by name, with version and where
  each came from (menu, package, snap, Flatpak; on Windows the Start menu,
  installed programs, app packages). One program can be searched for by part
  of its name, and removed programs the assistant remembers can be listed
  with the day they were removed. **Off until you allow it** (below).

Nothing from your memory, mail, files, conversations or settings is offered,
and nothing that changes anything.

## You decide what they may ask
On the Programs page, the card "What other programs may ask" has one switch
for each thing that can be allowed. Today there is one: "Which programs are
installed on this computer". It is off on a new installation and after an
update. While it is off, a program that asks is told that you have not
allowed it and where you can; it gets no list.

The switch is set at this computer: not from a paired device and not by a
web page, and the asking program is given no way to change it. Every change
of it is written in "What I did", so a change you did not make would show
there.

## You see who asked
Every question is written in "What I did" with "Asked by" and the name the
program gave, whether it was answered or not. The last five are also listed
under the switch. If something another program asks for ever needs your
yes, its card says "Asked by" the same way, and offers yes or no for that
once: no "always" is offered for what another program asks, and another
program can never give the yes itself.

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
"ask PN Scripts Assistant whether GIMP is installed and which version".

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
  installed programs then leaves this computer with it. That is your choice
  when you allow it; switch it off again on the Programs page, or remove the
  entry there, to stop it.
- **The assistant sees its own entry.** Under Organisation, Integrations,
  "Found in your other programs", the entry you added is listed as the
  assistant itself, with nothing to import.
