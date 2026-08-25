# pnexus (Go CLI)

Terminal client for the Pnexus API. Zero dependencies (standard library only).

## Build

```bash
go build -o pnexus .
```

## Use

```bash
./pnexus "what's on my mind lately?"     # single-shot, continues the last conversation
./pnexus --new "let's start fresh"       # start a new conversation
./pnexus --provider anthropic "..."      # force a provider for this message
./pnexus                                  # interactive mode (':new' to reset, 'exit' to quit)
```

Conversation state (just the current `conversation_id`) is kept in
`$XDG_CONFIG_HOME/pnexus/cli.json` (or the OS equivalent) — not in this repo.

By default it talks to `http://localhost:8090`; override with `--api` or `BRAIN_API_URL`.

## Install globally

```bash
ln -sf "$(pwd)/pnexus" ~/.local/bin/pnexus   # ~/.local/bin must be on PATH
```

Then `pnexus` works from any directory. It's a symlink to the binary here, so it
only works while this drive is plugged in and the containers are running
(`../../scripts/start-brain.sh`).
