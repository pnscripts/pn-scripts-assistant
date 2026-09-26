# Security

This program reads its owner's files, holds what it learns about them in a
database on their machine, and — with their permission — runs commands as
them. A fault in it is a fault in somebody's private life, not only in a
program. Reports are welcome and they are read.

## Reporting a vulnerability

**Do not open a public issue.**

Use GitHub's private reporting: **Security → Report a vulnerability** on
[this repository](https://github.com/pnscripts/pn-scripts-assistant/security/advisories/new).
It is private between you and the maintainer, and it is the preferred route
because the conversation, the fix and the advisory stay in one place.

If that is not available to you, write to **petar.v.nikolov@gmail.com** with
`pn-scripts-assistant` in the subject.

What helps, in rough order of usefulness:

- what an attacker gets, stated plainly — read a file, run a command, read the
  database, reach it over the network;
- how to reproduce it, ideally as a failing test or a `curl` command;
- the version (`pn-scripts-assistant version`) and the system.

You will get a first reply within **seven days**. This is one person's project,
not a company with a rota, and that is said here rather than promising an hour
and missing it.

## What is in scope

Anything that breaks one of these, which are what the program claims about
itself:

| Claim | Where it is enforced |
|---|---|
| It listens on loopback unless its owner deliberately opens it | `internal/brain/server` — `TestListenRefusesNonLoopback` |
| A web page the owner has open cannot reach it | `internal/brain/server/guard.go` — `Origin` and `Host` are both checked before routing |
| A model cannot act by itself: an approved action runs the arguments that were *stored and shown*, never anything supplied with the approval | `internal/brain/brain/approvals.go`, `internal/brain/store/proposals.go` |
| Work on a project is confined to that project's folder, by the kernel | `internal/brain/sandbox/landlock_linux.go` — `Restrict` |
| What may leave the machine follows the privacy mode, and a provider outside it is not even probed | `internal/brain/llm/privacy_test.go` — `TestRouterRefusesForbiddenProviderEvenWhenAskedDirectly`, `TestForbiddenProvidersAreNotProbed` |
| Credentials are not written into logs, conversations, evidence or the database in the clear | `internal/brain/redact` |

## Everywhere it can connect to

A program that reads your files and calls itself local-first is making a claim
you cannot check by reading this page. So the list is in the source, with a
reason beside each entry:
[`clients/internal/outside/outside.go`](clients/internal/outside/outside.go).

A test beside it reads every line of the program and fails if an address
appears that is not on the list. A telemetry endpoint added quietly is not
something that can happen here without somebody also writing down what it is
for.

There is no telemetry, no analytics and no call to anything belonging to this
project. Everything on that list is one of: something you asked it to install,
a documentation site it reads while working, a web search (never in `private`
mode), or a model provider you configured with your own key (only in `open`
mode).

Downloads are checked again at the moment they happen, not only in that list:
`internal/preflight/fetch.go` permits a fixed set of hosts and re-checks
**every redirect hop**, because a release download hands off to a content host
and the first hop being allowed says nothing about the last.

What that list does not cover, stated because it matters as much: addresses
*you* type — your Ollama, your mail server, a paired device, an MCP server, a
smart home — and web pages you ask it to read. Those are the program doing as
it is told.

The trust model these come from is written out in
[docs/audit/security.md](docs/audit/security.md), including who is trusted with
what and how each line is enforced. If you think a row of that table is wrong,
that is a report worth making too.

## What is not in scope

- Somebody with an account on the machine, or physical access to it. This is a
  single-user desktop program; it does not defend its owner against themselves
  or against a person at their keyboard.
- The model being wrong, rude, or confidently mistaken. That is a bug, not a
  vulnerability — open an issue.
- Ollama, the models, and the hosted providers. Report those to them.
- A setting that is dangerous and says so, used deliberately. "Act freely" runs
  commands without asking because somebody chose that; it is documented and it
  is not a vulnerability.

## Which versions are supported

The latest release. This is a young project with one maintainer, and a claim to
support older versions would not be true.

## What is honestly not there yet

Written here rather than discovered by somebody who assumed otherwise:

- **Nothing is signed with a certificate.** No Apple Developer certificate, no
  Windows code-signing certificate, no Debian archive key. macOS and Windows
  both refuse the first open and say why; the way past each is in
  [installing.md](docs/guide/installing.md).
- **Provenance is offered instead**, which costs nothing and is in some ways
  better: every released file carries a signed statement binding it to this
  repository, the workflow that built it and the commit it was built from.
  Check any download with no account and nothing installed but the GitHub CLI:

  ```bash
  gh attestation verify <file> --repo pnscripts/pn-scripts-assistant
  ```

  Alongside it, `SHA256SUMS` is published with every release.
- **Builds are reproducible on one machine, not yet across two.** Two builds of
  one commit produce an identical `.deb`, and CI proves it on every push. The
  same commit built on somebody else's machine has not been compared with ours,
  which is the version of this claim that would actually let a stranger check
  our work.
- **No third party has audited this.** The audits in `docs/audit/` are the
  maintainer's own, written to a fixed vocabulary that distinguishes what was
  measured from what was merely implemented. They are honest, and they are not
  independent.
