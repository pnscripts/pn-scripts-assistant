# Working on it

## What it is

One Go module in `clients/`, one binary. About 112,000 lines of Go, 40,000 of
them tests, and 23,000 lines of interface embedded in the binary — there is
nothing to serve from disk and nothing to install beside the program.

```
clients/cmd/pn-scripts-assistant   the program
clients/internal/brain/…           everything it is
clients/internal/setup/…           the first-run wizard
scripts/build-packages.sh          the packaging
packaging/                         the manual page
docs/decisions/                    why things are the way they are
docs/audit/                        what was measured, and when
```

## Building

```bash
CGO_ENABLED=1 go build -C clients -o ../build/pn-scripts-assistant ./cmd/pn-scripts-assistant
```

**CGO matters.** With `CGO_ENABLED=0` it builds a program with no native window
— it still serves its interface, and a browser still works, but the thing you
double-click does nothing you can see. That has cost an afternoon more than
once.

Needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`.

## Tests

```bash
make test        # every package
cd clients && go test ./internal/brain/tasks/ -run TestSomething -v
```

Sixty-one packages have them. They are written against the real thing wherever
that is possible: real files with real permissions, a real database, a real
HTTP server. A test that asserts against a simulation tests the simulation.

`make check` is `gofmt -l`, `go vet` and a check that two versions sort the way
dpkg thinks they do. It is the cheapest check there is and the one most worth
failing on.

## What CI will run

On every push and every pull request, in
[.github/workflows/ci.yml](../../.github/workflows/ci.yml):

| Job | What it does |
|---|---|
| Formatting and vet | `make check` |
| Tests | the whole suite **with `-race`** |
| Builds for … | a build for each of the five systems, and the no-window packages tested with cgo off |
| Known vulnerabilities | `govulncheck` |
| The package | the `.deb` built, put through `lintian`, then **built again and compared** |
| The package installs | it is installed, upgraded over and removed, in a root of dpkg's own |
| Shell scripts | `shellcheck --severity=error` |

`make ci` runs the same things here, in the same order. It is slow — several
minutes against `make check`'s few seconds — and it is the difference between
finding out now and finding out in a pull request.

The reproducibility step is worth knowing about before it fails on you:
everything the build writes takes its timestamp from the commit
(`SOURCE_DATE_EPOCH`), never from the clock. Putting a `date` back into the
packaging will make one commit produce two different packages, and that job
will say so.

## What is expected of a change

- It is formatted, vetted, and the whole suite passes.
- Anything it claims was measured, was measured. The audit documents in
  `docs/audit/` use a fixed vocabulary for this: VERIFIED, IMPLEMENTED — NOT
  VERIFIED, BROKEN, and so on. "It should work" is not one of them.
- A decision that somebody will wonder about later goes in
  `docs/decisions/`, including the ones that turn out to be wrong — ADR 0005
  now carries the correction to its own context-window reasoning.
- No new dependencies without a reason that survives being written down. There
  are three.

## Releasing

`make release`. See [releasing.md](releasing.md).
