# vesper (Go CLI)

Terminal client for the Vesper API. Zero dependencies (standard library only).

## Build

```bash
go build -o vesper .
```

## Use

```bash
./vesper "what's on my mind lately?"     # single-shot, continues the last conversation
./vesper --new "let's start fresh"       # start a new conversation
./vesper --provider anthropic "..."      # force a provider for this message
./vesper                                  # interactive mode (':new' to reset, 'exit' to quit)
```

Conversation state (just the current `conversation_id`) is kept in
`$XDG_CONFIG_HOME/vesper/cli.json` (or the OS equivalent) — not in this repo.

By default it talks to `http://localhost:8090`; override with `--api` or `BRAIN_API_URL`.
