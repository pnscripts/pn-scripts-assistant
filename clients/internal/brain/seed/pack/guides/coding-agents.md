# Coding agents as helpers

If Claude Code, Codex or Cursor's agent is installed on this computer and
signed in with your own account, the assistant can hand one of them a task:
"ask Claude Code why the tests in ~/projects/shop fail", "let Codex add a
README to ~/projects/notes".

## How far it may reach
You choose for each task:

- **Read**: it only reads the folder and answers. The default.
- **Folder**: it may change files in that folder and runs no commands.
- **Full**: it may run any command and change any file you can. Asked
  about every time, even when you have allowed everything else.

Like anything that changes something, the task is shown to you first: which
agent, how far it may reach, the folder and the task word for word. Your
rules go to the agent with the task, and are checked again against the
commands it ran.

## What comes back
Its answer, every file in the folder it added, changed or removed (read
from the folder itself afterwards, not taken from what it says) and the
commands it ran. While it works, what it is doing shows in the panel.

## Your own account
It works on your subscription, signed in as you signed it in. The assistant
never reads, keeps or passes on its sign-in, and starts it without any API
key in its environment. Not signed in? Open a terminal and run `claude`,
`codex login` or `cursor-agent login`, then ask again.

Never the whole disk, never the assistant's own data folder, and, for an
agent that may write, never your whole home folder or a system folder. One
task at a time, stopped after 30 minutes.

## Ask Claude a question about your files
"Ask Claude to summarise notes.md and save the summary as summary.md": the
assistant gives your own Claude one question and only the files you name,
and brings the answer back. Your own Claude means your Claude Code, signed
in on this computer by you, or the Anthropic key you typed into the
assistant (in setup or on the Models page). A key that is only in the
environment is never used, and the assistant has no key of its own to use.

Every time, before anything goes, you see which Claude, the question word
for word, each file with its size and, when you asked for it, the new file
the answer will be saved as. Nothing else goes: Claude gets no tools, so it
cannot open anything on this computer, and runs in an empty folder without
your Claude Code settings, hooks or MCP servers. The files are read after
your yes and must be what you were shown: a file changed in between is not
sent, and you are asked again.

Never sent: protected files (keys, passwords, the assistant's own data),
links, anything that is not text, more than 10 files, a file over 256 KB or
512 KB together. The answer is saved only as a new file, never over one.

Claude's answer comes back marked as information, not instructions: a line
in it that tells the assistant to do something is read as text, and
anything it would lead to is asked like everything else.
