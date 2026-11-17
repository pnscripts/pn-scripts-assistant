# Other programs asking the assistant

Claude Code, Claude Desktop, Cursor and other programs that speak the Model
Context Protocol (MCP) can ask the assistant running on this computer. You
set it up once in that program; after that it starts
`pn-scripts-assistant mcp` by itself whenever it needs it.

## What they can ask today
Two things, and both only read:

- **Is the assistant running**: its version, and whether its terms of use
  are accepted.
- **Which programs are on this computer**: by name, with version and where
  each came from (menu, package, snap, Flatpak; on Windows the Start menu,
  installed programs, app packages). One program can be searched for by part
  of its name, and removed programs the assistant remembers can be listed
  with the day they were removed.

Nothing from your memory, mail, files, conversations or settings is offered,
and nothing that changes anything. More will come only together with a way
for you to see who asks and to decide at the desk.

## Setting it up
The assistant must be installed so that `pn-scripts-assistant` is a command
(the Linux package does that).

- **Claude Code**, in a terminal:
  `claude mcp add pn-scripts-assistant -- pn-scripts-assistant mcp`
- **Claude Desktop** (`claude_desktop_config.json`) and **Cursor**
  (`~/.cursor/mcp.json`), inside `"mcpServers"`:
  `"pn-scripts-assistant": {"command": "pn-scripts-assistant", "args": ["mcp"]}`

Then ask there, for example: "ask PN Scripts Assistant whether GIMP is
installed and which version".

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
  when you set it up there; remove the entry there to stop it.
- **The assistant sees its own entry.** Under Organisation, Integrations,
  "Found in your other programs", the entry you added is listed as the
  assistant itself, with nothing to import.
