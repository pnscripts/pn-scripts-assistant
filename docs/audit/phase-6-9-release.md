# Phases 6–9 — Ubuntu support, the package, the install, the release

**Date:** 2026-09-23 · **Machine:** Ubuntu 24.04.5, x86_64 · Everything below
was run here unless it says otherwise.

## Phase 6 — Ubuntu production support

| Requirement | Status | Evidence |
|---|---|---|
| Nothing depends on the development directory | **VERIFIED** | `strings` over the packaged binary: **0** references to `/media/petar/…`. The packaging build already used `-trimpath`; an ad-hoc `go build` does not, which is why a scratch build shows 349. |
| User data under `~/.local/share` | **VERIFIED** | A fresh run creates `~/.local/share/pn-scripts-assistant/data` and nothing else outside XDG paths. |
| Configuration under `~/.config` | **VERIFIED** | `~/.config/pn-scripts-assistant/last-root.json`. The settings themselves live in the brain's own folder, by design: they describe the brain, and travel with it when the drive moves. |
| State and logs under `~/.local/state` | **VERIFIED** | Phase 3. `~/.local/state/pn-scripts-assistant/logs/`, 0700, log 0600, rotated. |
| Desktop entry and icons where Ubuntu looks | **VERIFIED** | `/usr/share/applications`, `/usr/share/icons/hicolor/{48,64,128,256,512}` in the package. |
| The window is matched to its launcher | **VERIFIED** | Phase 4: `WM_CLASS` is `pn-scripts-assistant` whatever the file is called. |

## Phase 7 — a real `.deb`

Built by `make package`, which is `scripts/build-packages.sh deb`.

**lintian is silent.** No errors, no warnings, with
`--fail-on error,warning` in the release path and in CI. It was 23 warnings
before this phase.

| Finding from the audit | Status |
|---|---|
| H2 — no `copyright` (Debian policy §12.5) | **FIXED** — machine-readable MIT, plus `changelog.gz` |
| M5 — dependencies written out by hand | **FIXED** — `dpkg-shlibdeps` reads the binary: 7 packages with versions, where 2 were written by hand |
| L2 — desktop file installed 0664 | **FIXED** — 0644, and every directory 0755 rather than this machine's umask |
| L3 — `Maintainer: noreply@localhost`, no `Homepage` | **FIXED** — a person and a repository |
| `no-manual-page` | **FIXED** — `man pn-scripts-assistant`: commands, files, environment, and what it answers to on the network |
| `no-md5sums-control-file` | **FIXED** — dpkg can now say which installed file has been changed |
| `hardening-no-pie` | **FIXED** — `-buildmode=pie`, like everything else Ubuntu ships |
| `description-synopsis-starts-with-article`, long lines | **FIXED** |
| `wrong-name-for-changelog-of-native-package` | **FIXED** — `changelog.gz`, as a native package's must be |

What the package carries, and nothing else:

```
/usr/bin/pn-scripts-assistant                                   755
/usr/share/applications/pn-scripts-assistant.desktop            644
/usr/share/icons/hicolor/{48,64,128,256,512}/apps/…png          644
/usr/share/man/man1/pn-scripts-assistant.1.gz                   644
/usr/share/doc/pn-scripts-assistant/{copyright,changelog.gz}    644
```

`desktop-file-validate` runs over the entry at build time and fails the build.

## Phase 8 — the install lifecycle

**Status: BLOCKED — awaiting the password for `sudo`.** `apt install` was
started in a terminal on this machine and is sitting at the password prompt;
nothing about a system-wide install is claimed here until it has run.

What was verified without root, using the real package's own extracted files:

| Check | Result |
|---|---|
| The packaged binary runs from where the package puts it, on a fresh `HOME` | **VERIFIED** — `version`, `status`, `serve` all work; creates only XDG paths |
| `pn-scripts-assistant version` matches the package's version | **VERIFIED** — `0.1.0~git20260923.983e04f (983e04f), built 2026-09-23` |
| `menu` with the packaged entry present | **VERIFIED** — "already in the applications menu, installed with the package. Nothing was written." No second entry. |
| `menu --remove` on a packaged entry | **VERIFIED** — refused, exit 1, names `sudo apt remove pn-scripts-assistant` |

That last pair found a real defect in the Phase 4 fix: it compared the *full
path* of the running binary against the entry's `Exec` line, while a package
writes `Exec=pn-scripts-assistant` — a bare name resolved on the PATH. The
check passed its own tests, because its own tests wrote entries the way this
program writes them, and wrote a second entry the first time it met a real
`.deb`. Both shapes are read now, and a bare name resolving to a *different*
copy of the program is still not this one.

Still to be done, all of it needing root: `apt install`, launching from the
menu on the desktop, an upgrade over an installed copy, `apt remove`, and what
is left behind afterwards.

## Phase 9 — a release anybody can repeat

`make release` — clean, `gofmt`/`vet`, every test, the package, the checks,
the checksums. Each step is also a target of its own.

Run end to end on this machine: **62 packages pass, lintian clean, SHA256SUMS
written**, and the whole thing takes about five minutes.

### One version, where there were three

`VERSION` at the top of the repository. The package name, the `Version:`
field, the string in the binary and `/api/v1/version` all read it. At a
matching tag the package is `0.1.0`; anywhere else it is
`0.1.0~git20260923.983e04f`, which dpkg sorts *before* the release it is
working towards. A build made outside the release script says `built from
source (983e04f)` rather than claiming a number nobody can look up.

`pn-scripts-assistant version` did not exist before this phase. It is the
first question anybody is asked when they report something.

### CI

M6's other half: the `.deb` is now built and checked in CI, on a clean
machine, with `lintian --fail-on error,warning`, and the publish job waits for
it. Every other artifact was already built there; the one the documentation
tells people to install was not.

**NOT VERIFIED:** the CI job itself has not run — it needs a push, and pushing
is the owner's call.

### What is not reproducible yet

- **Bit-for-bit.** `-trimpath` and a timestamp-free gzip are in, and the build
  has **not** been run twice on two machines and compared. Claiming
  reproducibility on the strength of the flags would be exactly the kind of
  claim this audit is meant to refuse.
- **Signing.** Nothing is signed.

## Phase 10 — documentation

The README described a release that does not exist — "download the AppImage",
when there are no tags and no published artifacts — and told the reader to run
`pn-scripts-assistant-doctor`, which the package does not install. It now
describes the package: installing, checking the checksum, updating over it,
and removing it, including that `apt remove` deliberately leaves everything
learned.

Six guides beside it in `docs/guide/`: installing, setting it up, which model
answers and why the second question is faster than the first, what to read
when something is wrong, working on the code, and making a release.

**PHASES 6, 7, 9 AND 10 COMPLETE · PHASE 8 BLOCKED**
