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

## Privacy
The list is yours: the names, versions and paths are sealed in the knowledge base like
everything personal, and the log only counts changes. Nothing on the page
starts a program or changes the system.
