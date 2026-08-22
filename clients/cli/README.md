# brain (Go CLI)

Terminal client for the AI Brain API. Zero dependencies (standard library only).

## Build

```bash
go build -o brain .
```

## Use

```bash
./brain "what's on my mind lately?"     # single-shot, continues the last conversation
./brain --new "let's start fresh"       # start a new conversation
./brain --provider anthropic "..."      # force a provider for this message
./brain                                  # interactive mode (':new' to reset, 'exit' to quit)
```

Conversation state (just the current `conversation_id`) is kept in
`$XDG_CONFIG_HOME/ai-brain/cli.json` (or the OS equivalent) — not in this repo.

By default it talks to `http://localhost:8090`; override with `--api` or `BRAIN_API_URL`.
