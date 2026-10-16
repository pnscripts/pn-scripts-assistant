# ADR 0008 — Every permission, and personal data sealed at rest

**Status:** accepted · **Date:** 2026-10-10 · **Builds on:** ADR 0007

## The request

Its owner, in his words (translated): "make PN Scripts Assistant have
absolutely all and every kind of permission. If there is personal data etc.,
it should be encrypted so that only the program can read it."

Two halves. ADR 0007 lifted the approvals, the confinement and the privacy
default. This finds what was still in the way and lifts it, adds a way to
become root that leaves root's password with its owner, and seals what the
program keeps about him so that only the program can read it.

## A. The remaining limits

Found by reading for refusals, caps, timeouts, block-lists and confirmations.

| Limit | Was | Now |
|---|---|---|
| A command's time (`run_command`) | 5 minutes | 3 hours — the website's full suite alone takes 90 minutes |
| A command's environment | inherited minus anything credential-shaped | inherited whole while everything is allowed, so `gh`, SSH and cloud CLIs work as in his terminal |
| Steps in one turn (`agent.MaxSteps`) | 8 | 30 — publishing or building is more than eight calls |
| A task's budget (`tasks.Sensible`) | 12 steps, 40 calls, 1 replan, 30 minutes | 40 steps, 200 calls, 2 replans, 4 hours |
| One download (`preflight`) | 3 GB | 64 GB |
| Model installs without asking (`orchestrator.Default`) | 10 GB each, 40 GB in all, 20 GB left free | 100 GB each, 400 GB in all, 5 GB left free |
| Confirmation before moving the brain or opening a copy | a dialog | none: nothing is deleted and both take effect on restart |
| Root | impossible from a conversation (`sudo` has no terminal) | **`run_as_admin`**: the system asks him — polkit/pkexec on Linux, UAC on Windows, the administrator prompt on macOS |
| Per-name refusals (`permissions.json`) | none by default | unchanged: none by default, and he can still refuse a tool by name |
| `BRAIN_TURNED_OFF` | empty | unchanged: empty |

Lifted already in ADR 0007 and listed again for completeness: the always-asked
approvals, protected files, project confinement and its kernel sandbox, a
hire's tool lists, integration grants and approval, local and private
addresses, the coding agents' own checks (`--dangerously-skip-permissions`,
`--dangerously-bypass-approvals-and-sandbox`).

**Kept, and why** — none of these is a permission the assistant lacks:

- **Paid use stays at a budget of nothing** (`MonthlyAPI`, `TaskAPI`). That is
  money, and the sales hold and cost rules in the playbooks are his. Setting a
  budget in `resources/policy.json` is one line.
- **Deleting a conversation and forgetting the voiceprint still ask**: they are
  his own clicks on the only copy, not something the assistant is stopped
  from doing — `run_command` can delete anything.
- **A project is not laid out in `/`, `/etc` or the home folder itself**
  (`workspace.forbiddenRoots`): that decides where a new project's folder goes
  and asks where instead; it does not stop any tool from writing anywhere.
- **The download door's hosts and checksums** (`preflight`): integrity, not
  permission — the model cannot name an install URL at all.
- **Network reach** (`BRAIN_REACH`): who may connect *to* the assistant, behind
  pairing and TLS. Not a limit on the assistant.
- **Truncation of what is read back** (`MaxReadBytes`, `MaxFetchChars`,
  `ToolOutputLimit`): the model's context, not a refusal; nothing is blocked.
- **Privacy is not changed from a conversation** (`change_a_setting`): with
  everything open, the only change a conversation could make is to close it,
  and a page the assistant reads should not be able to do that.
- **The system prompt still says to show what will be sent before sending
  mail or spending money.** A habit about other people, not a gate in code.

### Root, and why not passwordless

Passwordless sudo, a sudoers line or a setuid helper would change the
machine's own security, which is its owner's to change and not this program's.
`run_as_admin` asks the operating system to ask him, each time.

If he decides he wants no question at all, he can grant it himself — this is a
note, nothing here does it:

- Linux, for one command: `sudo visudo -f /etc/sudoers.d/pn-scripts-assistant`
  and a line such as `alex ALL=(root) NOPASSWD: /usr/bin/apt-get`, then point
  the assistant at `sudo -n apt-get …` through `run_command`. Wider grants
  (`NOPASSWD: ALL`) are possible and mean anything running as his user is root.
- Linux, through polkit: a rule in `/etc/polkit-1/rules.d/` returning
  `polkit.Result.YES` for `org.freedesktop.policykit.exec` and his user makes
  `pkexec` stop asking.
- Windows: turning UAC prompts off for administrators (Local Security Policy)
  removes the question for everything, not only this program.
- macOS: a sudoers entry as on Linux.

## B. Sealed at rest

### What is personal, and what happens to it

| Data | Where | Sealed how |
|---|---|---|
| Conversations: titles, every message | SQLite `conversations`, `messages` | each value, AES-256-GCM |
| What it learned: lessons, facts **and their embeddings** | `lessons`, `knowledge_facts` | text and vector blobs |
| The audit log of every tool call: arguments, summary, result | `tool_invocations` | each value; decrypted for the interface by the database connection |
| Reminders, calendar events (title, place, notes), goals | `reminders`, `events`, `goals` | each value |
| Tasks and steps: name, goal, report, instructions, answers, evidence text | `tasks`, `task_steps` | each value |
| Agents' memories, proposals, evidence, the activity feed | `agent_memories`, `proposals`, `evidence`, `activity` | each value |
| What he wrote about himself | `profile.md` | whole file |
| Integration credentials | `integrations/secrets.json` | whole file |
| His voiceprint | `voiceprint/known.json` | whole file |
| Recordings kept of turns that failed | `unheard/*.wav.pnsealed` | whole file |
| Passwords, API keys, tokens, his name and mail address | `brain.conf` | each value, in place |
| The program's log | `~/.local/state/pn-scripts-assistant/logs` | each line, once the brain is unlocked |

Not sealed, said plainly: the occupation catalogue and other public data; ids,
times, states and tool names (the shape of the record, not its content); the
paths a fact was learned from (`source`) and the folders it reads; the skills
he writes by hand, which are meant to be opened in an editor; generated
pictures and project folders, which are his work rather than data about him;
what was logged before the brain was unlocked at start, which is about
starting.

### How

- **One data key per brain**: 256 random bits, AES-256-GCM from the standard
  library, a fresh 96-bit nonce per value, the key's id bound in as
  authenticated data. A sealed value carries a marker (`pnsealed1:` for text,
  a binary magic for blobs), the key id, the nonce and the tag. **No new
  dependency**: `crypto/aes` and `crypto/cipher` are the standard library;
  `golang.org/x/crypto` is not in the module graph and is not needed.
- **Columns, not the file.** modernc's SQLite has no page encryption. The
  alternatives were a whole-file seal around a plaintext working copy — on a
  machine with frequent power cuts, everything said since the last
  re-encryption would be lost, and the plaintext copy would sit somewhere —
  or a SQLCipher-style driver, which means replacing the database engine and
  adding dependencies. Column sealing keeps SQLite's own crash safety intact:
  every write is still one transaction.
- **Reading is transparent.** The store opens its pool through a small
  `database/sql` connector (`store/sealed.go`) that opens every sealed value
  on its way out of the driver, so the hundred places that read are unchanged
  and the interface shows the audit log as before. Writing is explicit:
  `d.seal(…)` at every place something personal is written. The few queries
  that did text work in SQL on a personal column (`substr`, `LIKE`, a blank
  check) now do it in Go.
- **Nothing left behind.** `secure_delete` is on, so freed pages are zeroed;
  sealing in place ends with `VACUUM` and an emptied journal.
- **Tampering and the wrong key are errors**, not gibberish: a changed byte
  fails GCM's tag, another brain's key is caught by its id, and either fails
  the query.

### Where the key lives

| System | Keyring | How |
|---|---|---|
| Linux | the Secret Service (GNOME Keyring, KWallet) | libsecret, loaded at run time from the GTK desktop the window already needs — no build dependency; asked with a 20-second limit |
| Windows | DPAPI, for his account on that computer | `golang.org/x/sys/windows`, already a dependency |
| macOS | the Keychain | the `security` command |
| none of those answering | a key file, 0600, in his settings folder — never on the brain's drive | with a warning in the interface saying what it does not protect |

`PN_SCRIPTS_ASSISTANT_KEYRING=file` forces the key file.

**What this protects against, honestly:** the drive, a copy or a backup being
taken or read elsewhere; another user on the machine; another machine. **What
it does not:** a program running as him, which can ask the same keyring this
one asks — that is the limit of any program-only encryption on a desktop. With
everything allowed (ADR 0007), the assistant itself can read its own key's
keyring entry too; the record shows it if it does.

### The drive travels, so the key has a way to travel too

- On first sealing a **recovery key** is made: 256 random bits, written as 52
  letters and digits in groups of four. `vault.json`, beside the brain, holds
  the data key wrapped with it — and nothing that opens the brain without it.
- The interface shows the recovery key **once**, in a banner, with "Save it as
  a file" and "I have kept it". Until he presses the second, it is kept in
  `vault.json` sealed with the data key, so a restart does not lose it; after,
  it is gone and never shown again. It is shown only on the computer itself,
  never to a paired device.
- **On a new machine**, whose keyring has never seen this brain, the app opens
  a small window asking for the recovery key (or `pn-scripts-assistant
  unlock` in a terminal). The recovery key unwraps the data key, which is then
  put into that machine's keyring; from then on it opens there without asking.
- Copies (`brain_copies`) carry `vault.json`; moving the brain carries the
  whole folder.
- A database already sealed whose `vault.json` is missing is **not** given a
  new key, which would make it unreadable; the program says to put the file
  back.

### Sealing an existing brain, with the power going off

On the first start of this version, before anything is read:

1. A sealed copy of the whole database is written beside it
   (`brain.sqlite.before-sealing.pnsealed`, chunked and authenticated, chunk
   order bound in) and read back until it matches byte for byte. Nothing is
   changed until it does.
2. Rows are sealed 200 at a time, each batch one transaction. A value is told
   sealed by looking at it, so a cut leaves every row either as it was or
   sealed, and the next start carries on.
3. `VACUUM` and an emptied journal, then `vault_state` says done.

The test kills the process after the second batch — no cleanup, the way a cut
would — then starts again, finishes, finds every message readable, no plain
text in the database or its journal, and the copy taken first opening to the
brain as it was.

## Verified

- Unit tests: round trip; another key; a changed byte, a cut value and a cut
  stream; recovery on a fresh keyring, a wrong recovery key, the recovery key
  typed loosely; a missing `vault.json` not replaced over sealed data; the key
  file's mode; a sealed log; a sealed brain with no marker string left in the
  database or its journal, read back after a restart; a migration killed part
  way that resumes; `run_as_admin` reaching the system's asker.
- **The real Secret Service through libsecret**, inside a session bus and
  GNOME Keyring of its own (`dbus-run-session` with a scratch
  `gnome-keyring-daemon --unlock --components=secrets`), so its owner's keyring
  was not touched: a key kept, read back, deleted.
- A scratch instance of the CGO build (own home, settings, state and data
  root; no session bus, so the key file was used, which is what a machine
  with no keyring gets):
  - a plain brain made by the previous build, holding a marker string in a
    message, the profile, the owner's name and a mail password, was sealed in
    place on the first start ("sealed 4 records in place", copy taken); the
    marker was then in none of `brain.sqlite`, its journal, `brain.conf`,
    `profile.md`, the copy or the log, and a new message written afterwards
    was sealed too;
  - the interface read every conversation back, showed the recovery key in its
    banner, and after a restart read everything back and still showed it;
    "I have kept it" removed it (the endpoint then answers 410);
  - a new home and settings folder — a new machine — got the unlock page; a
    wrong key was refused with the reason, the real one unlocked it, put the
    key into that machine's key file, and the brain opened with its
    conversations.
- **Not verified:** the Windows DPAPI and macOS Keychain paths (they build;
  no such machine here); `run_as_admin` with a real polkit dialog (it would
  put a password prompt in front of whoever is at this machine).
