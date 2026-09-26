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

`make release` builds the Ubuntu package, because that is the one this machine
can build and check completely. `make packages` builds the macOS bundles and
disk images and the Windows folder as well — everything except the Windows
installer, which needs Inno Setup and therefore needs Windows. CI builds each
one on its own system, which is where the published files come from.

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

CI then builds every download on the system it belongs to — the `.deb` and its
`lintian` check on Ubuntu, the disk images with `hdiutil` on a Mac, the
installer with Inno Setup on Windows — and attaches them all to the release
with one `SHA256SUMS` over the lot. The publish job waits for every one of
them: a release that publishes everything except the thing the documentation
tells people to install is not a release, and that is as true of the installer
as it is of the package.

The two checks in those jobs are there because both failures are silent
otherwise. The macOS job runs `hdiutil imageinfo` on what it built, so an
image that quietly came from the cross-platform fallback fails the job rather
than the download. The Windows job checks the setup `.exe` exists, because the
script only warns when Inno Setup is missing — right for somebody building on
Linux, wrong for the job whose whole purpose is that file.

## What is not automated yet

- **Reproducible builds across machines.** The `.deb` is now byte-for-byte
  reproducible *on one machine*: two builds of one commit produce the same
  SHA-256, verified, and CI builds it twice on every push and fails if they
  differ. What made that possible was pinning every timestamp the build writes
  to the commit's own date (`SOURCE_DATE_EPOCH`) — before that the changelog
  carried the moment the script ran and dpkg wrote the staging directory's
  mtimes, so one commit had as many checksums as it had builds. What is still
  open is the harder half: the same commit built on a *different* machine, with
  a different toolchain path and a different filesystem, has not been compared.
- **Signing.** Nothing is signed anywhere: no Debian key, no Apple Developer
  certificate, no Windows code-signing certificate. Each download is trusted
  because you built it or because its checksum matches, not because of a key.
  macOS and Windows both say so at the first open, and
  [installing.md](installing.md) says how to get past each.
