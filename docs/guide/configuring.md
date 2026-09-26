# Setting it up

Everything is in the window. The terminal is for people who prefer one, and
nothing here is a step you are required to take at a prompt.

## The first run

Press the Ubuntu key, type the first letters of the name, press Return. Setup
opens by itself and asks three things:

1. **Where the brain lives.** Any folder, on any drive. A memory on an external
   disk can be unplugged, carried to another machine and opened there — the
   program finds it by a marker file rather than by a remembered path.
2. **Which model does the thinking.** It looks for Ollama, says what is
   installed, and offers to install one that suits this machine. What it
   recommends depends on what it measured: cores, memory, and whether there is
   a graphics card.
3. **Keys, if you want them.** None is required. Without any key it works
   entirely on this machine, which is the default and the point.

Nothing changes until the last screen.

## Afterwards

`pn-scripts-assistant setup` opens the same wizard. Or use the System tab in
the window, which has the same settings one at a time, plus:

- **Privacy** — `private` (nothing leaves), `research` (search queries only),
  `open` (what you type may go to a hosted model). What the assistant has
  learned about you never leaves in any mode.
- **Permissions** — *act freely* by default: it does what you ask without
  stopping to confirm, and writes down everything it does. *Ask about what I
  have not allowed* and *ask every time* are the other two.

  These are two separate settings and neither moves the other. Allowing the
  assistant to act on this machine does not let anything leave it, and opening
  privacy does not give it a free hand here. They were one switch until
  September 2026, which meant exactly those two surprises.
- **The microphone**, which is off until you turn it on, and the voice, which
  is on.
- **The applications menu**, if you built the program yourself rather than
  installing the package. Installed from the package it is already there and
  the button says so.
- **What is on this machine** — every language, engine, editor, coding agent,
  browser and tool it found, with its version and whether it is ready. This is
  the same list the assistant is given when it answers, so the panel and the
  program cannot disagree about whether Godot is installed.

## Switching something off

Everything found is available; nothing has to be enabled. If you would rather
the assistant left something alone, press **Do not use** beside it in *What is
on this machine*. That takes away the tools that depend on it too — switch off
Godot and it stops offering to build Godot projects — and the permissions panel
says which decision put a capability out of reach, rather than leaving you to
guess between a privacy setting and your own.

It is one line in the settings file, empty until you write in it:

```
BRAIN_TURNED_OFF=godot,service:openai
```

## Settings that live in the environment

For the few things that have to be decided before the program starts:

| Variable | What it does |
|---|---|
| `PN_SCRIPTS_ASSISTANT_DATA_ROOT` | use this folder as the brain, instead of the remembered one |
| `BRAIN_ADDR` | the address to serve on, e.g. `127.0.0.1:9000` |
| `OLLAMA_BASE_URL` | where Ollama is, if not `http://127.0.0.1:11434` |

## The settings file

`brain.conf`, inside the brain's own folder rather than beside the program,
because it describes *this brain* and not this installation — move the drive to
another machine and the settings go with it. It is readable only by you, and so
is the database beside it.
