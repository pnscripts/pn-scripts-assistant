# Coding agents as helpers

If Claude Code, Codex or Cursor's agent is installed on this computer and
signed in with your own account, the assistant can hand one of them a task:
"ask Claude Code why the tests in ~/projects/shop fail", "let Codex add a
README to ~/projects/notes".

## How far it may reach
You choose for each task:

- **Read** — it only reads the folder and answers. The default.
- **Folder** — it may change files in that folder and runs no commands.
- **Full** — it may run any command and change any file you can. Asked
  about every time, even when you have allowed everything else.

Like anything that changes something, the task is shown to you first: which
agent, how far it may reach, the folder and the task word for word. Your
rules go to the agent with the task, and are checked again against the
commands it ran.

## What comes back
Its answer, every file in the folder it added, changed or removed — read
from the folder itself afterwards, not taken from what it says — and the
commands it ran. While it works, what it is doing shows in the panel.

## Your own account
It works on your subscription, signed in as you signed it in. The assistant
never reads, keeps or passes on its sign-in, and starts it without any API
key in its environment. Not signed in? Open a terminal and run `claude`,
`codex login` or `cursor-agent login`, then ask again.

Never the whole disk, never the assistant's own data folder, and — for an
agent that may write — never your whole home folder or a system folder. One
task at a time, stopped after 30 minutes.
