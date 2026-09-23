# When something is wrong

Start here: **the log**.

```
~/.local/state/pn-scripts-assistant/logs/pn-scripts-assistant.log
```

`pn-scripts-assistant status` prints that path along with where the data lives.
The file holds the same lines the terminal would have shown, which is the point
— started from the applications menu there is no terminal to show them. It is
rotated at five megabytes and three are kept, it is readable only by you, and
secrets are taken out of it before it is written.

## It does not start

| What you see | What it means |
|---|---|
| `Assistant is already running on this brain (…) and answering on http://127.0.0.1:8790` | A second copy was asked to open the same brain. Use the one that is running, or serve a different brain with `PN_SCRIPTS_ASSISTANT_DATA_ROOT=<folder>`. |
| `something is already answering on 127.0.0.1:8790` | Something else has the port. Open that address to see what it is, or start elsewhere with `BRAIN_ADDR=127.0.0.1:9000`. |
| Nothing at all, launched from the menu | Read the log. If it is empty, run `pn-scripts-assistant` in a terminal — the reason will be on screen. |
| It waits for a drive | The brain is on a disk that is not plugged in. Plug it in, or `pn-scripts-assistant start-again` to stop waiting for one that is gone for good. |

## It answers slowly

Minutes, on a machine with no graphics card, is normal for the first question
after it starts: the model reads the whole conversation before it writes a
word. Measured on four cores with a 7B model: **598 s** for the first question,
**58 s** for the next one in the same conversation, because the model keeps
what it has already read.

Ask it: *"why are you so slow"*. It answers with the real numbers for this
machine and says what would help.

Two Ollama settings matter more than anything else:

```
OLLAMA_MAX_LOADED_MODELS=2    # or every reply evicts the chat model
OLLAMA_HOST=127.0.0.1:11434   # not 0.0.0.0
```

## It says it has no model

Ollama is not running, or is answering somewhere else. The message names the
address it tried:

```
Ollama is not answering at http://127.0.0.1:11434 — start it (ollama serve),
or point this at another one with OLLAMA_BASE_URL
```

## It calls tools instead of answering, or ignores what it was told

That was a truncated prompt, fixed in 0.1.0: the whole instruction is now sent
to the model rather than the last quarter of it. `pn-scripts-assistant version`
says which copy you have.

## The microphone hears nothing

`pn-scripts-assistant mic-test` listens once and says what it heard. The
microphone is **off** until you turn it on in the interface — a fresh install
does not open it.

## A web page cannot reach it, and should not

If something tries to use the assistant's API from a browser tab on another
site it is refused:

```
that request came from somewhere else
this assistant answers to localhost, not to example.com
```

That is deliberate. See [security.md](../audit/security.md).
