# Making a release

```bash
make release
```

One command, in one order, because a sequence somebody has to remember is a
sequence that gets a step skipped on the evening it matters.

| Step | What it does | Fails when |
|---|---|---|
| `clean` | removes `build/` | — |
| `check` | `gofmt -l`, `go vet ./...` | anything is unformatted or vet complains |
| `test` | `go test ./... -p 2` — every package | any test fails |
| `package` | builds the `.deb`: `-trimpath`, `-buildmode=pie`, the version stamped in, dependencies computed by `dpkg-shlibdeps`, the desktop entry validated | the build fails, or the entry is invalid |
| `validate` | the files policy requires are present, then `lintian --fail-on error,warning` | anything is missing, or lintian says anything at all |
| `checksums` | writes `SHA256SUMS` beside the package | — |

Each is a target of its own: `make test` after a change costs a minute,
`make release` costs several.

## The version

`VERSION` at the top of the repository is the only place that says what this
is. Everything else reads it:

- the package file name and its `Version:` field,
- the string stamped into the binary (`pn-scripts-assistant version`),
- what `/api/v1/version` reports.

At a tag that matches `VERSION` the package is `0.1.0`. Anywhere else it is
`0.1.0~git20260923.e9b80e0` — and the tilde sorts *before* the plain number in
dpkg's ordering, which is the right way round: a build from the middle of the
work is older than the release it is working towards.

A binary built by hand, outside the release script, says `built from source
(e9b80e0)` rather than claiming a number that cannot be looked up.

## Releasing

1. Put the new number in `VERSION`, commit it.
2. `make release`, and read what `validate` printed.
3. Tag it `v<version>` and push the tag.

CI then builds the same package on a clean machine, checks it the same way, and
attaches it to the release along with the binaries for other systems. The
publish job waits for the package job: a release that publishes everything
except the thing the documentation tells people to install is not a release.

## What is not automated yet

- **Reproducible bit-for-bit builds.** `-trimpath` and a timestamp-free gzip
  are in; the build has not been checked twice on two machines for an identical
  hash.
- **Signing.** Nothing is signed. The `.deb` is trusted because you built it or
  because its checksum matches, not because of a key.
