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
- **The list of programs: dpkg only for Linux packages, nothing on macOS.**
  It reads menu programs, snaps and Flatpaks on any Linux, and packages
  through `dpkg-query` (Debian, Ubuntu); rpm and pacman packages are not
  listed yet. On Windows it reads the Start menu, the installed programs
  (Settings, Apps) and the app packages installed for the user; app packages
  installed only for another user are not listed, and Start menu shortcuts
  more than four folders deep are not read. A folder that does
  not exist when it starts (for example the Flatpak folders before the first
  Flatpak is installed), or one the system refuses to watch, is looked at
  every 10 minutes instead of at once. Menu entries more than three folders
  deep are not read.
- **Choices per program: three kinds of use.** A program's card decides
  starting it, typing and clicking in its windows (the window under the click,
  or the one in front for typing, read from X11; on Wayland and Windows the
  window's program is not known, so typing and clicking are asked) and running
  it as a command (the program the command names; a program started by a
  script file or by another program is not followed). A command is read
  through `env`, `nohup`, `setsid`, `nice`, `ionice`, `timeout`, `stdbuf`,
  `sudo`, `doas`, `pkexec`, `xargs`, `busybox`, `script -c`, `systemd-run`,
  `eval` and a shell given a line (`sh -c "..."`). A "never" reaches every
  command of such a line, the lines inside it too, and every program a
  one-line script names (`python3 -c`, `perl -e`, `node -e`, `php -r`); it
  does not reach what a script file starts, a name the line puts together
  while it runs (from a variable, or in pieces), or a command handed to
  `find -exec` and the like. "Always" answers only for the program itself, run
  plainly: the file its name runs here (the first one found where programs are
  looked for, your own folders such as `~/.local/bin` included). Run through
  any of those wrappers, or as administrator, a program can be refused by its
  card and is never allowed by it, so it is asked about. A file elsewhere that
  only shares the name (in `/opt` too) is asked about and is not shown under
  the program's name, and a menu entry that starts its program through another
  one (`flatpak`, `python3`, `sh`, `env`, `java` and the like) is known by its
  menu entry, Flatpak or snap, not by that other program's file. An "always"
  on a menu entry, which has no version, is given to its Exec line and to the
  file that line really starts (where it is, its size and its times; its
  contents are not read): after an update, a changed Exec line, or a
  same-named file put earlier where programs are looked for, it is asked
  again, and an "always" given by 1.1.0 is asked about once. On a snap, a
  Flatpak or a package it is asked again after an update. A Windows program
  that states no version has nothing of the kind to follow yet, so its
  "always" stays. Typing approved at the desk goes to the window in front at
  that moment; a "never" on that window's program still refuses it. A
  package's choice is found through `dpkg-query -S` only when some package has
  one. A question about a program offers "Always for <program>" only when the
  program is known (a window or command it cannot place offers the tool-wide
  "Always" alone). Scrolling is not tied to a program.
- **"Stop using other programs" covers the program tools.** It stops
  opening, typing, clicking, scrolling and running commands, as administrator
  too, and none of these is started in the background. It does not
  stop the assistant's own helpers (reading the screen, recognising text,
  its own browser), MCP servers that are already approved, or a coding agent
  already at work: stop those on the Organisation page.
- **Other programs asking the assistant: four questions, this computer
  only.** `pn-scripts-assistant mcp` (for Claude Code, Claude Desktop,
  Cursor) answers whether the assistant is running, which programs are
  installed, a question to the assistant itself (in words, by the model on
  this computer only, one at a time, with no tools: it does nothing for the
  program) and a search of what it remembers (a few short pieces a search).
  No file, mail or conversation is handed over as such, and nothing that
  changes anything; what was remembered from them can be, and a remembered
  thing can name the file it came from. An answer that takes longer than
  most of a minute is fetched by the asking program with a ticket; an
  answer nobody fetches is dropped after 15 minutes or when the next
  question is taken. The ticket is all that guards an answer: any program
  on this computer that has it can fetch it. The record keeps the question
  and the first 600 characters of the answer. With no model on this computer a question is not answered, and
  the memory cannot be searched. At most 30 questions a minute are
  answered, from all programs together; permission questions (below) are
  counted by themselves, at most 12 a minute, so neither kind uses up the
  other's. Both numbers are for all programs together: a program that asks
  one kind in a loop makes the next program's question of that kind a
  refusal for that minute. It needs the assistant
  running, and asks it only on this computer's own address
  (`--api http://127.0.0.1:<port>` when the port was changed; the address in
  an encrypted settings file is not read). It passes on only what carries
  the running assistant's seal. When the assistant starts it keeps a key in
  a file that only your account can read (`answers.key` in its settings
  folder: `~/.config/pn-scripts-assistant` on Linux,
  `%AppData%\pn-scripts-assistant` on Windows,
  `~/Library/Application Support/pn-scripts-assistant` on a Mac), and seals
  each answer with
  it together with the question it answers; so a program that listens on
  the assistant's port while the assistant is not running is not believed,
  and cannot answer a permission question "allow". This does not stop a
  program running under your account: it can read that file, as it can
  read your other files. On Windows the file is as private as that folder
  is. An assistant that was already running when it was updated seals
  nothing until it is closed and started again, and until then other
  programs are told that what answered could not be shown to be the
  assistant. The same is said for good when the asking program starts
  `pn-scripts-assistant mcp` under another account, or with another
  settings folder than the assistant's (a program packaged in its own
  confinement may): it cannot read the key there. What it answers is passed by the
  asking program to the model it uses, usually a service on the internet.
  Everything but whether it is running is off until you allow it on the
  Programs page, each thing by its own switch.
  Every question is on the record with the name the asking program gave;
  that name is the program's own word and nothing checks it. The switch
  and the record are for programs that ask through `pn-scripts-assistant
  mcp`: a program running under your account on this computer can reach
  the assistant's own address, so it can ask under any name, or read the
  list of programs or search the memory there. It cannot turn a switch:
  the switches are set only in the assistant's own window (see "Answers on
  cards and the switches" below). Every
  change of the switch is on the record. On Windows give the program's
  full path in the entry.
- **Claude Code's permission questions: asked through the assistant, only
  for runs without a window.** With the switch "Their own permission
  questions, put to you here" on, Claude Code started as `claude -p ...
  --permission-prompt-tool mcp__pn-scripts-assistant__permission_prompt`
  puts each permission question on a card in the assistant's window, and
  only the answer given there goes back; the assistant answers none, a question
  nobody answers in ten minutes is a no, at most three wait at once. The
  card shows what Claude Code asks to use (the command, the file, the
  address), not what it will write into a file, and a command over 1,500
  characters is shown cut; a line break in a command is shown as ↵ and
  whatever else the tool would be given is named, not shown. A question
  refused because three already wait, or because of too many in a minute,
  is on the record as refused. Nothing is said out loud when a question
  arrives: it appears under "Waiting for you" within a few seconds. The
  questions Claude Code wants to ask you itself are not carried. It was
  built from Claude Code's documentation and from what its 2.1.303 release
  hands a permission tool, and tested with a stand-in for it and with the
  MCP project's own client; a signed-in Claude Code session was not part of
  the tests. As with every question from another program, the name on the
  card is the program's own word.
- **Answers on cards and the switches for other programs: only from the
  assistant's own window, and what that does not stop.** When the window
  opens it is given a key that lasts until it closes. Without that key
  these are refused when they come from this computer: an answer posted to
  a card (Approve, Reject, Always for a program), a change of "What other
  programs may ask", a pairing code (asking for one and taking it back),
  and opening the assistant to the network or the internet. A wrong
  pairing code sent from this computer itself (its own addresses on your
  network included) does not spend the code on offer, and only three such
  guesses are looked at for one code: after them no code is read from this
  computer, the right one included, until you ask for a new code, so a
  program here can still get in the way of pairing through this computer's
  own address (a forwarded port), not of pairing a device elsewhere. "From
  this computer" is what arrives from its own addresses; a program in a
  container with a network of its own arrives like another device. A wrong
  code from another device on
  a network you opened the assistant to still spends the code, so somebody
  on that network can keep a device from being paired for as long as they
  keep guessing; ask for a new code, or close the assistant to the
  network. A device paired with every right can still ask for a code and
  take one back. A card another program put there (its permission
  questions among them) is answered in that window only, not in words to
  the assistant and not from a paired device; that includes Claude Code.
  Not stopped: a program running under your account that types and clicks
  in the window as you would, that works the window through UI automation
  as assistive technology does (the assistant's own Windows check ticks a
  switch that way), that reads the memory or the files of the window, or
  that starts the window itself or replaces or reaches into its web view
  (the window keeps the key in its session storage, which some web
  views hold in their profile folder until the window closes). The record
  therefore says a question was answered "on its card", not "by you".
  Everything else at the assistant's address is open to programs on this
  computer as before: a conversation, the settings, what the assistant may
  do without asking. So a program can still talk to the assistant as you do,
  and have it approve its own waiting cards in words ("approve everything
  waiting") or change what it asks about; the key protects what other
  programs ask, not the assistant's own actions. In a browser instead of
  the window (`pn-scripts-assistant serve`, macOS, a build without a
  window) the page shows everything and answers no card, until that browser
  tab is opened by a link that works once. Started in a terminal, the
  assistant prints the link there; whoever can read the terminal before you
  use it can use it instead, and the page then says the link no longer
  works. Started any other way it prints none unless asked with
  `-desk-link`, and the link is then wherever that output is kept. On
  Windows, when the window cannot be drawn and the browser is opened
  instead, the link is handed to the browser on its command line. A device
  paired with every right answers the assistant's own cards with its token,
  as before.
- **MCP servers from other programs: four programs, their own settings
  only.** Claude Desktop, Claude Code (`~/.claude.json`, or the folder
  `CLAUDE_CONFIG_DIR` names), Cursor (`~/.cursor/mcp.json`) and VS Code (the
  default profile's `mcp.json`) are read. Project files (`.mcp.json`,
  `.cursor/mcp.json`, `.vscode/mcp.json`), other VS Code profiles, VS Code
  Insiders and VSCodium, Claude Desktop installed from the Microsoft Store
  (its settings live inside the package), and the Windows on-device agent
  registry (`odr.exe`, a preview) are not. A server written to start through
  a shell (`cmd /c npx …` is common on Windows), with a variable only that
  program fills in, an environment file, a working folder, the SSE transport
  or several headers is listed with the reason and not imported. So is one
  whose settings would make it load other code (`NODE_OPTIONS`, `LD_PRELOAD`,
  `PYTHONSTARTUP` and the like), and one with a key or password written into
  its command or address: put the key in a setting (`env`) or a header there
  and it is kept sealed here. Every other setting is shown with its value on
  the card. An imported program is treated as one that may reach the
  internet, so it is held back while privacy keeps everything on this
  computer. Its command and the settings that are not secrets are kept as
  plain words in the integrations file.

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
- **Asking your own Claude** is tested against stand-ins for Claude Code
  and the Anthropic API on Linux and Windows. A real Claude Code (2.1.303,
  Linux, 14.11.2026) accepted exactly the way it is started (no tools, no MCP
  servers, only an empty folder's settings, its own system prompt) and was
  sent about 620 tokens for a one-line question, so none of your personal
  Claude Code files went with it; it has not been watched with a real Claude
  Code on Windows. A Claude Code whose configuration lives in a folder named
  by CLAUDE_CONFIG_DIR is seen as not signed in. Signing in or out shows
  within a minute.

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
  says it does not look yet. Goals that start themselves and the reading of
  places wait for the quiet hours and start in them by themselves, and so does
  a coding agent question held for them alone. The program checks for those
  questions at most every ten minutes, so one can start up to ten minutes
  after the window begins when the window was changed while it waited; it
  starts only while the program is running.

## Telling voices apart

- **"Answer only me" is not offered in this version.** The model that told
  voices apart (1.0.0 downloaded it when the feature was first turned on)
  was trained on recordings licensed for research only, so 1.0.1 withdrew it
  until there is one made only from recordings free for any use. Updating
  switches the setting off, and the assistant answers any voice that
  addresses it, as it did before the feature existed. Nothing is
  downloaded. The model, its runtime and a voice you taught it stay where
  1.0.0 left them and are not used; the Privacy page can forget the voice,
  and the files under the program's machine folder (`voice`) can be deleted
  by hand.
- **Documents 0.13 read only in part stay that way until they change.** 0.13
  could stop reading a long document in the middle. This version reads every
  document whole, but a document already read is read again only when it
  changes: save it again (any change) to have the rest of it read. The review
  queue 0.13 filled with readings of its own guides is tidied by itself, once,
  on the first start.

- **Two pieces of background work at once share one progress line.** When
  one finishes (for example a voice download while a folder is
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
- **The AI mark** is in the headers of every mail it sends
  (`AI-Disclosure: mode=ai-originated`) and in the properties of the PDF and
  Word documents it writes. Plain text, Markdown and spreadsheets have no
  place for it, and text you copy out of the page (a draft pasted into your
  own mail program, a translation) carries no mark: say it yourself where it
  matters.
- **Paired devices and the tunnel** work across one network and are tested
  there; using them across the internet has not been exercised. A device
  paired for talking can read and talk; changes are made at the computer.
- **One drive per brain.** A brain lives in one folder; when its drive fills,
  it is moved, not spread across drives.

## Languages

- The program and its built-in guides are in English and Bulgarian. Of these
  guides, installing, setting up, troubleshooting and this list are in both;
  the guides for developers and for making a release are in English only.
- With the page in Bulgarian, the menu and the pages behind it are in
  Bulgarian: Command centre, Models, System, Operations, The room, Teaching,
  Organisation, Tasks, Reaching it and Privacy among them, with the parts on
  What it runs on, the one-time recovery key notice and the status lines of
  the conversation. What still reads in English there is what the program
  shows as it came: model descriptions from the model library, the
  descriptions of capability packages and integrations, most jobs of the
  built-in classification below the ones the organisation chart uses, the
  names of programs and what they print about their own version. So do the
  details a tool or the model writes under a step, the rare error message
  from the server (shown after "That did not work:"), and, when no model
  answers, the plain list of facts the first introduction falls back to.
  Built-in skills are named in Bulgarian where they come with a Bulgarian
  description.
- Question cards are in Bulgarian on a Bulgarian page, around what they
  are about, which is shown as it will be used. Inside a card, what was
  written elsewhere stays as it came: the details of a proposed hire or
  project that the model wrote, facts a tool read (an error, a download's
  source), and the names of skills and programs. A question asked before
  1.1.0 is shown in English.
