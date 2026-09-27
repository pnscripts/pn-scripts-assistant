# Contributing

Issues, questions and pull requests are all welcome. So is being told that
something in here is wrong.

## Before a pull request

Read [docs/guide/developing.md](docs/guide/developing.md) — it is the real
document, and it says how to build, how to run the tests, and what a change is
expected to carry. The short version:

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
make check    # gofmt, go vet, and that versions sort the way dpkg thinks
make test     # every package
```

**Build with `CGO_ENABLED=1`.** With cgo off it compiles a program with no
native window — it still serves its interface and a browser still works, so
nothing fails and nothing is visibly wrong. That has cost an afternoon more
than once.

CI runs the same checks on every push and every pull request, plus the race
detector, a cross-compile for each system, `govulncheck`, and the `.deb` built,
put through `lintian`, built a second time and compared, then installed,
upgraded over and removed. If it is red, it is red for a reason.

## What a change should carry

- **A test, where there is something to test.** Written against the real thing
  where that is possible — real files with real permissions, a real database, a
  real HTTP server. A test that asserts against a simulation tests the
  simulation.
- **A comment saying why, not what.** The code in here explains its reasoning,
  including the attempts that did not work, because the wrong answer that
  sounded reasonable is the thing a later change will reach for again.
- **Honesty about what was checked.** If you measured it, say so. If you did
  not, say that instead. The audit documents in `docs/audit/` use a fixed
  vocabulary for exactly this: VERIFIED, IMPLEMENTED — NOT VERIFIED, BROKEN.
  "It should work" is not one of them.
- **A decision record**, in `docs/decisions/`, for anything somebody will
  wonder about later — including the decisions that turn out to be wrong.
- **A line in `clients/internal/outside/outside.go`** if it makes the program
  connect somewhere new, saying what the host is for and when it is reached.
  The test there reads the whole source and will fail until you do; that is
  deliberate, and it is how "there is no telemetry in this" stays a fact
  rather than a promise.

## What will be pushed back on

- **A new dependency.** There are three, and each earned its place in writing.
  A fourth needs a reason that survives being written down.
- **A feature that only works from a terminal.** Everything this program does
  has to be reachable from inside the program. A terminal is for people who
  want one, never the only way.
- **A claim in a comment or a document that was not checked.** This project's
  whole method is that what it says about itself is true.

## The prose

British English, sentences rather than fragments, and no exclamation marks in
the interface. The program talks to somebody about their own machine; it reads
better when it sounds like a person who knows the machine and is not selling
anything.

## Licence

This is MIT. By opening a pull request you are offering your change under the
same licence. There is no CLA and there will not be one.
