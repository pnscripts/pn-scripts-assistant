# pn-brain (Go CLI)

Terminal client for the PN Brain API. Zero dependencies (standard library only).

## Build

```bash
go build -o pn-brain .
```

## Use

```bash
./pn-brain "what's on my mind lately?"     # single-shot, continues the last conversation
./pn-brain --new "let's start fresh"       # start a new conversation
./pn-brain --provider anthropic "..."      # force a provider for this message
./pn-brain                                  # interactive mode (':new' to reset, 'exit' to quit)
```

Conversation state (just the current `conversation_id`) is kept in
`$XDG_CONFIG_HOME/pn-brain/cli.json` (or the OS equivalent) — not in this repo.

By default it talks to `http://localhost:8090`; override with `--api` or `BRAIN_API_URL`.

## Install globally

```bash
ln -sf "$(pwd)/pn-brain" ~/.local/bin/pn-brain   # ~/.local/bin must be on PATH
```

Then `pn-brain` works from any directory. It's a symlink to the binary here, so it
only works while this drive is plugged in and the containers are running
(`../../scripts/pn-brain-launch.sh`).
