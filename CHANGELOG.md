# Changelog

What changed, in the terms somebody using it would notice. The full history is
in the commits; this is the part worth reading before updating.

Versions follow [semantic versioning](https://semver.org). While the first
number is 0, the second one changes when something visible changes.

## Unreleased

Everything below is on `dev` and not in a release yet. It is a lot, because
0.1.0 was the first version that could be installed at all and this is the
work that made it something to keep using.

### It can do more

- **An organisation rather than a team.** Jobs, seats and agents who occupy
  them; work goes to a specialist, up to a manager, or past a reviewer. Hiring
  happens from a sentence, from a template, or because a task needed somebody.
  The job catalogues behind it are the real ones — ESCO and O\*NET — fetched by
  setup and read in by the brain itself.
- **Tasks that finish**, with goals, a diary, and jobs that run in the
  background; every task and step says how serious it is, and a critical one
  asks through a grant rather than a prompt.
- **It can see the machine it runs on** — one list of what is installed, what
  is missing, what needs signing in to — and says what it can do from that
  list rather than from a paragraph written months ago.
- **It hires, equips and confines**, and decides for itself how each piece of
  work is done: Claude Code or Codex on your subscription, or a model here.
- **A real browser, pictures, a phone paired in person**, and reaching the
  brain from outside the house when you deliberately open it.
- **A voice that knows when to speak**, how loud the room is, and who is
  speaking.
- **Three voices that make three sounds**, and every system voice choosable by
  name — a robot, a woman, a man, and each underlying variant behind them.
- **The interface stands in one scene** and goes round without a menu.

### It is more honest about itself

- **Everything it does is said once, by the assistant** rather than by a string
  written into the program. The introduction, the help, the first run and the
  greeting are all decided from what is actually installed.
- **Everything is allowed by default**, and the switch that used to quietly
  change privacy when you changed permissions no longer does. They were one
  setting pretending to be two.
- **The tools a turn is given are chosen**, not filtered — the model was being
  sent more schema than it could read.

### Faults fixed

- **Any web page you had open could drive this.** A page in your browser could
  reach the assistant and change things. Both the `Origin` and the `Host` are
  checked before a request is routed now.
- **The model was reading a third of what it was sent.** The context window
  was smaller than the prompt, silently, so the beginning of every long
  conversation was dropped.
- **An answer that took 28 minutes was thrown away at 15.** The work finished;
  the connection had been cut. The three requests that wait on a model set
  their own deadline now.
- **A newer build looked older to `apt`**, so upgrades were refused as
  downgrades. Versions are ordered by a timestamp rather than by a commit hash.
- **Turning the room down could leave a program quiet for good.**
- **Twenty-eight vulnerabilities in the Go toolchain**, fixed by raising it;
  and requests to a model now retry rather than failing on the first refusal.
- **A build without cgo chose a different game engine** — the same source
  behaving differently depending on how it was compiled.

### The product around the program

- **Installers for every system**: the `.deb`, a macOS `.dmg`, and a Windows
  installer, each built on the system it is for.
- **CI on every push and pull request** — formatting, vet, the suite under the
  race detector, a build for each system, `govulncheck`, and the package put
  through `lintian`.
- **The package is byte-for-byte reproducible.** Two builds of one commit
  produce the same file, and CI fails if they ever stop doing so.
- **Every released file carries a signed provenance attestation**, so a
  download can be checked against the workflow and commit that built it.
- **`SECURITY.md`, `CONTRIBUTING.md`, a code of conduct** and issue templates,
  because a project that reads people's files should say how to report a fault
  in it before somebody has to ask.

## 0.1.0

The first release that could be installed rather than built: an Ubuntu
package, a native window, and a brain that remembers.

The full notes are in
[docs/RELEASE-NOTES-0.1.0.md](docs/RELEASE-NOTES-0.1.0.md).
