# Known limits

Български: [known-limits.bg.md](known-limits.bg.md)

What this version does not do, or has not been seen doing, written down so
nobody has to find it out. Each item says what happens instead. Checked
against the code on 9 November 2026.

## Systems

- **Linux and Windows only.** Ubuntu 24.04 or newer and Debian with the same
  libraries, 64-bit Intel/AMD (`amd64`); Windows 10 or 11, 64-bit Intel/AMD.
  There is no macOS download and no Windows on ARM build. The source still
  compiles for macOS, where it shows its interface in the browser, untested.
- **Nothing is signed.** Windows SmartScreen stops the first run ("Windows
  protected your PC"); *More info → Run anyway* passes it once.
  [installing.md](installing.md) says more. A checksum and a GitHub provenance
  attestation are published for every file instead.

## On Windows

- **Listening and speaking aloud, echo cancellation, desktop control and
  kernel confinement are Linux-only.** Typed conversation, memory, documents,
  mail, research and the other tools work the same.
- **Confinement.** Without Landlock, work on a project may not run a command
  written into `cmd /c` or PowerShell (nothing below the program could hold
  what it writes to the project's folder), so it writes a script into the
  project and runs that. Deleting through `cmd` or PowerShell is asked about
  like any deletion.
- **Telling your voice from others' and reading full web pages in a browser**
  need the Linux build. Plain page fetching works.
- **The window needs Microsoft Edge WebView2**, which Windows 11 has and almost
  every Windows 10 has. Without it the assistant opens in your browser and
  works the same.
- **Moving the window to a screen with different scaling** resizes it to what
  Windows suggests for that screen, and minimising it tells the page it is not
  seen. Both are proved on a Windows runner by sending the window the messages
  Windows sends; neither has yet been watched by a person on two real monitors.

## Processor

- **While you are using it**, the window animates and keeps itself current:
  on four cores without a graphics card that is about half a core for the
  program and its page, and a third of one more in the display server. A
  minute after you stop it rests at about 2 %, and minimised at about 0.5 %
  ([troubleshooting.md](troubleshooting.md) has the measurements).

## Models and providers

- **A local model needs Ollama.** Setup installs it on Linux; on Windows it is
  installed from ollama.com first. On a machine without a graphics card the
  first answer after a start can take minutes while the model loads.
- **Setup installs Ollama 0.40.11, not the newest.** Like the voice, the
  listening parts, Godot and Node.js, it is one fixed release checked against
  its SHA-256, so a model published after it may need a newer Ollama. An
  Ollama you install or update yourself is kept: setup never replaces a newer
  one with its own.
- **Hosted providers have only been tested against stand-ins.** All twelve
  are tested against fakes that answer like them; none has been watched
  answering with a real key. If one misbehaves, the error it returns is shown
  as it came.

- **Coding agents** that can be asked are Claude Code, Codex and Cursor's
  agent. Gemini CLI is not one yet: Google replaced it with Antigravity CLI for
  most accounts in June 2026, and its event stream is not documented well
  enough to read what it changed. Whether Cursor's agent is signed in is only
  known when it is asked, because asking starts its login.

## Safety and privacy

- **Goal mode** works on its own and asks only before paying, sending or
  publishing in your name, deleting outside the goal's folder, changing the
  whole system, and before opening a protected file (keys, passwords, the
  vault). While it is on, that applies to conversation turns too, not only to
  goal work.
- **Everything allowed** ("never stop") means exactly that: nothing is asked,
  protected files included. Everything is still recorded under *What I did*.
- **Web pages are fetched without a proxy** while the check that keeps them off
  this computer and the local network is on. Behind a proxy that is the only
  way out, plain page fetching fails; reading a page in the full browser still
  uses the system's settings.
- **Reading a page in the full browser** checks the address before it opens it,
  but the browser then looks the name up itself; a name that changes its
  address in between is not caught there. Plain page fetching checks the
  address it actually connects to.

- **Rules are checked by what an action is and the words it carries**:
  sending, posting, commits, changing files, paying and deleting, the names,
  addresses and words a rule lists, and the days it names. A condition it
  cannot see ("clients", "late at night") makes a "never" rule ask you
  instead of stopping on its own. Clicks and typing in other programs are
  read from their words only. Standing instructions are noticed in English and
  Bulgarian.
- **The last look before sending** is applied to every email the assistant
  sends. A draft you send yourself with the Send button on the Mail page goes
  as you see it; use *Last look before sending* on the Writing page first.

- **Looking after the computer** (leftover processes, stopping them, and
  whether a restart is safe) reads Linux's own records; on Windows the card
  says it does not look yet. Work is not yet moved to quiet hours by itself:
  the allowance of a paid coding agent is shown before each task instead.

## Telling voices apart, and updating from 0.13

- **The voice recognition runtime is replaced when the program first starts
  after an update**, in the background (about 11 MB), and only while *May look
  things up about itself* is on. Until it is done, voices are not told apart.
  With that setting off, or if the download fails, the Privacy page shows the
  part as missing and its button fetches it; pressed while a download runs,
  it says so and does not start another.
- **A voice recognition model fetched by 0.13 or earlier is checked the
  first time it is used after the update.** Those versions took it unchecked
  from a branch. If it is the published file nothing changes; if it is not,
  it is not used, voices are not told apart, and the Privacy page offers to
  fetch the published one.
- **"Answer only me" acts on nothing it cannot check.** While it is on and a
  voice cannot be recognised (the part above missing or being replaced, no
  voice taught yet, or a recording too short to judge), what it hears is not
  acted on. The command centre says why, the Privacy page shows it beside the
  switch, and typing works as always.
- **Documents 0.13 read only in part stay that way until they change.** 0.13
  could stop reading a long document in the middle. This version reads every
  document whole, but a document already read is read again only when it
  changes: save it again (any change) to have the rest of it read. The review
  queue 0.13 filled with readings of its own guides is tidied by itself, once,
  on the first start.

- **Two pieces of background work at once share one progress line.** When
  one finishes (for example the voice recognition download while a folder is
  being read), the line is cleared, and the other's progress shows again only
  with its next step. The work itself is not affected.

## The voices it speaks in

- **The neural voices are Cori (the robot, the default, a British woman), LJ
  Speech (the woman, American) and Norman (the man, American).** Each was
  trained from scratch on recordings in the public domain, so they may be
  used for anything. There is no British man's voice with such a lineage
  yet, so the woman and the man are both American, to match each other.
- **The voices 0.13 installed are no longer used.** They were fine-tuned from
  recordings licensed for research only. After the update the new voices are
  fetched when the program first starts (about 190 MB, in the background,
  only while *May look things up about itself* is on), and a voice chosen in
  0.13 becomes the robot. Until they arrive it speaks in the system voice;
  with that setting off, *Voice (speaking)* shows as out of date and its
  Update button fetches them. The old files stay on the disk until you remove
  the part, and are never spoken with.
- **Bulgarian is spoken by espeak-ng**, the system's own voice, as before.
  Piper's Bulgarian voice is not installed: it too was fine-tuned from the
  research-only recordings. A Piper voice you add yourself is offered as you
  add it, under its own licence.

## Mail, devices, storage

- **Mail** is read over IMAP with TLS (port 993) and sent over SMTP, with an
  app password. Signing in with a Google or Microsoft account (OAuth) is not
  offered. Nothing is sent without your yes to the exact words.
- **Paired devices and the tunnel** work across one network and are tested
  there; using them across the internet has not been exercised. A device
  paired for talking can read and talk; changes are made at the computer.
- **One drive per brain.** A brain lives in one folder; when its drive fills,
  it is moved, not spread across drives.

## Languages

- The program and its built-in guides are in English and Bulgarian. Of these
  guides, installing, setting up, troubleshooting and this list are in both;
  the guides for developers and for making a release are in English only.
- The *Your voice* card on the Privacy page is still in English on a
  Bulgarian page, apart from what it says when "Answer only me" cannot
  recognise your voice.
