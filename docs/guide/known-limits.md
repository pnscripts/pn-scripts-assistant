# Known limits

What this version does not do, or has not been seen doing, written down so
nobody has to find it out. Each item says what happens instead. Checked
against the code on 31 October 2026.

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
  written into `cmd /c` or PowerShell — nothing below the program could hold
  what it writes to the project's folder — so it writes a script into the
  project and runs that. Deleting through `cmd` or PowerShell is asked about
  like any deletion.
- **Telling your voice from others' and reading full web pages in a browser**
  need the Linux build. Plain page fetching works.
- **The window needs Microsoft Edge WebView2**, which Windows 11 has and almost
  every Windows 10 has. Without it the assistant opens in your browser and
  works the same.
- **Moving the window to a screen with different scaling** does not adapt it
  to that screen's scaling yet.

## Models and providers

- **A local model needs Ollama.** Setup installs it on Linux; on Windows it is
  installed from ollama.com first. On a machine without a graphics card the
  first answer after a start can take minutes while the model loads.
- **Hosted providers have only been tested against stand-ins.** All twelve
  are tested against fakes that answer like them; none has been watched
  answering with a real key. If one misbehaves, the error it returns is shown
  as it came.

## Safety and privacy

- **Goal mode** works on its own and asks only before paying, sending or
  publishing in your name, deleting outside the goal's folder, changing the
  whole system — and before opening a protected file (keys, passwords, the
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

- The program and its built-in guides are in English and Bulgarian. These
  guides (installing, configuring, troubleshooting) are in English.
