# Changelog

What changed, in the terms somebody using it would notice. The full history is
in the commits; this is the part worth reading before updating.

Versions follow [semantic versioning](https://semver.org). While the first
number is 0, the second one changes when something visible changes.

## Unreleased

### Added

- **Learning, with the assistant as a tutor.** A new Learning page, and the
  same in the chat ("teach me fractions for a ten-year-old", "quiz me",
  "what should I revise?"). Name any topic and a level — child, beginner,
  intermediate or advanced — for a lesson in one clear shape: the goal, what
  to know first, small steps each with a worked example, common mistakes,
  questions to check yourself with their answers folded away, and what to
  learn next. Practice questions (choices, numbers, short answers) come one
  at a time without their answers. Answers are marked by rules wherever a
  rule can tell — a letter or the option's words, a number however it is
  written (12,5, 1/3, "120 cm"), a short answer whatever its capitals — and
  otherwise judged by the model against the expected answer, or by you when
  there is no model. Missed questions come back in minutes, right ones
  tomorrow, then in three days, a week and a month, until they count as
  learnt; the page lists every topic with its progress and what is due. A
  tutor specialist teaches with only these tools and reading, never writing.

## 0.6.0 — 2026-10-26

### Added

- **Writing and proofreading, in English and Bulgarian.** A new Writing page,
  and the same in the chat ("proofread this", "make it more formal"). The
  proofreading uses rules rather than a model — instant, offline, nothing
  leaves the computer — and every correction says why: doubled words,
  spacing, Latin letters hidden in Cyrillic words, в/във and с/със, ѝ, „не“
  apart from a verb, по-/най-, the full article after a preposition, the comma
  before че, защото, но and който, „…“ quotes; a/an, could of, more then, I.
  Sure corrections are ticked and made in one press; the rest are yours to
  decide. Choose a tone (formal, friendly, concise, persuasive, simple,
  polite) and a word limit to have them checked too, or have the text
  rewritten in that tone and see the changes before keeping them. Compare
  two versions word by word, on the page or with "what changed?".
- **Translation with your own glossary.** Translate a text, or a file into a
  new file beside it with its layout kept: Word and OpenDocument (styles,
  tables, headers, pictures), subtitles (timings kept), Markdown (code left
  alone) and plain text; the original is never replaced. Keep a glossary of
  agreed translations — add terms one by one, paste them from a spreadsheet,
  or save them as one — and every translation follows it and is checked: a
  term not translated as agreed, or a number, address or link that went
  missing, is listed with the result. A new Translation page, in English and
  Bulgarian, and a translator specialist for longer jobs.
- **Scans, photos and spreadsheets.** Scans and photos — a scanned contract,
  a photo of a receipt, a PDF with no text in it — are read on this computer,
  by Tesseract or by a local model that can see, and say they came from a
  scan. Spreadsheets are written as real .xlsx files (bold frozen header,
  numbers as numbers) and read back as rows with their numbers, every sheet
  by name; OpenDocument files are read without LibreOffice. Tesseract and the
  PDF tools can be installed from the list of what a task needs.
- **Mail, sorted, with replies that wait for your yes.** A new Mail page sorts
  new mail from its headers — suspicious first, then what needs a reply, then
  people, then newsletters and notifications by sender — from your mail
  server or from Thunderbird. Replies are drafted in the message's language
  and tone and kept waiting; each is sent only when you press Send on it, as
  it stands, and is recorded in What I did. Sending a draft from the chat
  always asks, even when everything else is allowed. Optionally, a one-line
  summary of new mail every morning.
- **Plan: what is next, to-do lists and your online calendar.** A new Plan
  page puts the diary, your online calendar, reminders and due to-dos in one
  time order, overdue first. Keep to-do lists by name, with an optional due
  day, ticked off in one click. Connect a CalDAV calendar (Nextcloud,
  Fastmail, iCloud and others) with an app password: the server is checked
  before anything is kept, its appointments appear under What is next, and
  new ones can be added — never changed or deleted. Also in the chat: "what
  is next?", "put milk on the shopping list".
- **Answers say where they came from.** Each answer lists its sources — things
  it remembers with the file they came from, files and web pages it read,
  search results — numbered, and a written answer ends each sentence that
  relies on one with its number, like [2]. A number that points nowhere is
  taken out before the answer is shown. Open a web source in your browser or
  a file's folder from the list; the list stays with the conversation.
- **What I did and why.** A new page lists every action, every action put to
  you and every refusal, with the request behind it, the specialist, the
  model and why it was chosen, what it was given and returned, and whether
  anything left the computer. Filter by period, by changes, or by what left.
- **Control.** The permissions page is now Control: every capability says
  whether it can send anything off this computer and what, has an on/off
  switch, and can be filtered; citing sources is switched here too. These
  pages, and the sources, are in English and Bulgarian, following the
  language you speak to it in.

- **Each step gets the model its work needs.** A job's steps are thought with
  the model their expert's work calls for — code, hard thinking, speed,
  pictures — chosen from what is installed at that moment, with the reason
  shown; a model that keeps failing gives way to the next.
- **One question at a time.** When it needs to ask, it asks one question with
  two to four answers to click, the recommended one first; reply with a
  number, a word or your own sentence.
- **Starting knowledge inside the program.** A new brain gets short guides in
  its knowledge folder (getting started, portable copy and USB, the scam
  guard, experts and models, trusted sources). Updates refresh the guides you
  have not changed and never touch the ones you have.

### Changed

- **Specialists for every field, and only when needed.** Business strategist,
  finance analyst, accountant, legal specialist, marketing specialist and
  music producer join the developers, writer, designer and researcher, each
  with only the tools its work needs.
- **Original and meaningful before finished.** Whatever a job produces — a
  game, a logo, a jingle, a story, a strategy, a contract draft — is checked
  by the specialists of the fields it touches (trademarks, look, sound, game
  rules, text and meaning, money, tax, law, business), side by side, and only
  for the sides it has; why each check was or was not used is in the task,
  and the findings have their own "Originality and meaning" section in the
  final report. New guides: "How I work" and
  "Originality and licences", in English and Bulgarian.
- **Works on goals on its own.** One setting, agreed to once: a goal is
  planned, staffed, provisioned and finished end to end. When no model here
  fits a step, the largest one that fits the computer's memory and disk is
  downloaded and its checksum verified, falling back to the next on failure;
  user-level tools are installed from their official sources with their
  published checksums; each goal that makes something works in a folder of
  its own; the report lists what was put on the computer. Only four things
  are still asked: a payment or a paid service, sending or publishing in your
  name, deleting files outside the goal's folder, and a change to the whole
  system. Only what it downloaded itself is ever offered for removal.
- **The goal-mode consent says what privacy does not cover.** Models, tools
  and integrations are downloaded from the internet even when privacy is
  "nothing leaves this machine"; the consent says so, and that those
  downloads send nothing about you. Settings show the same words the program
  enforces.
- **Integrations a goal needs are found and added on their own.** From the
  built-in catalogue first, then the official MCP registry — only from
  trusted publishers (add your own in integrations/trusted.json), pinned to
  one version from npm or PyPI, running on this computer and needing no
  account or secret, and only while privacy allows it. Each is reviewed
  before anything is downloaded, given to the one specialist that needs it,
  and the tools the server says destroy are not offered. The registry is
  searched with a short word for the kind of tool, never your words.
- **Unused downloads are removed on their own.** Once a day, models, tools
  and integrations that goal mode downloaded and nobody has used for 30 days
  are removed and the removal is proven; never anything you installed or set
  by name, and nothing while a job is under way. A failed removal is tried
  again the next day.
- **A goal is tested from the request to the report.** "Make me a space
  strategy game", with goal mode on and privacy set to this computer only,
  runs on a brain of its own against a fake model library and tool source:
  the plan, the specialists, a model and a tool downloaded and checked, the
  work in the goal's own folder, the originality checks and one report —
  with nobody asked anything.

### Fixed

- The diary said appointment times in UTC rather than local time, so an
  appointment at two in Sofia was read as noon and a whole day as the evening
  before.
- On a computer without graphics acceleration goal mode no longer downloads
  the largest model that fits in memory (minutes per answer); it chooses one
  that answers in reasonable time.
- When nothing here could write code, goal mode now downloads a model that
  can, instead of stopping the goal.
- The general assistant is no longer limited to a personal assistant's mail
  and diary tools, and a specialist is offered only the tools it may use, so
  steps no longer go round between specialists until they fail.
- A "strategy game" no longer brings a business strategist; music a plan
  makes brings the sound check even when the request never said "sound".
- Installs in goal mode wait for the result, are recorded and can be removed
  again; the report says work was done without asking because of goal mode.
- A data race when steps of one job ran side by side.
- **Originality checks with real tools.** Names are searched in trademark
  registers, app and game stores and the web; a game's one sentence is
  compared with the closest titles; pictures are compared with reference
  pictures you give (resized, recoloured and mirrored copies are found);
  melodies in MIDI files and code are compared, in any key, with well-known
  public-domain themes. Findings go into a report — item, aspect, finding,
  risk, action — saved as a table and a CSV sheet to download. "Audit the
  originality of <folder>" checks a whole catalogue, each item only for what
  it has, side by side; your own works are reported, never changed.
- **Fixed by itself.** When a check finds something in what the assistant
  made, the one who made it changes it and the same specialist checks again;
  the report says what was changed.
- **Plain wording on the screens.** The brain is now explained where its
  copies are kept: it is the assistant's knowledge base, everything it has
  learned, remembered and been told, in one folder. Team members are called
  specialists, learned ways of working are called procedures, and "ships
  with" became "included with".

- **A moved brain does not learn its own knowledge again.** Files inside the
  brain's folder are remembered relative to it (`@brain/knowledge/…`), and
  what was learned before is rewritten that way on the next start, so a brain
  on a USB stick or another computer keeps one copy of each fact.
- **Each expert runs only its own tools**, also when everything is allowed.
  A step that reaches for another expert's tool is handed, without asking
  you, to an expert who has it (a specialist first, the general assistant
  only when no specialist does).
- The built-in playbooks are only what every user needs: asking, planning a
  goal, mail, documents and the scam guard. Procedures specific to one
  business are no longer shipped in the program.

## 0.5.0 — 2026-10-23

It knows what your other agents know, carries its knowledge with it, and
looks twice at every message before you act on it.

### Added

- **Agent skills become playbooks.** On start and on every round of the
  learning, every `SKILL.md` under `~/.claude/skills` (and any folder listed
  in `BRAIN_SKILL_FOLDERS`) becomes a playbook: its description says when,
  its text (up to 6,000 characters) says how, with a pointer to the full
  file. A changed skill is refreshed; a skill you wrote or taught yourself is
  never overwritten — remove the `imported:` line from a playbook to make it
  yours.
- **A knowledge folder inside the brain.** `knowledge/` in the brain's folder
  is always learned, and is stored relative to the brain, so a brain on a USB
  stick or opened through Remote has the same documents on any machine.
  Imported skills are copied there with their references (never secrets,
  databases or caches), so their playbooks keep working where the agent is
  not installed.
- **A portable copy.** The Windows zip now runs without installing
  anything: the brain is kept in `PN-SCRIPTS-ASSISTANT-DATA` beside the
  program, so the folder works from a USB stick, and the computer it runs on
  does not remember it. On Linux, put a file named `PORTABLE` beside the
  program for the same. The installer and the .deb keep the brain in your
  account, as before.
- **A scam guard.** Every message `read_mailbox` and `read_email` read is
  checked — a sender domain that does not exist, look-alike brands and links,
  replies that go elsewhere, pressure combined with a request for passwords,
  money or access — and the verdict is written under it. Lists flag the
  obvious ones from the headers. A new `scam_guard` playbook says what to do
  next. Domains are looked up only when privacy allows the web. Mail read
  over IMAP brings its Reply-To along, and the addresses behind the links of
  an HTML message are listed under its text, so a link hidden behind
  "Verify your account" is checked too.

### Security

- Built with Go 1.26.10, which fixes GO-2026-6809 in `crypto/x509`
  (found by govulncheck in CI on 2026-10-23).

## 0.4.0 — 2026-10-11

Everything is allowed out of the box and still recorded, what it keeps about
you is encrypted at rest, it already knows how the work here is done, and the
first meeting is a conversation. See ADR 0007 and ADR 0008.

### Before you update — read this

- **The first start encrypts your brain, and shows a recovery key once.**
  Write it down or save it as a file before you press "I have kept it"; it is
  never shown again. Without it, the brain cannot be opened on another
  computer, or on this one if its keyring is lost.
- **Older versions cannot read an encrypted brain.** There is no way back to
  0.3.0 with the same data once 0.4.0 has started on it. The copy taken before
  sealing (`brain.sqlite.before-sealing.pnsealed`) is itself sealed, so if you
  might want to go back, copy the brain's folder somewhere safe before the
  first start.
- **Everything is allowed by default**, including on an existing install,
  which is switched over once. If you want the old limits, set
  `BRAIN_CONFINE=1` before you start it.

### What you will notice

- **Everything allowed, still recorded.** Privacy starts at `open`, nothing is
  asked before it acts, and nothing is confined: installing, sending mail,
  hiring, integrations, work outside a project, protected files, local
  addresses. Claude Code and Codex run with their own permission checks off.
  An existing install is switched over once, on upgrade. `BRAIN_CONFINE=1` puts
  every limit back. Every tool call is still written down, and a tool you
  refused by name stays refused.
- **Twelve built-in playbooks**, versioned, offered when their words come up:
  building and publishing HTML5 games on itch.io and Game Jolt, building
  products, shipping pnscripts.com, WordPress.org review, social profiles,
  naming, the sales hold, GitHub costs, machine load and the AI brain. A skill
  of your own with the same name wins.
- **It can drive a Chrome of its own** (`drive_browser`), signed in once by
  you, and **read Thunderbird's mail folders** (`read_mailbox`).
- **Encrypted at rest.** Conversations, memories, the record of every tool
  call, the profile, the voiceprint, credentials, your name and the log are
  sealed so only the program can read them; the key is in this computer's
  keyring. An existing brain is sealed in place on the first start, with a
  proved sealed copy first and safe against a power cut. You are shown a
  recovery key once; on another computer the brain opens with it.
- **More room to work.** Commands may run for three hours, a turn may take
  thirty steps, a task four hours; bigger downloads and models install
  without asking; root is asked for through the system's own password dialog
  (`run_as_admin`).
- **No naming form and no greeting formula.** The first meeting is a
  conversation; your name, its name and your language are picked up from what
  you say.
- **The copyright holder is the company**, ПН СКРИПТС ЕООД (PN Scripts),
  in the licence, the package and the Windows installer. Security reports and
  the package's maintainer address go to office@pnscripts.com. The licence is
  still MIT.

### Smaller fixes

- Built with Go 1.26.9, which closes twelve vulnerabilities govulncheck
  found in the standard library of 1.26.6 (`net/http`, `net/textproto`,
  `crypto/tls`, `html/template`). Nothing in this code changed for it.

## 0.3.0 — 2026-10-06

Two settings that had quietly become one are two again, the Models view says
something true about every model, and mail goes to the server that sends it.

### What you will notice

- **The permission level and privacy are independent.** Loading `brain.conf`
  used to raise the permission level to match privacy, so somebody who chose
  "ask every time" with privacy open was acting freely again after a restart.
  Setup coupled them the other way. Now each is read as written, and setup
  asks about privacy separately. The interface and the documentation say what
  each one actually does: in open mode the memory recalled for a turn goes to
  the hosted provider with the messages; the database itself stays here.
- **The Models view lists sizes, not "undefined".** Each model is one row with
  every size it is published in — "270m", "8x7b" and "e2b" included — and what
  each wants in memory and how it will run on this machine. A model opens on
  the largest size that runs comfortably here rather than the largest that
  merely fits. Model names are shown as typed, not capitalised.
- **Mail goes through the outgoing server.** With the outgoing server left
  blank, mail went to the reading server and failed. An incoming server named
  `imap.<domain>` now sends through `smtp.<domain>`; anything else asks for the
  outgoing server in Settings. A server that accepts and then says nothing no
  longer holds a tool call forever, and every message carries a Message-ID.

### Smaller fixes

- The naming card no longer saves an empty owner under a placeholder name.
- A reminder with nobody named begins "A reminder:", not ", a reminder:".
- The README and installing guide no longer name a version that was never
  published.

## 0.2.2 — 2026-09-27

Every action in both workflows on its current major, and pinned to a commit.

These four were held back on purpose: they run only in the job that publishes
a release, which nothing but a real release can exercise, and merging an
untested change into the path that signs and publishes downloads is how a
release day goes wrong. They go in here because this release is the thing that
tests them — `download-artifact`, `action-gh-release` and
`attest-build-provenance` all ran to produce the files beside this note.

`main` also holds admins to its checks now. It did not, and the consequence
was not theoretical: a commit that failed CI reached `main` because the person
pushing it could bypass the rule. A protection with a hole in it for whoever
is most likely to be in a hurry is not much of a protection.

## 0.2.1 — 2026-09-27

Six jobs had been uploading nothing and passing.

The first release exposed it, which is the only thing that could have: the
publish job went looking for five sets of artifacts and found three. The CLI
and the doctor, for all five systems, and the Linux desktop binary, were built
with `working-directory: clients` and written to `../dist` — the repository
root — while the upload step looked in `clients/dist`. It found no files, said
so in a line nobody reads, and reported success.

`if-no-files-found: error` on every upload in the workflow now, which is the
half of this fix that matters. The path was one wrong word; a step that can
find nothing and call it a success is a shape that will be wrong again.

0.2.0's downloads were complete for anybody installing it — the package, the
disk images and the Windows installer were all there. What was missing is the
command-line tools beside them.

## 0.2.0 — 2026-09-27

The first release anybody outside this machine can download.

It is a large one, and the reason is in the version number below: 0.1.0 was
prepared — it has notes, it has a package — and was never published. There was
no tag and no release, so the README pointed at an empty page. Everything
built since then arrives here at once.

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
- **The package is byte-for-byte reproducible**, checked between two machines:
  a GitHub runner and the maintainer's desktop build one commit into the same
  file. Three things had leaked into it — the clock, the Go patch version and
  the builder's umask. CI builds it twice on every push and fails if they ever
  stop matching.
- **Every released file carries a signed provenance attestation**, so a
  download can be checked against the workflow and commit that built it.
- **`SECURITY.md`, `CONTRIBUTING.md`, a code of conduct** and issue templates,
  because a project that reads people's files should say how to report a fault
  in it before somebody has to ask.

## 0.1.0 — prepared, never published

The first version that could be installed rather than built: an Ubuntu
package, a native window, and a brain that remembers. It was built and
checked, and no tag was ever pushed, so it never reached a releases page.
Kept here because the work is real and 0.2.0 stands on it.

The full notes are in
[docs/RELEASE-NOTES-0.1.0.md](docs/RELEASE-NOTES-0.1.0.md).
