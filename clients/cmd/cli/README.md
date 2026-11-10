# pn-scripts-assistant-cli (Go CLI)

Terminal client for the PN Scripts Assistant API. Zero dependencies (standard library only).

## Build

```bash
go build -o pn-scripts-assistant-cli .
```

## Use

```bash
./pn-scripts-assistant-cli "what's on my mind lately?"  # single-shot, continues the last conversation
./pn-scripts-assistant-cli --new "let's start fresh"    # start a new conversation
./pn-scripts-assistant-cli --provider anthropic "..."   # force a provider for this message
./pn-scripts-assistant-cli                              # interactive mode (':new' to reset, 'exit' to quit)
```

Conversation state (just the current `conversation_id`) is kept in
`$XDG_CONFIG_HOME/pn-scripts-assistant/cli.json` (or the OS equivalent) — not in this repo.

By default it talks to `http://127.0.0.1:8790`, where PN Scripts Assistant serves unless told otherwise;
override with `--api` or `PN_SCRIPTS_ASSISTANT_API_URL` (`BRAIN_API_URL`, its name before 1.0.2, is still read).
Answers are signed with the name you gave the assistant, or `pn-scripts-assistant` before it has one.

## Install globally

```bash
ln -sf "$(pwd)/pn-scripts-assistant-cli" ~/.local/bin/pn-scripts-assistant-cli   # ~/.local/bin must be on PATH
```

Then `pn-scripts-assistant-cli` works from any directory. It's a symlink to the binary here, so it
only works while this drive is plugged in, and it answers only while PN Scripts Assistant is
running (`pn-scripts-assistant`, or `../../scripts/pn-scripts-assistant-launch.sh` from this tree).
