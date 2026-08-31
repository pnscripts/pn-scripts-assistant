# pn-brain-doctor (Go)

Checks what PN Brain needs from this machine, and installs what's missing.

```bash
pn-brain-doctor              # report
pn-brain-doctor --install    # fix what can be fixed, asking before each step
pn-brain-doctor --install --yes
pn-brain-doctor --quiet      # silent when healthy; used by the launcher
```

## Built to be run forever, not once

Setup tools usually assume a one-time install, which is wrong here: models get
replaced, Docker gets upgraded, a distribution moves a library out from under
you. So this reports current state on every run, and installing is simply
"make reality match the list" — the same command works on a fresh machine and
on one that's been running PN Brain for a year. `scripts/pn-brain-launch.sh` runs it
quietly on every launch, so drift surfaces when it happens rather than as a
confusing failure later.

## Why it is a separate static binary

Something has to be able to say "you have no Ollama" on a machine where Ollama
is missing — so it cannot live inside the brain, which needs a model before it
can say anything at all. It is pure standard library, cross-compiles anywhere,
and depends on nothing.

## Design notes

Each requirement in `requirements_list.go` carries **why** it exists and **what
breaks without it**. A list of red crosses tells someone what is wrong but not
whether they should care, and several requirements here are genuinely optional
— a missing web engine costs you the native window, not the app.

Optional requirements are reported and never block startup.

Two deliberate refusals:

- **Ollama is not auto-installed.** Its official instructions pipe a remote
  script into a shell. Normalising that in a tool that also runs `sudo` is a
  bad habit to build, so it links to the download page instead.
- **Installs are re-checked afterwards.** An `apt` command can exit zero having
  installed something that still doesn't satisfy the check; reporting success
  on the strength of an exit code would be a lie.
