# The programs on this computer

Open **Programs** in the menu. The assistant keeps a list of what is
installed here and keeps it current by itself: a program installed, updated
or removed shows up within seconds, without anybody asking and without
looking all the time. You can also ask: "which programs do I have?", "is
GIMP installed?", "what was removed this week?".

## Where it looks
On Linux:
- **Menu programs**: the programs a person starts from the menu.
- **Packages**: the system's packages (asked of dpkg-query on Debian and
  Ubuntu).
- **Snaps** and **Flatpaks**: both read from their own folders.

On Windows:
- **Menu programs**: the shortcuts in the Start menu, yours and everybody's
  (shortcuts to documents, help files and uninstallers are left out).
- **Installed programs**: the list Windows shows under Settings, Apps
  (without parts of Windows and updates).
- **App packages**: the Microsoft Store and other MSIX apps installed for
  you (without the frameworks they use and parts of Windows).

The page says how the list stays current: how many places (folders, and on
Windows the registry keys that list programs) are watched, and any that is
looked at every 10 minutes instead (one that is not there yet, or one the
system would not let it watch).

## What changed
The last 14 days: what was installed, updated (from which version to which),
removed, or installed again after a removal, newest first. What was already
here the first time it looked is not news and is not listed.

## Every program
Pick one of this computer's sources (**In the menu**, **Packages**,
**Snaps**, **Flatpaks** on Linux; **In the menu**, **Installed programs**,
**App packages** on Windows) or **Removed**, and type part of a name. A long list shows the first 200 and says so.

## A program's card
Press a program to see what was found about it: its kind, its own name,
version, where it lives, what it starts, when it was first seen (or that it
was already here at the first look) and what last happened to it. The card
also says what the assistant does with it today: it knows the program is
there and tells you about it when you ask; it does not start, change or
remove it. Using programs for you comes in a later version, and it will ask
you first, at this computer.

## MCP servers you set up in other programs
If you already use MCP servers in **Claude Desktop**, **Claude Code**,
**Cursor** or **VS Code**, the assistant finds them by itself and lists them
on the **Organisation** page, under **Integrations**, as "Found in your other
programs". It reads only each program's own settings file for you (Claude
Code: your own list and the lists it keeps per folder); the files that come
with a project folder are not read, because they come with whatever was
downloaded.

Nothing found is used until you press **Import** and then approve it, like
any integration. A key in those settings is kept sealed and never shown.
An imported server follows its entry there:
- removed there (or the program's settings deleted): it stops here at once,
  its tools are no longer offered, and it is kept, with your approval, until
  it comes back. Ask "why can't you use my filesystem server?" and the
  assistant says since when it is gone.
- back unchanged: it is used again as before.
- changed there (another program, version or address): it stops and you are
  asked to approve it again. Only a new key: the new key is kept, nothing
  else changes.
- a settings file that is half written or not readable changes nothing.

Some servers cannot be used as they are written there, and the page says
why: one started through a shell, one that needs a value only that program
fills in (such as `${env:TOKEN}`, or a question VS Code asks), an
environment file, a fixed working folder, the old SSE transport, or more than
one header. Add those by hand.

## Privacy
The list is yours: the names, versions and paths are sealed in the knowledge base like
everything personal, and the log only counts changes. Nothing on the page
starts a program or changes the system.
