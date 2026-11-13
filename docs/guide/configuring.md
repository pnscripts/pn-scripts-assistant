# Setting it up

Български: [configuring.bg.md](configuring.bg.md)

Everything is in the window. The terminal is for people who prefer one, and
nothing here is a step you are required to take at a prompt.

## The first run

Press the Ubuntu key, type the first letters of the name, press Return. Setup
opens by itself and asks three things:

1. **Where the brain lives.** Any folder, on any drive. A memory on an external
   disk can be unplugged, carried to another machine and opened there: the
   program finds it by a marker file rather than by a remembered path.
2. **Which model does the thinking.** It looks for Ollama, says what is
   installed, and offers to install one that suits this machine. What it
   recommends depends on what it measured: cores, memory, and whether there is
   a graphics card.
3. **Keys, if you want them.** None is required. Without any key it works
   entirely on this machine, which is the default and the point.

Nothing changes until the last screen.

When the window opens for the first time it shows the [terms of
use](terms.md) before anything else: that it is an AI system whose answers can
be wrong, what stays on this computer, that it acts for you and that what it
sends is marked. One button accepts them, and until then it does nothing.
After an update that changes them it asks again. They stay readable on the
*What it runs on* page.

## Afterwards

`pn-scripts-assistant setup` opens the same wizard. Or use the System tab in
the window, which has the same settings one at a time, plus:

- **Privacy**: `private` (nothing leaves), `research` (search queries only),
  `open` (hosted models allowed). In open mode, your messages and the memory
  recalled for that turn are sent to the hosted provider you chose; the memory
  database itself stays on your machine. In `private` and `research` no memory
  leaves. `private` is about your words: with *May look things up about
  itself* on (a separate setting, on by default), the program may still
  download its own parts, such as a voice replaced by an update. Those
  downloads carry nothing about you; switch that setting off
  for a machine that must not reach the internet at all.
- **Permissions**: *act freely* by default: it does what you ask without
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
- **What is on this machine**: every language, engine, editor, coding agent,
  browser and tool it found, with its version and whether it is ready. This is
  the same list the assistant is given when it answers, so the panel and the
  program cannot disagree about whether Godot is installed.
- **Quiet hours for heavy work**: a window such as 22:00 until 07:00. Outside
  it, a coding agent whose paid allowance is being spent faster than it
  renews (judged from what the agent itself last said about its allowance)
  is not started on its own: the question says until when the work waits,
  and your yes starts it anyway. When nothing else would have asked about
  it (your permissions, a rule, full access), the question also says it
  starts by itself when the window begins, and it does, unless you answered
  first; what it said appears in the conversation it was asked in. Whatever
  you changed while it waited is checked again then: a permission now set to
  ask or never, or a rule that asks about it, leaves it to you. With the
  allowance on pace, inside the
  window, or with no window, nothing waits. Work nobody asked for at that
  moment waits for the window whatever the allowance, and starts in it by
  itself: goals set to start themselves (the goal says "waits for your
  quiet hours" and when it starts; "Work on it" starts it at once) and the
  reading of the places it learns from (the Places card says so; "Read some
  now" reads at once). Work already under way when the window closes is
  finished, not stopped. In the settings file it is
  `BRAIN_QUIET_HOURS=22:00-07:00`.

## Switching something off

Everything found is available; nothing has to be enabled. If you would rather
the assistant left something alone, press **Do not use** beside it in *What is
on this machine*. That takes away the tools that depend on it too (switch off
Godot and it stops offering to build Godot projects), and the permissions panel
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
| `PN_SCRIPTS_ASSISTANT_ADDR` | the address to serve on, e.g. `127.0.0.1:9000` (`BRAIN_ADDR`, its name before 1.0.2, still works) |
| `OLLAMA_BASE_URL` | where Ollama is, if not `http://127.0.0.1:11434` |

## The settings file

`brain.conf`, inside the brain's own folder rather than beside the program,
because it describes *this brain* and not this installation: move the drive to
another machine and the settings go with it. It is readable only by you, and so
is the database beside it.
