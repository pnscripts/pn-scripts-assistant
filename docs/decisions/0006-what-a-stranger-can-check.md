# ADR 0006 — What a stranger can check

**Status:** accepted · **Date:** 2026-09-27

## The real question

This program reads somebody's files, keeps what it learns about them in a
database, and runs commands as them. Its central claim — that all of that stays
on their machine — is the one thing a person deciding whether to install it
cannot verify by using it. By the time they could tell, they would already have
given it everything.

So the question is not "is the claim true" — it is true, and the code enforces
it — but **what can somebody who has never met the maintainer check for
themselves?** Every answer that ends in "because we say so" is not an answer.

Until now, the honest list of what a stranger could check was short: read the
source, which almost nobody does; and run the tests, which requires trusting
that the tests test what they say. Everything else was assertion. The
documentation was unusually careful — the audits distinguish measured from
implemented, which is rarer than it should be — and careful assertion is still
assertion.

## The decision

**Every claim this project makes about itself is either checked by something
that runs on every push, or is written down as not checked.** No third option.

Four things follow from that, and they are the whole of this ADR.

### 1. The checks run on every change, not on release day

There was one workflow and it ran on tags. So the tests ran *after* the work,
and a pull request from a stranger arrived with nothing having looked at it —
which also meant a stranger could not see a green tick before deciding whether
to trust the project.

`ci.yml` now runs on every push and every pull request: formatting, vet, the
whole suite under the race detector, a build for each of the five systems it
claims to build for, `govulncheck`, and the `.deb` built and put through
`lintian`.

### 2. A download can be tied to the source it came from

There is no code-signing certificate and there will not be one soon: an Apple
one and a Windows one cost money every year and identify a company. That left a
real gap — a checksum proves a file matches a published number, and nothing
proved the number came from here.

Every released file now carries a **provenance attestation**: GitHub signs a
statement binding the file to this repository, the workflow that built it and
the commit, using a key nobody here holds. Anyone can check it with
`gh attestation verify`, with no account and nothing installed.

It costs nothing, needs no secret, and is in one way better than a certificate:
it names the commit, not just the publisher.

### 3. The build is the same build

A checksum is worth less than it looks if the build is not reproducible —
"check what you downloaded is what was built" assumes there is one thing that
was built. There was not: the changelog carried the moment the script ran and
`dpkg-deb` wrote whatever mtimes the staging directory happened to have, so one
commit produced a different package every time.

Every timestamp the build writes now comes from the commit's own date
(`SOURCE_DATE_EPOCH`). Two builds of one commit produce an identical file,
verified, and CI builds it twice and fails if they ever differ.

And then the harder half, which is the one that matters: the same commit built
on a *different* machine. It was compared, it differed, and the difference was
chased down twice more. The toolchain — `go-version: 1.26` installs the newest
patch, so the runner had go1.26.8 and the desktop go1.26.6 — now comes out of
`go.mod`, which already says which toolchain this is built with. Then the
umask, which `mkdir` obeys and `dpkg-deb` records, so the package carried a
permission bit belonging to whoever ran the script.

With those closed, a GitHub runner and the maintainer's desktop build the same
commit into a byte-identical package. A stranger can reproduce a release
rather than trust its checksum, which was the point of the exercise.

Still depends on the build machine's Debian libraries, which decide the
`Depends` line — a different Ubuntu release legitimately differs. Said in the
documents rather than glossed.

### 4. Where it connects is a list, and the list is enforced

The most suspicious thing about a local-first program is the thing you cannot
see: what it talks to. `internal/outside` is every address written into the
program with a reason beside each, and the test beside it reads every line of
the source and fails on an address that is not on the list.

That is not a firewall and the package comment says so. It does not cover
addresses the owner types, or a web page they ask it to read. What it does
cover is the thing people are actually afraid of: a call to somewhere the
program's author controls, added quietly. That cannot now happen without a
commit that also writes down what it is for.

It sits beside a control that *is* enforced at runtime:
`internal/preflight/fetch.go` permits a fixed set of download hosts and
re-checks every redirect hop, because the first hop being allowed says nothing
about the last.

## What was rejected

- **A security badge or a score.** Scorecard and the rest measure whether files
  exist. Adding the files to raise a number, rather than because each one does
  something, is exactly the kind of assertion this ADR exists to stop.
- **Promising a response time.** SECURITY.md says seven days because that is
  what one person can honestly promise. An hour would look better and would be
  a lie the first time somebody is on holiday.
- **Claiming an audit.** The documents in `docs/audit/` are the maintainer's
  own. They are careful and they are not independent, and both halves of that
  are now said in the README and in SECURITY.md.
- **Signing with a self-generated key.** A key nobody has any reason to trust
  proves only that whoever holds it signed something. The attestation is worth
  more precisely because the signer is not us.

## How it could be wrong

The provenance chain trusts GitHub. If the workflow file itself is changed by
somebody who should not have changed it, the attestation faithfully records
that a compromised workflow built the file. Branch protection on `main` and
review on the workflow files is what would close that, and neither is in place
yet — this is written here so it is not mistaken for solved.

The connection list checks the source, not the binary. A host assembled at
runtime from pieces would not appear in it. That is a real limit, it is stated
in the package comment, and the reason it is acceptable is that the code doing
the assembling would still have to be written by somebody and read by somebody.
