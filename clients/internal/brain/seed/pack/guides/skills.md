# Skills

Open **Skills** in the menu. A skill is written instructions for one kind of
work — how to plan a trip, review a change, file an invoice. The assistant
offers a skill to the model only when what you ask matches what the skill is
for, and follows it with its own tools.

## Where skills come from
- **Built in** — skills and playbooks that ship with the program, with a
  "built-in" badge, in Bulgarian too when the screen is in Bulgarian.
- **From skill folders** — the skill folders of other agents on this
  computer: a folder with a `SKILL.md` (its name and what it is for at the
  top, the instructions below, and any `references/` or `scripts/` beside
  it), as Claude Code keeps them in `.claude/skills` in your home folder.
  Plain playbook files (`.md` with `name:`, `when:` and a line of dashes) are
  read too.
- **This assistant's own folder** — `my-skills` inside its data folder: put a
  skill folder there and press **Reload**.
- **Yours** — skills you wrote or taught on the Teaching page. Yours win over
  any of the same name.

Add folders of your own under **Where skills are read from**. Every folder is
read when the assistant starts, on every learning pass and when you press
**Reload**, so a skill you add or change is picked up.

## Switching skills off
Each skill has a switch. A skill switched off is not offered at all; switch
it on again at any time.

## What a skill may do
A skill is instructions, not permission. Anything that changes something —
writing a file, sending mail, running one of the skill's scripts — goes
through the same approval as always. A skill's scripts run only as commands
you approve.
