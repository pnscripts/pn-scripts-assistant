# Looking after this computer

Open **System**: the card **Looking after this computer** looks at the
machine itself each time it is opened, or when you press **Look again**. You
can also ask: "can I restart now?", "is anything left running?".

## Can it be restarted now?
A restart is fine when nothing would be cut off and nothing would come back
missing. The card says what stands in the way:

- **A newer kernel is waiting** — said, because the restart will start it.
  If a driver built by DKMS (a USB Wi-Fi adapter's, a graphics card's) is
  built for the running kernel but not for the new one, it would be missing
  after the restart: the card names it and the command that rebuilds it.
- **Software is being installed or updated** — wait until it has finished.
- **The assistant is still working** on a task, or a coding agent is.
- **Work not committed or not pushed** in the places it learns from and the
  folders its tasks worked in — a restart keeps it, but it is worth knowing.

## Left running
Processes whose job looks over: a browser running without a window, a
virtual display, a waiting `sleep` or `timeout`, a `tail -f` on a log, a
small web server — started by the assistant, or whose starter has gone, and
running for at least half an hour. Your own programs, anything started in a
terminal and other users' processes are never listed. **Stop** asks once
more on the page, then asks the process to stop and, only if it does not
within five seconds, ends it. In a conversation the same is asked before it
is done.

## After a power cut
When the assistant was not closed — a power cut, a crash — the card says
when it was last seen working. A task it was running is parked, with the
reason, and waits under **Tasks** for you to resume or stop it; a
conversation cut off mid-answer says so.

## A paid allowance
Before a coding agent is given a task, the question shows how its allowance
is being spent — "60 % used with 30 % of its 7-day window gone — ahead of
pace" — whenever the agent has said, so you know what the work costs before
you say yes.
